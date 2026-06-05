package vectorize

import (
	"math"
	"testing"
)

// floatNear checks that got is within ±tolerance of want.
func floatNear(t *testing.T, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Errorf("got %.6f, want %.6f (diff %.6f > tolerance %.6f)", got, want, math.Abs(got-want), tolerance)
	}
}

func TestVectorization_LegitExample(t *testing.T) {
	payload := Payload{
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
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// expected from DETECTION_RULES.md lines 24-25
	expected := [14]float64{
		0.0041, 0.1667, 0.05, 0.7826, 0.3333,
		-1, -1,
		0.0292, 0.15, 0, 1, 0, 0.15, 0.006,
	}

	const tol = 0.0001
	for i, want := range expected {
		floatNear(t, vec[i], want, tol)
	}
}

func TestVectorization_FraudExample(t *testing.T) {
	payload := Payload{
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
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// expected from DETECTION_RULES.md lines 122-123
	expected := [14]float64{
		0.9506, 0.8333, 1.0, 0.2174, 0.8333,
		-1, -1,
		0.9523, 1.0, 0, 1, 1, 0.75, 0.0055,
	}

	const tol = 0.0001
	for i, want := range expected {
		floatNear(t, vec[i], want, tol)
	}
}

func TestVectorization_LastTransactionNull(t *testing.T) {
	payload := Payload{
		Transaction: Transaction{
			Amount:       100,
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[5] != -1.0 {
		t.Errorf("dim 5 (minutes_since_last_tx): got %f, want -1 (last_transaction null)", vec[5])
	}
	if vec[6] != -1.0 {
		t.Errorf("dim 6 (km_from_last_tx): got %f, want -1 (last_transaction null)", vec[6])
	}
}

func TestVectorization_UnknownMerchant(t *testing.T) {
	payload := Payload{
		Transaction: Transaction{
			Amount:       100,
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001", "MERC-002"},
		},
		Merchant: Merchant{
			ID:        "MERC-999",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[11] != 1.0 {
		t.Errorf("dim 11 (unknown_merchant): got %f, want 1 (MERC-999 not in known_merchants)", vec[11])
	}
}

func TestVectorization_KnownMerchant(t *testing.T) {
	payload := Payload{
		Transaction: Transaction{
			Amount:       100,
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001", "MERC-002"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[11] != 0.0 {
		t.Errorf("dim 11 (unknown_merchant): got %f, want 0 (MERC-001 IS in known_merchants)", vec[11])
	}
}

func TestVectorization_UnknownMCC(t *testing.T) {
	payload := Payload{
		Transaction: Transaction{
			Amount:       100,
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "9999", // not in mcc_risk.json
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[12] != 0.5 {
		t.Errorf("dim 12 (mcc_risk): got %f, want 0.5 (default for unknown MCC)", vec[12])
	}
}

func TestVectorization_WeekdayRemapping(t *testing.T) {
	// Monday 2026-03-02 → Go weekday = 1, spec weekday = 0
	payload := Payload{
		Transaction: Transaction{
			Amount:       100,
			Installments: 1,
			RequestedAt:  "2026-03-02T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantMonday := 0.0 / 6.0 // Monday=0
	if vec[4] != wantMonday {
		t.Errorf("dim 4 (day_of_week) for Monday: got %f, want %f", vec[4], wantMonday)
	}

	// Sunday 2026-03-08 → Go weekday = 0, spec weekday = 6
	payloadSun := payload
	payloadSun.Transaction.RequestedAt = "2026-03-08T12:00:00Z"

	vecSun, err := Vectorize(payloadSun)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSunday := 1.0 // Sunday=6, 6/6=1.0
	if vecSun[4] != wantSunday {
		t.Errorf("dim 4 (day_of_week) for Sunday: got %f, want %f", vecSun[4], wantSunday)
	}

	// Wednesday 2026-03-11 → Go weekday = 3, spec weekday = 2
	payloadWed := payload
	payloadWed.Transaction.RequestedAt = "2026-03-11T12:00:00Z"

	vecWed, err := Vectorize(payloadWed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantWednesday := 2.0 / 6.0 // Wednesday=2
	if vecWed[4] != wantWednesday {
		t.Errorf("dim 4 (day_of_week) for Wednesday: got %f, want %f", vecWed[4], wantWednesday)
	}
}

func TestVectorization_ClampHigh(t *testing.T) {
	// amount > max_amount should clamp to 1.0
	payload := Payload{
		Transaction: Transaction{
			Amount:       20000, // > max_amount 10000
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[0] != 1.0 {
		t.Errorf("dim 0 (amount): got %f, want 1.0 (clamped high)", vec[0])
	}
}

func TestVectorization_ClampLow(t *testing.T) {
	// negative values should clamp to 0
	payload := Payload{
		Transaction: Transaction{
			Amount:       -100,
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[0] != 0.0 {
		t.Errorf("dim 0 (amount): got %f, want 0.0 (clamped low)", vec[0])
	}
}

func TestVectorization_AmountVsAvgFailSafe(t *testing.T) {
	// customer.avg_amount <= 0 → clamp to 1.0 (fail-safe high-risk)
	payload := Payload{
		Transaction: Transaction{
			Amount:       100,
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      0, // zero triggers fail-safe
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[2] != 1.0 {
		t.Errorf("dim 2 (amount_vs_avg): got %f, want 1.0 (fail-safe when avg_amount=0)", vec[2])
	}

	// negative avg_amount also triggers fail-safe
	payloadNeg := payload
	payloadNeg.Customer.AvgAmount = -50

	vecNeg, err := Vectorize(payloadNeg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vecNeg[2] != 1.0 {
		t.Errorf("dim 2 (amount_vs_avg): got %f, want 1.0 (fail-safe when avg_amount=-50)", vecNeg[2])
	}
}

func TestVectorization_DuplicateKnownMerchants(t *testing.T) {
	// Duplicate merchants in known_merchants should not affect behavior
	payload := Payload{
		Transaction: Transaction{
			Amount:       100,
			Installments: 1,
			RequestedAt:  "2026-03-11T12:00:00Z",
		},
		Customer: Customer{
			AvgAmount:      100,
			TxCount24h:     1,
			KnownMerchants: []string{"MERC-001", "MERC-001", "MERC-001"}, // duplicates
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5411",
			AvgAmount: 100,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  10,
		},
		LastTransaction: nil,
	}

	vec, err := Vectorize(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if vec[11] != 0.0 {
		t.Errorf("dim 11 (unknown_merchant): got %f, want 0 (MERC-001 IS in known_merchants, even with duplicates)", vec[11])
	}
}
