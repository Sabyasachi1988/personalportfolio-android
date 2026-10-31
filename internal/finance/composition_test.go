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

func TestComputeFundCompositionSingleFund(t *testing.T) {
	p := &store.Portfolio{FundHistories: []store.FundHistory{{AssetID: "a", Months: []store.FundMonth{
		{Month: "2026-02", Split: []float64{50, 30, 10, 0, 0, 0, 5, 5}, Sectors: map[string]float64{"Banks": 40}},
		{Month: "2026-01", Split: []float64{60, 20, 10, 0, 0, 0, 5, 5}, Estimated: true},
	}}}}
	got := ComputeFundComposition(p, "a")
	if len(got) != 2 || got[0].Month != "2026-01" || got[0].Large != 60 || got[0].Estimated != 100 || got[1].Sectors["Banks"] != 40 {
		t.Fatalf("unexpected: %+v", got)
	}
	if n := len(ComputeFundComposition(p, "zzz")); n != 0 {
		t.Fatalf("unknown asset should be empty, got %d", n)
	}
}

// The donut/dashboard/drift market-cap numbers and the newest Over Time point must be the same numbers.
func TestMarketCapMixAgreesAcrossViews(t *testing.T) {
	u := func(v float64) *float64 { return &v }
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	start := now.AddDate(0, -3, 0).Format("2006-01-02")
	p := &store.Portfolio{
		Accounts: []store.Account{{ID: "ac", MemberID: "m", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "A", Name: "Pub Fund", AccountID: "ac", Type: "MutualFund"},
			{ID: "B", Name: "Hand Fund", AccountID: "ac", Type: "MutualFund"},
			{ID: "C", Name: "NIFTYBEES", AccountID: "ac", Type: "Stock"},
		},
		Transactions: []store.StoredTransaction{
			{AssetID: "A", Date: start, Units: u(10), Amount: -1000}, {AssetID: "B", Date: start, Units: u(10), Amount: -2000}, {AssetID: "C", Date: start, Units: u(10), Amount: -500},
		},
		Prices: []store.PriceRecord{
			{AssetID: "A", Date: today, Price: 100}, {AssetID: "B", Date: today, Price: 200}, {AssetID: "C", Date: today, Price: 50},
		},
	}
	p.MergeFundMonths("A", "1", "v", "d", []store.FundMonth{{Month: now.AddDate(0, -1, 0).Format("2006-01"), Split: []float64{60, 20, 10, 2, 3, 1, 2, 2}}})
	p.SetCapComposition("B", 10, 20, 70, 0, now.Format("2006-01")+"-01", "Manual entry")
	hs := ComputeHoldings(p)
	slices := AllocationByMarketCapMix(p, hs, now)
	by := map[string]float64{}
	for _, s := range slices {
		by[s.Label] = s.Percent
	}
	pts := ComputeCompositionHistory(p, "", now)
	last := pts[len(pts)-1]
	chk := func(name string, donut, over float64) {
		if math.Abs(donut-over) > 0.02 {
			t.Errorf("%s: donut %.2f vs over-time %.2f", name, donut, over)
		}
	}
	chk("large", by["Large Cap"], last.Large)
	chk("mid", by["Mid Cap"], last.Mid)
	chk("small", by["Small Cap"], last.Small)
	chk("cash", by["Cash"], last.Cash)
	chk("debt", by["Debt"], last.Debt)
	chk("foreign", by["International Equity"], last.Foreign)
	chk("others", by["Others"], last.Other)
	chk("unclassified", by["Unclassified"], last.Unclassified)
	if by["Large Cap"] < 1 {
		t.Fatalf("expected some large cap: %v", by)
	}
}
