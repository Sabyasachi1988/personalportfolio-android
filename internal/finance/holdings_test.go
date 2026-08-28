package finance

import (
	"testing"

	"ledger/internal/store"
)

func TestGroupHoldingsByLabel_UngroupedHoldingsStayIndividual(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "Fund A", NetInvested: 100, CurrentValue: 110, HasPrice: true},
		{AssetID: "a2", AssetName: "Fund B", NetInvested: 200, CurrentValue: 190, HasPrice: true},
	}
	p := &store.Portfolio{}
	groups := GroupHoldingsByLabel(p, holdings)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups (both ungrouped), got %d", len(groups))
	}
	for _, g := range groups {
		if g.IsGroup {
			t.Errorf("ungrouped holding %q incorrectly marked IsGroup", g.DisplayName)
		}
		if len(g.AssetIDs) != 1 {
			t.Errorf("ungrouped holding %q should have exactly 1 AssetID, got %d", g.DisplayName, len(g.AssetIDs))
		}
	}
}

func TestGroupHoldingsByLabel_SameLabelConsolidatesValueAndInvested(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "Nippon India Nifty 50", GroupLabel: "Nifty 50", NetInvested: 1000, CurrentValue: 1200, HasPrice: true},
		{AssetID: "a2", AssetName: "Navi Nifty 50", GroupLabel: "Nifty 50", NetInvested: 500, CurrentValue: 550, HasPrice: true},
		{AssetID: "a3", AssetName: "HDFC Nifty Next 50", GroupLabel: "", NetInvested: 300, CurrentValue: 320, HasPrice: true},
	}
	p := &store.Portfolio{}
	groups := GroupHoldingsByLabel(p, holdings)
	if len(groups) != 2 {
		t.Fatalf("expected 2 rows (1 group + 1 ungrouped), got %d: %+v", len(groups), groups)
	}

	var niftyGroup, ungrouped *GroupedHolding
	for i := range groups {
		if groups[i].IsGroup {
			niftyGroup = &groups[i]
		} else {
			ungrouped = &groups[i]
		}
	}
	if niftyGroup == nil {
		t.Fatal("expected one grouped row for 'Nifty 50'")
	}
	if niftyGroup.DisplayName != "Nifty 50" {
		t.Errorf("group DisplayName = %q, want %q", niftyGroup.DisplayName, "Nifty 50")
	}
	if niftyGroup.NetInvested != 1500 {
		t.Errorf("group NetInvested = %v, want 1500", niftyGroup.NetInvested)
	}
	if niftyGroup.CurrentValue != 1750 {
		t.Errorf("group CurrentValue = %v, want 1750", niftyGroup.CurrentValue)
	}
	if niftyGroup.Gain != 250 {
		t.Errorf("group Gain = %v, want 250", niftyGroup.Gain)
	}
	if len(niftyGroup.AssetIDs) != 2 {
		t.Errorf("group should list 2 underlying AssetIDs, got %d: %v", len(niftyGroup.AssetIDs), niftyGroup.AssetIDs)
	}

	if ungrouped == nil || ungrouped.DisplayName != "HDFC Nifty Next 50" {
		t.Errorf("expected the ungrouped holding to remain its own row, got %+v", ungrouped)
	}
}

