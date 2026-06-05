package vectorize

import (
	"math"
	"time"
)

// Payload is the full transaction payload received by the fraud-score endpoint.
type Payload struct {
	ID              string           `json:"id"`
	Transaction     Transaction      `json:"transaction"`
	Customer        Customer         `json:"customer"`
	Merchant        Merchant         `json:"merchant"`
	Terminal        Terminal         `json:"terminal"`
	LastTransaction *LastTransaction `json:"last_transaction"`
}

type Transaction struct {
	Amount       float64 `json:"amount"`
	Installments int     `json:"installments"`
	RequestedAt  string  `json:"requested_at"`
}

type Customer struct {
	AvgAmount      float64  `json:"avg_amount"`
	TxCount24h     int      `json:"tx_count_24h"`
	KnownMerchants []string `json:"known_merchants"`
}

type Merchant struct {
	ID        string  `json:"id"`
	MCC       string  `json:"mcc"`
	AvgAmount float64 `json:"avg_amount"`
}

type Terminal struct {
	IsOnline    bool    `json:"is_online"`
	CardPresent bool    `json:"card_present"`
	KmFromHome  float64 `json:"km_from_home"`
}

type LastTransaction struct {
	Timestamp     string  `json:"timestamp"`
	KmFromCurrent float64 `json:"km_from_current"`
}

// NormConfig holds the normalization constants from normalization.json.
type NormConfig struct {
	MaxAmount            float64 `json:"max_amount"`
	MaxInstallments      float64 `json:"max_installments"`
	AmountVsAvgRatio     float64 `json:"amount_vs_avg_ratio"`
	MaxMinutes           float64 `json:"max_minutes"`
	MaxKm                float64 `json:"max_km"`
	MaxTxCount24h        float64 `json:"max_tx_count_24h"`
	MaxMerchantAvgAmount float64 `json:"max_merchant_avg_amount"`
}

// DefaultConfig returns the normalization constants from normalization.json.
func DefaultConfig() NormConfig {
	return NormConfig{
		MaxAmount:            10000,
		MaxInstallments:      12,
		AmountVsAvgRatio:     10,
		MaxMinutes:           1440,
		MaxKm:                1000,
		MaxTxCount24h:        20,
		MaxMerchantAvgAmount: 10000,
	}
}

// DefaultMCCRisk returns the MCC risk map from mcc_risk.json.
func DefaultMCCRisk() map[string]float64 {
	return map[string]float64{
		"5411": 0.15,
		"5812": 0.30,
		"5912": 0.20,
		"5944": 0.45,
		"7801": 0.80,
		"7802": 0.75,
		"7995": 0.85,
		"4511": 0.35,
		"5311": 0.25,
		"5999": 0.50,
	}
}

var (
	config  = DefaultConfig()
	mccRisk = DefaultMCCRisk()
)

func clamp(x float64) float64 {
	return math.Max(0.0, math.Min(1.0, x))
}

func dayOfWeekSpec(t time.Time) float64 {
	d := int(t.Weekday())
	specVal := (d + 6) % 7
	return float64(specVal)
}

// Vectorize transforms a transaction payload into a 14-dimensional vector
// following the normalization formulas from DETECTION_RULES.md.
func Vectorize(payload Payload) ([14]float64, error) {
	var vec [14]float64

	requestedAt, err := time.Parse(time.RFC3339, payload.Transaction.RequestedAt)
	if err != nil {
		return vec, err
	}

	// 0: amount
	vec[0] = clamp(payload.Transaction.Amount / config.MaxAmount)

	// 1: installments
	vec[1] = clamp(float64(payload.Transaction.Installments) / config.MaxInstallments)

	// 2: amount_vs_avg (fail-safe: avg_amount <= 0 → clamp to 1.0)
	if payload.Customer.AvgAmount <= 0 {
		vec[2] = 1.0
	} else {
		vec[2] = clamp((payload.Transaction.Amount / payload.Customer.AvgAmount) / config.AmountVsAvgRatio)
	}

	// 3: hour_of_day (0-23 UTC)
	vec[3] = float64(requestedAt.Hour()) / 23.0

	// 4: day_of_week (mon=0, sun=6)
	vec[4] = dayOfWeekSpec(requestedAt) / 6.0

	// 5, 6: minutes_since_last_tx, km_from_last_tx
	if payload.LastTransaction == nil {
		vec[5] = -1
		vec[6] = -1
	} else {
		lastTs, err := time.Parse(time.RFC3339, payload.LastTransaction.Timestamp)
		if err != nil {
			return vec, err
		}
		minutes := requestedAt.Sub(lastTs).Minutes()
		vec[5] = clamp(minutes / config.MaxMinutes)
		vec[6] = clamp(payload.LastTransaction.KmFromCurrent / config.MaxKm)
	}

	// 7: km_from_home
	vec[7] = clamp(payload.Terminal.KmFromHome / config.MaxKm)

	// 8: tx_count_24h
	vec[8] = clamp(float64(payload.Customer.TxCount24h) / config.MaxTxCount24h)

	// 9: is_online
	if payload.Terminal.IsOnline {
		vec[9] = 1
	} else {
		vec[9] = 0
	}

	// 10: card_present
	if payload.Terminal.CardPresent {
		vec[10] = 1
	} else {
		vec[10] = 0
	}

	// 11: unknown_merchant (1 = unknown, 0 = known)
	knownSet := make(map[string]struct{}, len(payload.Customer.KnownMerchants))
	for _, m := range payload.Customer.KnownMerchants {
		knownSet[m] = struct{}{}
	}
	if _, ok := knownSet[payload.Merchant.ID]; ok {
		vec[11] = 0
	} else {
		vec[11] = 1
	}

	// 12: mcc_risk (default 0.5)
	if risk, ok := mccRisk[payload.Merchant.MCC]; ok {
		vec[12] = risk
	} else {
		vec[12] = 0.5
	}

	// 13: merchant_avg_amount
	vec[13] = clamp(payload.Merchant.AvgAmount / config.MaxMerchantAvgAmount)

	return vec, nil
}
