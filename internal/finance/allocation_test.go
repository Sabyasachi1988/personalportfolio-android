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

// TestGuessMarketCapSegment_BareETFTickers covers the reported bug: a
// CSV-imported ETF's Name is often a bare exchange ticker with no
// spaces at all (e.g. "NIFTYBEES"), a different naming vocabulary from
// AMFI's full scheme names - see GuessMarketCapSegment's doc comment.
func TestGuessMarketCapSegment_BareETFTickers(t *testing.T) {
	cases := map[string]string{
		"NIFTYBEES": "Large Cap",
		"niftybees": "Large Cap", // case-insensitivity, as imported verbatim from a CSV symbol column
		"GOLDBEES":  "Commodity", // already worked before this fix - "gold" substring matches with or without spaces
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

func TestGuessMarketCapSplit_BlendedNamesSplitEvenly(t *testing.T) {
	cases := map[string]map[string]float64{
		"HDFC Large & Mid Cap Fund - Direct Growth Plan":   {"Large Cap": 0.5, "Mid Cap": 0.5},
		"Kotak Large and Midcap Fund - Direct Growth Plan": {"Large Cap": 0.5, "Mid Cap": 0.5},
		"Axis Mid & Small Cap Fund - Direct Growth Plan":   {"Mid Cap": 0.5, "Small Cap": 0.5},
	}
	for name, want := range cases {
		got := GuessMarketCapSplit(name)
		if len(got) != len(want) {
			t.Fatalf("GuessMarketCapSplit(%q) = %v, want %v", name, got, want)
		}
		for label, weight := range want {
			if got[label] != weight {
				t.Errorf("GuessMarketCapSplit(%q)[%q] = %v, want %v", name, label, got[label], weight)
			}
		}
	}
}

func TestGuessMarketCapSplit_SingleBucketUnaffected(t *testing.T) {
	got := GuessMarketCapSplit("Nippon India Small Cap Fund - Direct Growth Plan")
	want := map[string]float64{"Small Cap": 1.0}
	if len(got) != 1 || got["Small Cap"] != 1.0 {
		t.Errorf("GuessMarketCapSplit(single-bucket name) = %v, want %v", got, want)
	}
}

func TestAllocationByMarketCapSegment_BlendedFundSplitsAcrossTwoBuckets(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "HDFC Large & Mid Cap Fund", HasPrice: true, CurrentValue: 1000},
	}
	slices := AllocationByMarketCapSegment(holdings, nil)
	byLabel := make(map[string]float64)
	for _, s := range slices {
		byLabel[s.Label] = s.Percent
	}
	if byLabel["Large Cap"] != 50 || byLabel["Mid Cap"] != 50 {
		t.Errorf("expected a 50/50 Large/Mid split for a blended fund, got %v", byLabel)
	}
}

// TestToSlices_OrderIsFixedAcrossManyCalls covers the reported
// "the pie chart and card keep swapping up and down" bug: Go
// deliberately randomizes map iteration order, and toSlices used to
// build its output straight from ranging over one - meaning the exact
// same underlying totals could come back in a different label order on
// every single call, with nothing in the actual portfolio data having
// changed. Running this many times catches non-determinism that a
// single call could miss just by chance.
func TestToSlices_OrderIsFixedAcrossManyCalls(t *testing.T) {
	holdings := []Holding{
		{AssetName: "NIPPON INDIA SMALL CAP FUND", HasPrice: true, CurrentValue: 100},
		{AssetName: "NIPPON INDIA GROWTH MID CAP FUND", HasPrice: true, CurrentValue: 100},
		{AssetName: "NIPPON INDIA NIFTY 50 VALUE 20 INDEX FUND", HasPrice: true, CurrentValue: 100},
		{AssetName: "NIPPON INDIA MULTI CAP FUND", HasPrice: true, CurrentValue: 100},
	}
	compositions := map[string]store.CapComposition{}

	var firstOrder []string
	for i := 0; i < 50; i++ {
		slices := AllocationByMarketCapSegment(holdings, compositions)
		order := make([]string, len(slices))
		for j, s := range slices {
			order[j] = s.Label
		}
		if i == 0 {
			firstOrder = order
			continue
		}
		if len(order) != len(firstOrder) {
			t.Fatalf("call %d: got %d slices, want %d", i, len(order), len(firstOrder))
		}
		for j := range order {
			if order[j] != firstOrder[j] {
				t.Fatalf("call %d: order = %v, want %v (order changed between calls with identical input)", i, order, firstOrder)
			}
		}
	}

	want := []string{"Large Cap", "Mid Cap", "Small Cap", "Multi Cap"}
	if len(firstOrder) != len(want) {
		t.Fatalf("order = %v, want %v", firstOrder, want)
	}
	for i := range want {
		if firstOrder[i] != want[i] {
			t.Errorf("order = %v, want %v (Large, Mid, Small before Multi/Flexi/Cash)", firstOrder, want)
		}
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
		got := EffectiveAssetClass(class, "Some Fund Name That Would Otherwise Confuse The Heuristic", "")
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
		got := EffectiveAssetClass("Other", name, "")
		if got != "Equity" {
			t.Errorf("EffectiveAssetClass(Other, %q) = %q, want Equity", name, got)
		}
	}

	if got := EffectiveAssetClass("Other", "Nippon India Gold ETF", ""); got != "Commodity" {
		t.Errorf("gold ETF under Other = %q, want Commodity", got)
	}
	if got := EffectiveAssetClass("Other", "Some Nifty G-Sec Debt Index Fund", ""); got != "Debt" {
		t.Errorf("debt index fund under Other = %q, want Debt", got)
	}
}

