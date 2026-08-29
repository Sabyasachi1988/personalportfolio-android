package finance

import (
	"encoding/json"
	"strings"
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

func TestGroupHoldingsByLabel_UngroupedRowUsesCanonicalNameAndCarriesAlsoHeldBy(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "Nippon India Nifty 50 Index Fund", CanonicalName: "Nippon India Nifty 50", AlsoHeldByMembers: []string{"Mom"}, NetInvested: 100, CurrentValue: 110, HasPrice: true},
	}
	p := &store.Portfolio{}
	groups := GroupHoldingsByLabel(p, holdings)
	if len(groups) != 1 {
		t.Fatalf("expected 1 row, got %d", len(groups))
	}
	g := groups[0]
	if g.DisplayName != "Nippon India Nifty 50" {
		t.Errorf("DisplayName = %q, want the harmonized CanonicalName %q, not the raw AssetName", g.DisplayName, "Nippon India Nifty 50")
	}
	if len(g.AlsoHeldByMembers) != 1 || g.AlsoHeldByMembers[0] != "Mom" {
		t.Errorf("AlsoHeldByMembers = %v, want [Mom]", g.AlsoHeldByMembers)
	}
}

func TestGroupHoldingsByLabel_UngroupedRowFallsBackToAssetNameWhenNoCanonicalNameSet(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "Fund With No CanonicalName Set", NetInvested: 100, CurrentValue: 110, HasPrice: true},
	}
	p := &store.Portfolio{}
	groups := GroupHoldingsByLabel(p, holdings)
	if groups[0].DisplayName != "Fund With No CanonicalName Set" {
		t.Errorf("DisplayName = %q, want the AssetName fallback", groups[0].DisplayName)
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

func TestComputeHoldings_CanonicalNameHarmonizedAcrossSameISIN(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Me"}, {ID: "m2", Name: "Mom"}},
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1"}, {ID: "acc2", MemberID: "m2"}},
		Assets: []store.Asset{
			// Added FIRST - its name should win as canonical for both.
			{ID: "a1", AccountID: "acc1", Name: "Nippon India Nifty 50", ISIN: "INF_SHARED_0001"},
			{ID: "a2", AccountID: "acc2", Name: "Nippon India Nifty 50 Index Fund", ISIN: "INF_SHARED_0001"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
			{ID: "t2", AccountID: "acc2", AssetID: "a2", Date: "2024-01-01", Amount: 1000, Units: &units},
		},
	}
	holdings := ComputeHoldings(p)
	if len(holdings) != 2 {
		t.Fatalf("expected 2 holdings, got %d", len(holdings))
	}
	for _, h := range holdings {
		if h.CanonicalName != "Nippon India Nifty 50" {
			t.Errorf("holding %s: CanonicalName = %q, want %q (the FIRST-added same-ISIN asset's name)", h.AssetID, h.CanonicalName, "Nippon India Nifty 50")
		}
		// AssetName itself must stay UNTOUCHED - CanonicalName is an
		// additional field, not a replacement, per the doc comment's
		// own point about not merging the underlying data.
	}
	if holdings[0].AssetName == holdings[1].AssetName {
		t.Errorf("AssetName should NOT be harmonized (only CanonicalName) - got both = %q", holdings[0].AssetName)
	}
}

func TestComputeHoldings_AlsoHeldByMembersCrossReferencesOtherMembers(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Me"}, {ID: "m2", Name: "Mom"}},
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1"}, {ID: "acc2", MemberID: "m2"}},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Nippon India Nifty 50", ISIN: "INF_SHARED_0001"},
			{ID: "a2", AccountID: "acc2", Name: "Nippon India Nifty 50 Index Fund", ISIN: "INF_SHARED_0001"},
			{ID: "a3", AccountID: "acc1", Name: "A Fund Only I Hold", ISIN: "INF_UNIQUE_0002"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
			{ID: "t2", AccountID: "acc2", AssetID: "a2", Date: "2024-01-01", Amount: 1000, Units: &units},
			{ID: "t3", AccountID: "acc1", AssetID: "a3", Date: "2024-01-01", Amount: 500, Units: &units},
		},
	}
	holdings := ComputeHoldings(p)
	byAssetID := make(map[string]Holding)
	for _, h := range holdings {
		byAssetID[h.AssetID] = h
	}

	if got := byAssetID["a1"].AlsoHeldByMembers; len(got) != 1 || got[0] != "Mom" {
		t.Errorf("a1.AlsoHeldByMembers = %v, want [Mom]", got)
	}
	if got := byAssetID["a2"].AlsoHeldByMembers; len(got) != 1 || got[0] != "Me" {
		t.Errorf("a2.AlsoHeldByMembers = %v, want [Me]", got)
	}
	if got := byAssetID["a3"].AlsoHeldByMembers; len(got) != 0 {
		t.Errorf("a3.AlsoHeldByMembers = %v, want empty (unique ISIN, nobody else holds it)", got)
	}
}

