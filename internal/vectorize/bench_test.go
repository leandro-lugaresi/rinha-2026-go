package vectorize

import (
	"testing"
)

// legitPayload is a representative low-risk transaction payload.
var legitPayload = Payload{
	ID: "tx-legit-bench",
	Transaction: Transaction{
		Amount:       41.12,
		Installments: 2,
		RequestedAt:  "2026-03-11T18:45:53Z",
	},
	Customer: Customer{
		AvgAmount:      82.24,
		TxCount24h:     3,
		KnownMerchants: []string{"MERC-003", "MERC-016"},
	},
	Merchant: Merchant{
		ID:        "MERC-016",
		MCC:       "5411",
		AvgAmount: 60.25,
	},
	Terminal: Terminal{
		IsOnline:    false,
		CardPresent: true,
		KmFromHome:  29.23,
	},
	LastTransaction: &LastTransaction{
		Timestamp:     "2026-03-11T14:58:35Z",
		KmFromCurrent: 18.8,
	},
}

// fraudPayload is a representative high-risk transaction payload.
var fraudPayload = Payload{
	ID: "tx-fraud-bench",
	Transaction: Transaction{
		Amount:       9505.97,
		Installments: 10,
		RequestedAt:  "2026-03-14T05:15:12Z",
	},
	Customer: Customer{
		AvgAmount:      81.28,
		TxCount24h:     20,
		KnownMerchants: []string{"MERC-008", "MERC-007", "MERC-005"},
	},
	Merchant: Merchant{
		ID:        "MERC-068",
		MCC:       "7802",
		AvgAmount: 54.86,
	},
	Terminal: Terminal{
		IsOnline:    false,
		CardPresent: true,
		KmFromHome:  952.27,
	},
	LastTransaction: &LastTransaction{
		Timestamp:     "2026-03-14T04:15:12Z",
		KmFromCurrent: 10.5,
	},
}

// BenchmarkVectorize measures the full vectorization of a legit payload.
func BenchmarkVectorize(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Vectorize(legitPayload)
		if err != nil {
			b.Fatalf("Vectorize: %v", err)
		}
	}
}

// BenchmarkVectorizeFraud measures vectorization of a fraud-like payload.
func BenchmarkVectorizeFraud(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Vectorize(fraudPayload)
		if err != nil {
			b.Fatalf("Vectorize: %v", err)
		}
	}
}

// BenchmarkVectorizeNoLastTx measures vectorization when last_transaction is nil
// (sentinel path for dimensions 5 and 6).
func BenchmarkVectorizeNoLastTx(b *testing.B) {
	p := legitPayload
	p.LastTransaction = nil
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Vectorize(p)
		if err != nil {
			b.Fatalf("Vectorize: %v", err)
		}
	}
}

// BenchmarkVectorizeUnknownMerchant measures vectorization with an unknown merchant
// (exercising the known-merchant map lookup).
func BenchmarkVectorizeUnknownMerchant(b *testing.B) {
	p := legitPayload
	p.Merchant.ID = "MERC-999" // not in known_merchants
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := Vectorize(p)
		if err != nil {
			b.Fatalf("Vectorize: %v", err)
		}
	}
}