func TestEffectiveAssetClass_NoSignalAtAllIsUnclassified(t *testing.T) {
	got := EffectiveAssetClass("", "Totally Novel Unrecognisable Fund XYZ", "")
	if got != "Unclassified" {
		t.Errorf("expected Unclassified, got %q", got)
	}
	// Unrecognised AMFI category (not "Other", not one of the four known
	// ones, and the name doesn't match any heuristic either) should still
	// surface as-is rather than being silently discarded.
	got = EffectiveAssetClass("Some Future AMFI Category", "Totally Novel Unrecognisable Fund XYZ", "")
	if got != "Some Future AMFI Category" {
		t.Errorf("expected the unrecognised category to pass through, got %q", got)
	}
}

func TestEffectiveAssetClass_OverrideWinsOverEverything(t *testing.T) {
	// The exact real-world case this feature was built for: a fund-of-fund
	// whose name matches none of the heuristic's patterns, so it would
	// otherwise fall all the way through to "Unclassified" - the override
	// should win regardless of what AMFI class or fund name say.
	got := EffectiveAssetClass("Other", "Some Totally Unrecognisable Fund Of Fund Name", "Equity")
	if got != "Equity" {
		t.Errorf("override should win, got %q", got)
	}
	// Override should also win over an official AMFI category that would
	// otherwise pass straight through.
	got = EffectiveAssetClass("Debt", "Some Debt-Sounding Fund Name", "Equity")
	if got != "Equity" {
		t.Errorf("override should win over official AMFI category too, got %q", got)
	}
	// An invalid/unrecognized override value fails safe to normal
	// resolution rather than silently misclassifying.
	got = EffectiveAssetClass("Equity", "Some Fund", "NotARealClass")
	if got != "Equity" {
		t.Errorf("invalid override should be ignored, got %q", got)
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

func TestHoldingsInSegment_HeuristicFallbackMatchesByName(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "SOME NIFTY SMALLCAP 250 INDEX FUND", HasPrice: true},
		{AssetID: "a2", AssetName: "SOME NIFTY 50 INDEX FUND", HasPrice: true},
	}
	result := HoldingsInSegment(holdings, map[string]store.CapComposition{}, "Small Cap")
	if len(result) != 1 || result[0].AssetID != "a1" {
		t.Fatalf("expected only a1 (small cap by name) to match, got %+v", result)
	}
}

func TestHoldingsInSegment_PartialCompositionContributionStillCounts(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "SOME MULTI CAP FUND", HasPrice: true},
	}
	comp := map[string]store.CapComposition{
		// Mostly Large/Mid, but a small real Small Cap sliver too - a
		// fund like this genuinely does belong in "Small Cap holdings",
		// even though it's not primarily a small-cap fund.
		"a1": {Large: 50, Mid: 45, Small: 2, Cash: 3},
	}
	result := HoldingsInSegment(holdings, comp, "Small Cap")
	if len(result) != 1 {
		t.Fatalf("expected the fund with a nonzero Small Cap slice to be included, got %d results", len(result))
	}

	// But it should NOT show up for a segment it has zero exposure to.
	noExposure := map[string]store.CapComposition{
		"a1": {Large: 60, Mid: 40, Small: 0, Cash: 0},
	}
	result2 := HoldingsInSegment(holdings, noExposure, "Small Cap")
	if len(result2) != 0 {
		t.Fatalf("expected zero results for a segment with zero composition exposure, got %d", len(result2))
	}
}

