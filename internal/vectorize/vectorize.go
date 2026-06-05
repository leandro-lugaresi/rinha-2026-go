package vectorize

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
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

var (
	resourcesMu sync.RWMutex
	config      NormConfig
	mccRisk     map[string]float64
)

// LoadResources loads the normalization constants and MCC risk reference data.
func LoadResources(normalizationPath, mccRiskPath string) error {
	cfg, err := LoadConfig(normalizationPath)
	if err != nil {
		return err
	}
	risk, err := LoadMCCRisk(mccRiskPath)
	if err != nil {
		return err
	}

	resourcesMu.Lock()
	config = cfg
	mccRisk = risk
	resourcesMu.Unlock()
	return nil
}

// LoadConfig reads normalization constants from normalization.json.
func LoadConfig(path string) (NormConfig, error) {
	f, err := os.Open(path)
	if err != nil {
		return NormConfig{}, fmt.Errorf("open normalization config: %w", err)
	}
	defer f.Close()

	var cfg NormConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return NormConfig{}, fmt.Errorf("decode normalization config: %w", err)
	}
	return cfg, nil
}

// LoadMCCRisk reads the MCC risk map from mcc_risk.json.
func LoadMCCRisk(path string) (map[string]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open MCC risk map: %w", err)
	}
	defer f.Close()

	var risk map[string]float64
	if err := json.NewDecoder(f).Decode(&risk); err != nil {
		return nil, fmt.Errorf("decode MCC risk map: %w", err)
	}
	return risk, nil
}

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

	resourcesMu.RLock()
	cfg := config
	riskByMCC := mccRisk
	resourcesMu.RUnlock()
	if riskByMCC == nil {
		return vec, fmt.Errorf("vectorize resources not loaded")
	}

	requestedAt, err := time.Parse(time.RFC3339, payload.Transaction.RequestedAt)
	if err != nil {
		return vec, err
	}

	// 0: amount
	vec[0] = clamp(payload.Transaction.Amount / cfg.MaxAmount)

	// 1: installments
	vec[1] = clamp(float64(payload.Transaction.Installments) / cfg.MaxInstallments)

	// 2: amount_vs_avg (fail-safe: avg_amount <= 0 → clamp to 1.0)
	if payload.Customer.AvgAmount <= 0 {
		vec[2] = 1.0
	} else {
		vec[2] = clamp((payload.Transaction.Amount / payload.Customer.AvgAmount) / cfg.AmountVsAvgRatio)
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
		vec[5] = clamp(minutes / cfg.MaxMinutes)
		vec[6] = clamp(payload.LastTransaction.KmFromCurrent / cfg.MaxKm)
	}

	// 7: km_from_home
	vec[7] = clamp(payload.Terminal.KmFromHome / cfg.MaxKm)

	// 8: tx_count_24h
	vec[8] = clamp(float64(payload.Customer.TxCount24h) / cfg.MaxTxCount24h)

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
	vec[11] = 1
	for _, m := range payload.Customer.KnownMerchants {
		if m == payload.Merchant.ID {
			vec[11] = 0
			break
		}
	}

	// 12: mcc_risk (default 0.5)
	if risk, ok := riskByMCC[payload.Merchant.MCC]; ok {
		vec[12] = risk
	} else {
		vec[12] = 0.5
	}

	// 13: merchant_avg_amount
	vec[13] = clamp(payload.Merchant.AvgAmount / cfg.MaxMerchantAvgAmount)

	return vec, nil
}
