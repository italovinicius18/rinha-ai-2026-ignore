package vec

import (
	"math"
	"testing"
)

// epsilon covers the 4-decimal rounding shown in the spec examples.
const epsilon = 1e-3

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < epsilon
}

func assertVector(t *testing.T, got [14]float64, want [14]float64) {
	t.Helper()
	for i := 0; i < 14; i++ {
		if !almostEqual(got[i], want[i]) {
			t.Errorf("dim %d: got %v, want %v (Δ=%v)", i, got[i], want[i], math.Abs(got[i]-want[i]))
		}
	}
}

// Flow example from docs/en/DETECTION_RULES.md — legitimate.
func TestVectorize_LegitimateExample(t *testing.T) {
	p := &Payload{
		ID: "tx-1329056812",
		Transaction: Tx{
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

	want := [14]float64{
		0.0041, 0.1667, 0.05, 0.7826, 0.3333,
		-1, -1,
		0.0292, 0.15, 0, 1, 0, 0.15, 0.006,
	}

	got, err := Vectorize(p)
	if err != nil {
		t.Fatalf("Vectorize: %v", err)
	}
	assertVector(t, got, want)
}

// Flow example from docs/en/DETECTION_RULES.md — fraudulent.
func TestVectorize_FraudulentExample(t *testing.T) {
	p := &Payload{
		ID: "tx-3330991687",
		Transaction: Tx{
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

	want := [14]float64{
		0.9506, 0.8333, 1.0, 0.2174, 0.8333,
		-1, -1,
		0.9523, 1.0, 0, 1, 1, 0.75, 0.0055,
	}

	got, err := Vectorize(p)
	if err != nil {
		t.Fatalf("Vectorize: %v", err)
	}
	assertVector(t, got, want)
}

// Sanity check the non-null last_transaction path (taken from
// resources/example-payloads.json). No reference vector in the spec, so we
// check the two affected dims arithmetically.
func TestVectorize_LastTransactionNotNull(t *testing.T) {
	p := &Payload{
		ID: "tx-3576980410",
		Transaction: Tx{
			Amount:       384.88,
			Installments: 3,
			RequestedAt:  "2026-03-11T20:23:35Z",
		},
		Customer: Customer{
			AvgAmount:      769.76,
			TxCount24h:     3,
			KnownMerchants: []string{"MERC-009", "MERC-001", "MERC-001"},
		},
		Merchant: Merchant{
			ID:        "MERC-001",
			MCC:       "5912",
			AvgAmount: 298.95,
		},
		Terminal: Terminal{
			IsOnline:    false,
			CardPresent: true,
			KmFromHome:  13.7090520965,
		},
		LastTransaction: &LastTx{
			Timestamp:     "2026-03-11T14:58:35Z",
			KmFromCurrent: 18.8626479774,
		},
	}

	got, err := Vectorize(p)
	if err != nil {
		t.Fatalf("Vectorize: %v", err)
	}

	// 20:23:35 - 14:58:35 = 5h 25min = 325 min; 325/1440 = 0.2257
	if !almostEqual(got[5], 0.2257) {
		t.Errorf("dim 5 (minutes_since_last_tx): got %v, want ~0.2257", got[5])
	}
	// 18.8626479774 / 1000 = 0.01886
	if !almostEqual(got[6], 0.01886) {
		t.Errorf("dim 6 (km_from_last_tx): got %v, want ~0.01886", got[6])
	}
	// dim 11 unknown_merchant: MERC-001 is in known list → 0
	if got[11] != 0 {
		t.Errorf("dim 11 (unknown_merchant): got %v, want 0 (merchant is known)", got[11])
	}
	// dim 12 mcc_risk: 5912 → 0.20
	if !almostEqual(got[12], 0.20) {
		t.Errorf("dim 12 (mcc_risk for 5912): got %v, want 0.20", got[12])
	}
}

func TestClampBoundaries(t *testing.T) {
	cases := []struct{ in, want float64 }{
		{-0.5, 0}, {0, 0}, {0.5, 0.5}, {1, 1}, {1.5, 1},
	}
	for _, c := range cases {
		if got := clamp(c.in); got != c.want {
			t.Errorf("clamp(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestUnknownMerchant(t *testing.T) {
	known := []string{"M-1", "M-2"}
	if v := unknownMerchant("M-1", known); v != 0 {
		t.Errorf("known merchant should yield 0, got %v", v)
	}
	if v := unknownMerchant("M-99", known); v != 1 {
		t.Errorf("unknown merchant should yield 1, got %v", v)
	}
	if v := unknownMerchant("M-1", nil); v != 1 {
		t.Errorf("nil known list should yield 1, got %v", v)
	}
}

func TestMccRiskDefault(t *testing.T) {
	if got := mccRiskFor("5411"); !almostEqual(got, 0.15) {
		t.Errorf("known mcc 5411: got %v, want 0.15", got)
	}
	if got := mccRiskFor("9999"); got != defaultMccRisk {
		t.Errorf("unknown mcc 9999: got %v, want default %v", got, defaultMccRisk)
	}
}