// TestComputeHoldings_AlsoHeldByMembersNeverMarshalsAsNull is the actual
// regression test for a CONFIRMED REAL CRASH: a nil Go slice marshals
// to JSON `null`, and Gson's Kotlin deserialization sets the field to
// an actual null REGARDLESS of the Kotlin data class's `= emptyList()`
// default (the exact same landmine already documented for
// store.Asset.Tags/GroupLabel elsewhere in this codebase) - a holding
// with NO cross-member match (the common case) had a nil
// AlsoHeldByMembers, so `.isNotEmpty()` on the Kotlin side threw a
// NullPointerException the instant RecyclerView scrolled to reveal any
// such row. This test checks the actual marshaled JSON text for the
// literal substring "null" next to this field name, not just the Go
// slice's own nil-ness, since that's the level at which the real bug
// lived.
func TestComputeHoldings_AlsoHeldByMembersNeverMarshalsAsNull(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Fund With No Cross-Member Match", ISIN: "INF_UNIQUE_0003"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
		},
	}
	holdings := ComputeHoldings(p)
	out, err := json.Marshal(holdings)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), `"AlsoHeldByMembers":null`) {
		t.Errorf("AlsoHeldByMembers marshaled as JSON null - this is the confirmed real Kotlin crash, got: %s", string(out))
	}

	groups := GroupHoldingsByLabel(p, holdings)
	groupsOut, err := json.Marshal(groups)
	if err != nil {
		t.Fatalf("marshal groups: %v", err)
	}
	if strings.Contains(string(groupsOut), `"AlsoHeldByMembers":null`) {
		t.Errorf("GroupedHolding.AlsoHeldByMembers marshaled as JSON null, got: %s", string(groupsOut))
	}
}

func TestPoolHoldingsByISIN_PoolsSameISINAcrossMembersIntoOneRow(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Me"}, {ID: "m2", Name: "Mom"}},
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1"}, {ID: "acc2", MemberID: "m2"}},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Nippon India Nifty 50", ISIN: "INF_SHARED_0001"},
			{ID: "a2", AccountID: "acc2", Name: "Nippon India Nifty 50 Index Fund", ISIN: "INF_SHARED_0001"},
			{ID: "a3", AccountID: "acc1", Name: "A Fund Only I Hold", ISIN: "INF_UNIQUE_0002"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
			{ID: "t2", AccountID: "acc2", AssetID: "a2", Date: "2024-01-01", Amount: 2000, Units: &units},
			{ID: "t3", AccountID: "acc1", AssetID: "a3", Date: "2024-01-01", Amount: 500, Units: &units},
		},
	}
	holdings := ComputeHoldings(p) // whole-portfolio, unfiltered - the "All (family)" view
	pooled := PoolHoldingsByISIN(p, holdings)

	if len(pooled) != 2 {
		t.Fatalf("expected 2 rows (1 pooled + 1 unique), got %d: %+v", len(pooled), pooled)
	}

	var nifty50Row, uniqueRow *GroupedHolding
	for i := range pooled {
		if pooled[i].IsGroup {
			nifty50Row = &pooled[i]
		} else {
			uniqueRow = &pooled[i]
		}
	}
	if nifty50Row == nil {
		t.Fatal("expected one pooled (IsGroup) row for the shared ISIN")
	}
	if nifty50Row.NetInvested != 3000 {
		t.Errorf("pooled NetInvested = %v, want 3000 (1000 + 2000, combined across both members)", nifty50Row.NetInvested)
	}
	if len(nifty50Row.AssetIDs) != 2 {
		t.Errorf("pooled AssetIDs = %v, want both a1 and a2", nifty50Row.AssetIDs)
	}
	if nifty50Row.MemberName != "Family" || nifty50Row.MemberID != "" {
		t.Errorf("pooled row MemberID/MemberName = %q/%q, want empty/\"Family\" (spans more than one holder)", nifty50Row.MemberID, nifty50Row.MemberName)
	}

	if uniqueRow == nil || uniqueRow.IsGroup {
		t.Fatal("expected an ungrouped row for the unique-ISIN holding")
	}
	if uniqueRow.NetInvested != 500 {
		t.Errorf("unique row NetInvested = %v, want 500", uniqueRow.NetInvested)
	}
}

func TestPoolHoldingsByISIN_NoISINNeverPooled(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Me"}, {ID: "m2", Name: "Mom"}},
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1"}, {ID: "acc2", MemberID: "m2"}},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Manually Entered Fund A", ISIN: ""},
			{ID: "a2", AccountID: "acc2", Name: "Manually Entered Fund A", ISIN: ""}, // same NAME, but no ISIN to pool on
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
			{ID: "t2", AccountID: "acc2", AssetID: "a2", Date: "2024-01-01", Amount: 1000, Units: &units},
		},
	}
	holdings := ComputeHoldings(p)
	pooled := PoolHoldingsByISIN(p, holdings)
	if len(pooled) != 2 {
		t.Fatalf("expected 2 SEPARATE rows (no ISIN to pool on, even with matching names), got %d: %+v", len(pooled), pooled)
	}
	for _, g := range pooled {
		if g.IsGroup {
			t.Errorf("row %+v incorrectly pooled despite having no ISIN", g)
		}
	}
}

func TestPoolHoldingsByISIN_SingleHolderStillGetsCanonicalNameAndAlsoHeldBy(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Me"}},
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1"}},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Some Fund Only I Hold", ISIN: "INF_UNIQUE_0003"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-01", Amount: 1000, Units: &units},
		},
	}
	holdings := ComputeHoldings(p)
	pooled := PoolHoldingsByISIN(p, holdings)
	if len(pooled) != 1 || pooled[0].IsGroup {
		t.Fatalf("expected 1 ungrouped row, got %+v", pooled)
	}
	if pooled[0].MemberID != "m1" || pooled[0].MemberName != "Me" {
		t.Errorf("ungrouped row MemberID/MemberName = %q/%q, want m1/Me", pooled[0].MemberID, pooled[0].MemberName)
	}
	if pooled[0].DisplayName != "Some Fund Only I Hold" {
		t.Errorf("DisplayName = %q, want the CanonicalName", pooled[0].DisplayName)
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