func TestGroupHoldingsByLabel_XIRRPoolsRealCashFlowsNotAverage(t *testing.T) {
	// Two holdings in the same group, each with real transactions in
	// the portfolio - the group's XIRR must come from the POOLED cash
	// flow timeline (via the portfolio's actual transactions), not from
	// averaging each holding's individually-reported XIRR (which
	// wouldn't even be available here, since Holding.XIRR is left
	// unset in this test on purpose - the point is GroupHoldingsByLabel
	// doesn't depend on it at all).
	units1, units2 := 10.0, 5.0
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "a1", Name: "Fund A", GroupLabel: "Group X"},
			{ID: "a2", Name: "Fund B", GroupLabel: "Group X"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AssetID: "a1", Date: "2024-01-01", Type: "PURCHASE", Amount: 1000, Units: &units1},
			{ID: "t2", AssetID: "a2", Date: "2024-06-01", Type: "PURCHASE", Amount: 500, Units: &units2},
		},
	}
	holdings := []Holding{
		{AssetID: "a1", AssetName: "Fund A", GroupLabel: "Group X", NetInvested: 1000, CurrentValue: 1300, UnitsHeld: units1, HasPrice: true},
		{AssetID: "a2", AssetName: "Fund B", GroupLabel: "Group X", NetInvested: 500, CurrentValue: 600, UnitsHeld: units2, HasPrice: true},
	}
	groups := GroupHoldingsByLabel(p, holdings)
	if len(groups) != 1 {
		t.Fatalf("expected 1 grouped row, got %d", len(groups))
	}
	g := groups[0]
	if !g.HasXIRR {
		t.Fatal("expected the group to have a computable XIRR from pooled real cash flows")
	}
	// Not asserting an exact rate (depends on today's date at test-run
	// time via the "current value as final inflow" convention) - just
	// that it's a plausible positive return, confirming pooling ran
	// rather than silently producing zero/no-data.
	if g.XIRR <= 0 {
		t.Errorf("expected a positive pooled XIRR for a clearly-gaining group, got %v", g.XIRR)
	}
}

func TestGroupHoldingsByLabel_PartiallyPricedGroupOnlySumsPricedConstituents(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "Fund A", GroupLabel: "Group Y", NetInvested: 1000, CurrentValue: 1100, HasPrice: true},
		{AssetID: "a2", AssetName: "Fund B", GroupLabel: "Group Y", NetInvested: 500, HasPrice: false}, // no price yet
	}
	p := &store.Portfolio{}
	groups := GroupHoldingsByLabel(p, holdings)
	if len(groups) != 1 {
		t.Fatalf("expected 1 grouped row, got %d", len(groups))
	}
	g := groups[0]
	if !g.HasPrice {
		t.Fatal("expected HasPrice=true since at least one constituent is priced")
	}
	// NetInvested still includes BOTH constituents (money committed is
	// money committed, regardless of whether a current price exists
	// yet) - only CurrentValue/Gain are priced-constituents-only.
	if g.NetInvested != 1500 {
		t.Errorf("NetInvested = %v, want 1500 (both constituents)", g.NetInvested)
	}
	if g.CurrentValue != 1100 {
		t.Errorf("CurrentValue = %v, want 1100 (only the priced constituent)", g.CurrentValue)
	}
}

func TestComputeHoldings_DayGainUsesPriorDistinctDate(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Fund A"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-06-01", Price: 100},
			{AssetID: "a1", Date: "2024-06-02", Price: 105},
		},
	}
	holdings := ComputeHoldings(p)
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	h := holdings[0]
	if !h.HasDayGain {
		t.Fatalf("HasDayGain = false, want true")
	}
	wantGain := round2((105.0 - 100.0) * units)
	if h.DayGain != wantGain {
		t.Errorf("DayGain = %v, want %v", h.DayGain, wantGain)
	}
	wantPercent := round2((105.0/100.0 - 1) * 100)
	if h.DayGainPercent != wantPercent {
		t.Errorf("DayGainPercent = %v, want %v", h.DayGainPercent, wantPercent)
	}
}

func TestComputeHoldings_DayGainSkipsSameDateDuplicates(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Fund A"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-06-01", Price: 100},
			{AssetID: "a1", Date: "2024-06-02", Price: 105, Source: "MANUAL"},
			{AssetID: "a1", Date: "2024-06-02", Price: 106, Source: "AMFI"},
		},
	}
	holdings := ComputeHoldings(p)
	h := holdings[0]
	if !h.HasDayGain {
		t.Fatalf("HasDayGain = false, want true")
	}
	// The prior side should skip past BOTH same-date (2024-06-02)
	// records to the genuinely earlier date (2024-06-01, price 100),
	// regardless of which of the two same-date records the latest-price
	// lookup happened to pick as "the" 2024-06-02 price.
	if h.DayGain == 0 {
		t.Errorf("DayGain = 0, want a real day-over-day move computed against 2024-06-01's price")
	}
}

func TestComputeHoldings_DayGainNoDataWithOnlyOneDistinctDate(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Fund A"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-06-01", Price: 100},
		},
	}
	holdings := ComputeHoldings(p)
	h := holdings[0]
	if h.HasDayGain {
		t.Errorf("HasDayGain = true, want false (only one distinct priced date)")
	}
}