func TestAllocationByEquityOrigin_DefaultsToIndianWhenNoComposition(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "NIPPON INDIA MULTI CAP FUND", HasPrice: true, CurrentValue: 1000},
	}
	classByAsset := map[string]string{"a1": "Equity"}

	slices := AllocationByEquityOrigin(holdings, classByAsset, nil)
	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}
	if byLabel["Indian Equity"].Value != 1000 {
		t.Errorf("Indian Equity = %+v, want 1000 (default with no composition entered)", byLabel["Indian Equity"])
	}
	if byLabel["International Equity"].Value != 0 {
		t.Errorf("International Equity = %+v, want 0", byLabel["International Equity"])
	}
}

func TestAllocationByEquityOrigin_RealCompositionOverridesDefault(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "SOME GLOBAL FUND OF FUNDS", HasPrice: true, CurrentValue: 1000},
		{AssetID: "a2", AssetName: "SOME PURE DOMESTIC FUND", HasPrice: true, CurrentValue: 500},
	}
	classByAsset := map[string]string{"a1": "Equity", "a2": "Equity"}
	compositions := map[string]store.EquityOriginComposition{
		"a1": {AssetID: "a1", Indian: 20, International: 80},
	}

	slices := AllocationByEquityOrigin(holdings, classByAsset, compositions)
	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}
	// a1: 200 Indian + 800 International. a2: 500 Indian (default).
	if byLabel["Indian Equity"].Value != 700 {
		t.Errorf("Indian Equity = %+v, want 700 (200 from a1 + 500 default from a2)", byLabel["Indian Equity"])
	}
	if byLabel["International Equity"].Value != 800 {
		t.Errorf("International Equity = %+v, want 800", byLabel["International Equity"])
	}
}

func TestAllocationByEquityOrigin_NonEquityHoldingsExcluded(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "Some Equity Fund", HasPrice: true, CurrentValue: 1000},
		{AssetID: "a2", AssetName: "Some Debt Fund", HasPrice: true, CurrentValue: 500},
	}
	classByAsset := map[string]string{"a1": "Equity", "a2": "Debt"}

	slices := AllocationByEquityOrigin(holdings, classByAsset, nil)
	total := 0.0
	for _, s := range slices {
		total += s.Value
	}
	if total != 1000 {
		t.Errorf("total = %v, want 1000 (the Debt holding must be excluded entirely)", total)
	}
}

func TestAllocationByPortfolioClass_BucketsIntoFour(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", HasPrice: true, CurrentValue: 500}, // Equity
		{AssetID: "a2", HasPrice: true, CurrentValue: 300}, // Debt
		{AssetID: "a3", HasPrice: true, CurrentValue: 100}, // Commodity
		{AssetID: "a4", HasPrice: true, CurrentValue: 50},  // Hybrid -> Others
		{AssetID: "a5", HasPrice: true, CurrentValue: 50},  // no class at all -> Unclassified -> Others
	}
	classByAsset := map[string]string{
		"a1": "Equity", "a2": "Debt", "a3": "Commodity", "a4": "Hybrid",
	}

	slices := AllocationByPortfolioClass(holdings, classByAsset)
	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}
	if byLabel["Equity"].Value != 500 {
		t.Errorf("Equity = %+v, want 500", byLabel["Equity"])
	}
	if byLabel["Debt"].Value != 300 {
		t.Errorf("Debt = %+v, want 300", byLabel["Debt"])
	}
	if byLabel["Commodity"].Value != 100 {
		t.Errorf("Commodity = %+v, want 100", byLabel["Commodity"])
	}
	if byLabel["Others"].Value != 100 {
		t.Errorf("Others = %+v, want 100 (50 Hybrid + 50 Unclassified)", byLabel["Others"])
	}
}

