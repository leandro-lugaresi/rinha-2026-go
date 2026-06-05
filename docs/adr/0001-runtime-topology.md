# ADR 0001: Runtime Topology

## Status

Accepted

## Context

The Rinha de Backend 2026 challenge imposes strict infrastructure constraints on all submissions. The total budget is 1 CPU core and 350 MB of memory shared across every service in the `docker-compose.yml`. The challenge requires at least one load balancer and two API instances, with traffic distributed evenly using round-robin. The load balancer must not inspect payloads or apply any business logic; it forwards requests only. Submissions must listen on port 9999, use bridge network mode (no host or privileged modes), and produce linux-amd64-compatible images.

The fraud detection workload is a vector search task: each incoming transaction is normalized into a 14-dimensional vector, and the API queries a pre-loaded reference index of 3 million labeled vectors to find the 5 nearest neighbors. The per-request compute is dominated by distance calculations against the reference index, not by request parsing or response serialization.

## Decision

We adopt the following runtime topology:

- **HAProxy** acts as a pure round-robin load balancer. It binds port 9999 and distributes requests to two identical Go API instances behind it. HAProxy performs no payload inspection, no conditional routing, and no business-logic evaluation. Its sole function is forwarding.
- **Two Go API instances** run behind HAProxy, each built from the same container image and configuration. Each instance loads the complete reference index at startup.
- **Bridge network mode**: all containers communicate over a Docker bridge network. No host networking or privileged mode is used.
- **Resource allocation per API instance**: 0.45 CPU cores and 160 MB of memory. HAProxy receives 0.10 CPU and 20 MB. The split uses 340 MB (160+160+20) and 1.0 CPU (0.45+0.45+0.10), leaving 10 MB of memory headroom and no spare CPU under the 1 CPU / 350 MB aggregate ceiling.

```
Client (port 9999) -> HAProxy (0.10 CPU / 20 MB) -> Go API 1 (0.45 CPU / 160 MB)
                                                       -> Go API 2 (0.45 CPU / 160 MB)
```

HAProxy is configured with a `roundrobin` balance algorithm, a `option httpchk GET /ready` health check against each backend, and `option redispatch` to redistribute requests when a backend is unavailable. No `http-mode` inspection rules are added beyond what is needed to observe HTTP-level health checks.

## Consequences

- **Positive**: The topology satisfies all mandatory constraints (two API instances, round-robin LB, no business logic in LB, bridge network, port 9999). HAProxy is a proven, low-overhead load balancer with minimal memory and CPU footprint. The split gives each API process sufficient headroom to hold the reference index and handle concurrent requests.
- **Positive**: Having two independent instances provides a basic form of availability. If one instance becomes unresponsive, HAProxy redirects traffic to the other.
- **Negative**: Both API instances load the full reference index into memory independently, doubling the memory footprint for the index. This is mitigated by the memory budget calculation (see ADR 0004), which confirms the index fits within the 160 MB per-instance limit.
- **Negative**: Horizontal scaling beyond two instances would require increasing the number of HAProxy backends and would further multiply memory usage for the index. The challenge constraints (1 CPU / 350 MB total) make it impractical to add more instances without reducing per-instance resources below what the index requires.

## Alternatives considered

**Single Go process with multiple goroutines, no HAProxy.** This would eliminate the load balancer overhead but would violate the challenge requirement of at least two API instances with a load balancer distributing traffic in round-robin. Additionally, a single process provides no fault isolation.

**Three or more Go API instances with HAProxy.** This would improve aggregate throughput under concurrent load but would require splitting the memory budget across more processes. Each instance needs to hold the full reference index (see ADR 0004), so adding instances without increasing total memory is not feasible under the 350 MB ceiling.

**nginx as the load balancer.** nginx is a capable reverse proxy, but HAProxy is more standard in this competition format and offers a slightly simpler configuration for pure round-robin with health checking. The performance difference is negligible for this workload.

**Round-robin DNS instead of a load balancer.** DNS-based distribution does not provide real-time health checking or rapid failover, and the round-robin guarantee is weaker. A dedicated load balancer is the expected and more controllable approach.

**No load balancer, client-side round-robin.** This would require test clients to implement their own distribution logic, which is outside the participant's control. The challenge mandates a load balancer in the architecture.