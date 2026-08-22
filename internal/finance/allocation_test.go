package finance

import (
	"testing"

	"ledger/internal/store"
)

func TestGuessMarketCapSegment_RealNipponFundNames(t *testing.T) {
	cases := map[string]string{
		"NIPPON INDIA GROWTH MID CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION":        "Mid Cap",
		"NIPPON INDIA INDEX FUND - NIFTY 50 PLAN - DIRECT GROWTH PLAN GROWTH OPTION": "Large Cap",
		"NIPPON INDIA MULTI CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION":             "Multi Cap",
		"NIPPON INDIA NIFTY 50 VALUE 20 INDEX FUND - DIRECT GROWTH PLAN":             "Large Cap",
		"NIPPON INDIA NIFTY 500 MOMENTUM 50 INDEX FUND - DIRECT GROWTH PLAN":         "Multi Cap",
		"NIPPON INDIA NIFTY MIDCAP 150 INDEX FUND - DIRECT GROWTH PLAN":              "Mid Cap",
		"NIPPON INDIA NIFTY NEXT 50 JUNIOR BEES FOF - DIRECT GROWTH PLAN":            "Large Cap",
		"NIPPON INDIA NIFTY SMALLCAP 250 INDEX FUND - DIRECT GROWTH PLAN":            "Small Cap",
		"NIPPON INDIA SMALL CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION":             "Small Cap",
	}
	for name, want := range cases {
		got := GuessMarketCapSegment(name)
		if got != want {
			t.Errorf("GuessMarketCapSegment(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestGuessMarketCapSegment_DebtAndCommodity(t *testing.T) {
	cases := map[string]string{
		"HDFC Corporate Bond Fund":                      "Debt",
		"Axis Liquid Fund":                              "Debt",
		"Aditya Birla Sun Life Banking & PSU Debt Fund": "Debt",
		"Nippon India Gold Savings Fund":                "Commodity",
		"HDFC Silver ETF":                               "Commodity",
	}
	for name, want := range cases {
		got := GuessMarketCapSegment(name)
		if got != want {
			t.Errorf("GuessMarketCapSegment(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestGuessMarketCapSegment_UnknownReturnsUnclassifiedNotAGuess(t *testing.T) {
	got := GuessMarketCapSegment("Some Totally Novel Fund Name XYZ")
	if got != "Unclassified" {
		t.Errorf("expected Unclassified for an unmatched name, got %q", got)
	}
}

func TestAllocationByMarketCapSegment(t *testing.T) {
	holdings := []Holding{
		{AssetName: "NIPPON INDIA NIFTY SMALLCAP 250 INDEX FUND", HasPrice: true, CurrentValue: 100},
		{AssetName: "NIPPON INDIA NIFTY 50 VALUE 20 INDEX FUND", HasPrice: true, CurrentValue: 300},
		{AssetName: "Not priced yet fund", HasPrice: false, CurrentValue: 0},
	}
	slices := AllocationByMarketCapSegment(holdings, nil)

	total := 0.0
	for _, s := range slices {
		total += s.Value
	}
	if total != 400 {
		t.Errorf("total value = %v, want 400 (unpriced holding must be excluded)", total)
	}

	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}
	if byLabel["Small Cap"].Value != 100 || byLabel["Small Cap"].Percent != 25 {
		t.Errorf("small cap slice = %+v, want value=100 percent=25", byLabel["Small Cap"])
	}
	if byLabel["Large Cap"].Value != 300 || byLabel["Large Cap"].Percent != 75 {
		t.Errorf("large cap slice = %+v, want value=300 percent=75", byLabel["Large Cap"])
	}
}

func TestAllocationByMarketCapSegment_RealCompositionOverridesHeuristic(t *testing.T) {
	holdings := []Holding{
		// This fund's name alone would heuristically classify as 100% Mid
		// Cap, but real factsheet data (found via web search, Policybazaar,
		// ~Mar 2026) shows it's actually a mix - the real composition
		// should be used instead once entered.
		{AssetID: "growth-midcap", AssetName: "NIPPON INDIA GROWTH MID CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION", HasPrice: true, CurrentValue: 1000},
		// This one has no entered composition, so it should still fall
		// back to the heuristic.
		{AssetID: "smallcap-250", AssetName: "NIPPON INDIA NIFTY SMALLCAP 250 INDEX FUND", HasPrice: true, CurrentValue: 500},
	}
	compositions := map[string]store.CapComposition{
		"growth-midcap": {AssetID: "growth-midcap", Large: 20.24, Mid: 69.56, Small: 10.20},
	}

	slices := AllocationByMarketCapSegment(holdings, compositions)
	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}

	// From growth-midcap (1000 * pct/100): Large=202.4, Mid=695.6, Small=102.0
	// From smallcap-250 (heuristic, 100% Small): +500 to Small.
	if byLabel["Large Cap"].Value != 202.4 {
		t.Errorf("large cap = %+v, want 202.4", byLabel["Large Cap"])
	}
	if byLabel["Mid Cap"].Value != 695.6 {
		t.Errorf("mid cap = %+v, want 695.6", byLabel["Mid Cap"])
	}
	if byLabel["Small Cap"].Value != 602.0 {
		t.Errorf("small cap = %+v, want 602.0 (102.0 from growth-midcap + 500 heuristic)", byLabel["Small Cap"])
	}
}

func TestAllocationByMarketCapSegment_CashComponentIncluded(t *testing.T) {
	holdings := []Holding{
		{AssetID: "mixed-fund", AssetName: "Some Fund", HasPrice: true, CurrentValue: 1000},
	}
	compositions := map[string]store.CapComposition{
		"mixed-fund": {AssetID: "mixed-fund", Large: 60, Mid: 20, Small: 10, Cash: 10},
	}
	slices := AllocationByMarketCapSegment(holdings, compositions)
	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}
	if byLabel["Cash"].Value != 100 {
		t.Errorf("cash = %+v, want 100", byLabel["Cash"])
	}
	if byLabel["Large Cap"].Value != 600 {
		t.Errorf("large cap = %+v, want 600", byLabel["Large Cap"])
	}
}

func TestAllocationByAssetClass(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", HasPrice: true, CurrentValue: 700},
		{AssetID: "a2", HasPrice: true, CurrentValue: 300},
		{AssetID: "a3", HasPrice: true, CurrentValue: 100}, // no class entry -> Unclassified
	}
	classByAsset := map[string]string{"a1": "Equity", "a2": "Debt"}

	slices := AllocationByAssetClass(holdings, classByAsset)
	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}
	if byLabel["Equity"].Value != 700 {
		t.Errorf("equity = %+v", byLabel["Equity"])
	}
	if byLabel["Debt"].Value != 300 {
		t.Errorf("debt = %+v", byLabel["Debt"])
	}
	if byLabel["Unclassified"].Value != 100 {
		t.Errorf("unclassified = %+v", byLabel["Unclassified"])
	}
}

func TestEffectiveAssetClass_OfficialCategoriesPassThrough(t *testing.T) {
	for _, class := range []string{"Equity", "Debt", "Hybrid", "Solution Oriented"} {
		got := EffectiveAssetClass(class, "Some Fund Name That Would Otherwise Confuse The Heuristic")
		if got != class {
			t.Errorf("EffectiveAssetClass(%q, ...) = %q, want unchanged %q", class, got, class)
		}
	}
}

func TestEffectiveAssetClass_OtherBucketRefinedByFundName(t *testing.T) {
	// This is the exact real-world case: AMFI buckets all of these under
	// "Other Scheme - Index Funds" regardless of what they track, but
	// they're genuinely equity funds and should be reported as such.
	realIndexFunds := []string{
		"NIPPON INDIA INDEX FUND - NIFTY 50 PLAN - DIRECT GROWTH PLAN GROWTH OPTION",
		"NIPPON INDIA NIFTY SMALLCAP 250 INDEX FUND - DIRECT GROWTH PLAN",
		"NIPPON INDIA NIFTY MIDCAP 150 INDEX FUND - DIRECT GROWTH PLAN",
		"NIPPON INDIA NIFTY NEXT 50 JUNIOR BEES FOF - DIRECT GROWTH PLAN",
		"NIPPON INDIA NIFTY 500 MOMENTUM 50 INDEX FUND - DIRECT GROWTH PLAN",
		"NIPPON INDIA NIFTY 50 VALUE 20 INDEX FUND - DIRECT GROWTH PLAN",
	}
	for _, name := range realIndexFunds {
		got := EffectiveAssetClass("Other", name)
		if got != "Equity" {
			t.Errorf("EffectiveAssetClass(Other, %q) = %q, want Equity", name, got)
		}
	}

	if got := EffectiveAssetClass("Other", "Nippon India Gold ETF"); got != "Commodity" {
		t.Errorf("gold ETF under Other = %q, want Commodity", got)
	}
	if got := EffectiveAssetClass("Other", "Some Nifty G-Sec Debt Index Fund"); got != "Debt" {
		t.Errorf("debt index fund under Other = %q, want Debt", got)
	}
}

func TestEffectiveAssetClass_NoSignalAtAllIsUnclassified(t *testing.T) {
	got := EffectiveAssetClass("", "Totally Novel Unrecognisable Fund XYZ")
	if got != "Unclassified" {
		t.Errorf("expected Unclassified, got %q", got)
	}
	// Unrecognised AMFI category (not "Other", not one of the four known
	// ones, and the name doesn't match any heuristic either) should still
	// surface as-is rather than being silently discarded.
	got = EffectiveAssetClass("Some Future AMFI Category", "Totally Novel Unrecognisable Fund XYZ")
	if got != "Some Future AMFI Category" {
		t.Errorf("expected the unrecognised category to pass through, got %q", got)
	}
}

func TestAllocationDrift_ComputesActualMinusTarget(t *testing.T) {
	actual := []AllocationSlice{
		{Label: "Large Cap", Percent: 45},
		{Label: "Mid Cap", Percent: 25},
		{Label: "Small Cap", Percent: 20},
		{Label: "Cash", Percent: 10},
	}
	target := store.TargetAllocation{Large: 40, Mid: 33, Small: 24, Cash: 3}

	drift := AllocationDrift(actual, target)

	if len(drift) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(drift))
	}
	byLabel := make(map[string]AllocationDriftSlice)
	for _, d := range drift {
		byLabel[d.Label] = d
	}

	if got := byLabel["Large Cap"].Drift; got != 5 {
		t.Errorf("Large Cap drift = %v, want 5 (overweight)", got)
	}
	if got := byLabel["Mid Cap"].Drift; got != -8 {
		t.Errorf("Mid Cap drift = %v, want -8 (underweight)", got)
	}
	if got := byLabel["Small Cap"].Drift; got != -4 {
		t.Errorf("Small Cap drift = %v, want -4 (underweight)", got)
	}
	if got := byLabel["Cash"].Drift; got != 7 {
		t.Errorf("Cash drift = %v, want 7 (overweight)", got)
	}
}