func TestPortfolioClassDrift_ComputesActualMinusTarget(t *testing.T) {
	actual := []AllocationSlice{
		{Label: "Equity", Percent: 70},
		{Label: "Debt", Percent: 15},
		{Label: "Commodity", Percent: 5},
		{Label: "Others", Percent: 10},
	}
	target := store.PortfolioClassTarget{Equity: 66, Debt: 20, Commodity: 5, Others: 9}

	drift := PortfolioClassDrift(actual, target)
	if len(drift) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(drift))
	}
	byLabel := make(map[string]AllocationDriftSlice)
	for _, d := range drift {
		byLabel[d.Label] = d
	}
	if got := byLabel["Equity"].Drift; got != 4 {
		t.Errorf("Equity drift = %v, want 4 (overweight)", got)
	}
	if got := byLabel["Debt"].Drift; got != -5 {
		t.Errorf("Debt drift = %v, want -5 (underweight)", got)
	}
	if got := byLabel["Commodity"].Drift; got != 0 {
		t.Errorf("Commodity drift = %v, want 0 (on target)", got)
	}
	if got := byLabel["Others"].Drift; got != 1 {
		t.Errorf("Others drift = %v, want 1 (overweight)", got)
	}
}

func TestPortfolioClassTarget_HasTargetDetection(t *testing.T) {
	if (store.PortfolioClassTarget{}).HasTarget() {
		t.Errorf("zero-value PortfolioClassTarget should report HasTarget() = false")
	}
	if !(store.PortfolioClassTarget{Commodity: 5}).HasTarget() {
		t.Errorf("a target with even one nonzero field should report HasTarget() = true")
	}
}

func TestHoldingsInSegment_UnpricedHoldingsExcluded(t *testing.T) {
	holdings := []Holding{
		{AssetID: "a1", AssetName: "SOME NIFTY LARGE CAP FUND", HasPrice: false},
	}
	result := HoldingsInSegment(holdings, map[string]store.CapComposition{}, "Large Cap")
	if len(result) != 0 {
		t.Fatalf("expected unpriced holdings to be excluded (they have no meaningful current value), got %d", len(result))
	}
}

func TestAllocationByTag(t *testing.T) {
	holdings := []Holding{
		// EffectiveTag is precomputed by ComputeHoldings in real use (see
		// asset.EffectiveTag()) - this test exercises AllocationByTag in
		// isolation, so it's set directly here as ComputeHoldings would.
		{AssetName: "Nippon India Growth Mid Cap Fund", HasPrice: true, CurrentValue: 100, EffectiveTag: "Mid Cap"},
		{AssetName: "HDFC Mid Cap Opportunities Fund", HasPrice: true, CurrentValue: 200, EffectiveTag: "Mid Cap"},
		{AssetName: "Parag Parikh Flexi Cap Fund", HasPrice: true, CurrentValue: 300, EffectiveTag: "Flexi Cap"},
		{AssetName: "Some stock with no tags yet", HasPrice: true, CurrentValue: 400, EffectiveTag: ""},
		{AssetName: "Not priced yet fund", HasPrice: false, CurrentValue: 0, EffectiveTag: "Mid Cap"},
	}
	slices := AllocationByTag(holdings)

	total := 0.0
	for _, s := range slices {
		total += s.Value
	}
	if total != 1000 {
		t.Errorf("total value = %v, want 1000 (unpriced holding must be excluded)", total)
	}

	byLabel := make(map[string]AllocationSlice)
	for _, s := range slices {
		byLabel[s.Label] = s
	}
	if byLabel["Mid Cap"].Value != 300 || byLabel["Mid Cap"].Percent != 30 {
		t.Errorf("Mid Cap slice = %+v, want value=300 percent=30 (two priced holdings summed, unpriced one excluded)", byLabel["Mid Cap"])
	}
	if byLabel["Flexi Cap"].Value != 300 || byLabel["Flexi Cap"].Percent != 30 {
		t.Errorf("Flexi Cap slice = %+v, want value=300 percent=30", byLabel["Flexi Cap"])
	}
	if byLabel["Untagged"].Value != 400 || byLabel["Untagged"].Percent != 40 {
		t.Errorf("Untagged slice = %+v, want value=400 percent=40 (empty EffectiveTag falls under Untagged)", byLabel["Untagged"])
	}
}
