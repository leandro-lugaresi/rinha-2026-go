# ─── Builder stage ─────────────────────────────────────────
FROM --platform=linux/amd64 golang:1.23-alpine AS builder

RUN apk add --no-cache ca-certificates

WORKDIR /src
COPY . .

# Build the indexer and generate the IVF index from references
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bin/indexer ./cmd/indexer
RUN /bin/indexer -input .context/rinha-de-backend-2026/resources/references.json.gz -output /var/references.ivf

# Build the API binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /bin/api ./cmd/api

# ─── Runtime stage ────────────────────────────────────────
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/api /app/api
COPY --from=builder /var/references.ivf /app/references.ivf

ENV RINHA_INDEX_PATH=/app/references.ivf

EXPOSE 9999

USER 65534:65534

ENTRYPOINT ["/app/api"]