func TestAllocationDrift_MissingActualBucketTreatedAsZero(t *testing.T) {
	// No Small Cap holdings at all yet - should show as 0% actual, not
	// be silently dropped from the comparison.
	actual := []AllocationSlice{
		{Label: "Large Cap", Percent: 70},
		{Label: "Mid Cap", Percent: 30},
	}
	target := store.TargetAllocation{Large: 40, Mid: 33, Small: 24, Cash: 3}

	drift := AllocationDrift(actual, target)

	var smallCapDrift *AllocationDriftSlice
	for i := range drift {
		if drift[i].Label == "Small Cap" {
			smallCapDrift = &drift[i]
		}
	}
	if smallCapDrift == nil {
		t.Fatalf("expected a Small Cap entry even with zero actual holdings")
	}
	if smallCapDrift.Actual != 0 {
		t.Errorf("Small Cap actual = %v, want 0", smallCapDrift.Actual)
	}
	if smallCapDrift.Drift != -24 {
		t.Errorf("Small Cap drift = %v, want -24 (fully underweight vs target)", smallCapDrift.Drift)
	}
}

func TestTargetAllocation_HasTargetDetection(t *testing.T) {
	if (store.TargetAllocation{}).HasTarget() {
		t.Errorf("zero-value TargetAllocation should report HasTarget() = false")
	}
	if !(store.TargetAllocation{Cash: 3}).HasTarget() {
		t.Errorf("a target with even one nonzero field should report HasTarget() = true")
	}
}
