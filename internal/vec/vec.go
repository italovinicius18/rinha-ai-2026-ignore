// Package vec converts a fraud-score request payload into the 14-dimensional
// vector specified in docs/en/DETECTION_RULES.md of the Rinha 2026 challenge.
//
// Constants (normalization.json) and the MCC risk table (mcc_risk.json) are
// baked in — they don't change during the test.
package vec

import (
	"fmt"
	"time"
)

// Normalization constants (resources/normalization.json).
const (
	maxAmount            = 10000.0
	maxInstallments      = 12.0
	amountVsAvgRatio     = 10.0
	maxMinutes           = 1440.0
	maxKm                = 1000.0
	maxTxCount24h        = 20.0
	maxMerchantAvgAmount = 10000.0
)

// MCC risk table (resources/mcc_risk.json). Unknown MCC defaults to 0.5.
var mccRisk = map[string]float64{
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

const defaultMccRisk = 0.5

// --- payload schema ---

type Payload struct {
	ID              string     `json:"id"`
	Transaction     Tx         `json:"transaction"`
	Customer        Customer   `json:"customer"`
	Merchant        Merchant   `json:"merchant"`
	Terminal        Terminal   `json:"terminal"`
	LastTransaction *LastTx    `json:"last_transaction"`
}

type Tx struct {
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

type LastTx struct {
	Timestamp     string  `json:"timestamp"`
	KmFromCurrent float64 `json:"km_from_current"`
}

// Vectorize produces the 14-dim feature vector for p.
//
// Indices 5 and 6 hold the sentinel -1 when LastTransaction is nil (no
// previous transaction). All other indices are clamped to [0,1].
//
// Day-of-week convention: Monday=0, Sunday=6 (spec). Go's time.Weekday is
// Sunday=0..Saturday=6, so we shift by (+6 mod 7).
func Vectorize(p *Payload) ([14]float64, error) {
	var v [14]float64

	reqAt, err := time.Parse(time.RFC3339, p.Transaction.RequestedAt)
	if err != nil {
		return v, fmt.Errorf("parse requested_at %q: %w", p.Transaction.RequestedAt, err)
	}

	v[0] = clamp(p.Transaction.Amount / maxAmount)
	v[1] = clamp(float64(p.Transaction.Installments) / maxInstallments)

	if p.Customer.AvgAmount > 0 {
		v[2] = clamp((p.Transaction.Amount / p.Customer.AvgAmount) / amountVsAvgRatio)
	} else {
		// avg_amount = 0 → ratio is infinite → clamp to 1.
		v[2] = 1.0
	}

	v[3] = float64(reqAt.Hour()) / 23.0
	v[4] = float64((int(reqAt.Weekday())+6)%7) / 6.0

	if p.LastTransaction == nil {
		v[5] = -1
		v[6] = -1
	} else {
		lastAt, err := time.Parse(time.RFC3339, p.LastTransaction.Timestamp)
		if err != nil {
			return v, fmt.Errorf("parse last_transaction.timestamp %q: %w", p.LastTransaction.Timestamp, err)
		}
		minutes := reqAt.Sub(lastAt).Minutes()
		v[5] = clamp(minutes / maxMinutes)
		v[6] = clamp(p.LastTransaction.KmFromCurrent / maxKm)
	}

	v[7] = clamp(p.Terminal.KmFromHome / maxKm)
	v[8] = clamp(float64(p.Customer.TxCount24h) / maxTxCount24h)
	v[9] = boolFloat(p.Terminal.IsOnline)
	v[10] = boolFloat(p.Terminal.CardPresent)
	v[11] = unknownMerchant(p.Merchant.ID, p.Customer.KnownMerchants)
	v[12] = mccRiskFor(p.Merchant.MCC)
	v[13] = clamp(p.Merchant.AvgAmount / maxMerchantAvgAmount)

	return v, nil
}

func clamp(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func unknownMerchant(id string, known []string) float64 {
	for _, k := range known {
		if k == id {
			return 0
		}
	}
	return 1
}

func mccRiskFor(mcc string) float64 {
	if v, ok := mccRisk[mcc]; ok {
		return v
	}
	return defaultMccRisk
}
