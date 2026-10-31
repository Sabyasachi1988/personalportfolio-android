package finance

import (
	"math"
	"testing"
	"time"

	"ledger/internal/store"
)

func TestCompositionHistoryWeighting(t *testing.T) {
	u := func(v float64) *float64 { return &v }
	p := &store.Portfolio{
		Accounts: []store.Account{{ID: "ac", MemberID: "m", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "A", AccountID: "ac"}, {ID: "B", AccountID: "ac"}},
		Transactions: []store.StoredTransaction{
			{AssetID: "A", Date: "2025-01-10", Units: u(10)}, {AssetID: "B", Date: "2025-02-05", Units: u(10)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "A", Date: "2025-01-31", Price: 10}, {AssetID: "A", Date: "2025-02-28", Price: 10},
			{AssetID: "B", Date: "2025-02-28", Price: 30},
		},
	}
	p.MergeFundMonths("A", "1", "v", "d", []store.FundMonth{{Month: "2025-01", Split: []float64{100, 0, 0, 0, 0, 0, 0, 0}, Sectors: map[string]float64{"Banks": 100}}})
	p.MergeFundMonths("B", "2", "v", "d", []store.FundMonth{{Month: "2025-02", Split: []float64{0, 0, 100, 0, 0, 0, 0, 0}}})
	pts := ComputeCompositionHistory(p, "", time.Date(2025, 2, 28, 0, 0, 0, 0, time.UTC))
	if len(pts) != 2 {
		t.Fatalf("points %d", len(pts))
	}
	if math.Abs(pts[0].Large-100) > 1e-6 {
		t.Fatalf("jan %+v", pts[0])
	}
	// Feb: A=100 (large, carried one month from Jan: not flagged), B=300 small -> 25 / 75
	if math.Abs(pts[1].Large-25) > 1e-6 || math.Abs(pts[1].Small-75) > 1e-6 || pts[1].Estimated != 0 {
		t.Fatalf("feb %+v", pts[1])
	}
}
