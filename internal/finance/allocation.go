package finance

import (
	"strings"

	"ledger/internal/store"
)

// AllocationSlice is one wedge of a portfolio-allocation breakdown.
type AllocationSlice struct {
	Label   string
	Value   float64
	Percent float64
}

// AllocationByAssetClass groups current value by each Asset's AssetClass
// field (the official AMFI/SEBI category, e.g. "Equity", "Debt",
// "Other" - which is where index funds and ETFs land, "Hybrid",
// "Solution Oriented"). Assets without a known class (not yet priced via
// AMFI, or a stock/ETF with no AMFI record) are grouped under "Unclassified".
// AllocationByAssetClass groups current value by each holding's *effective*
// asset class (see EffectiveAssetClass): the official AMFI/SEBI category
// where that's meaningful (Equity/Debt/Hybrid/Solution Oriented), refined
// by the fund-name heuristic specifically for AMFI's generic "Other"
// bucket, since that bucket otherwise hides whether an index fund/ETF is
// equity, debt, or commodity. Assets with no signal at all fall under
// "Unclassified".
func AllocationByAssetClass(holdings []Holding, classByAssetID map[string]string) []AllocationSlice {
	totals := make(map[string]float64)
	var total float64
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		class := EffectiveAssetClass(classByAssetID[h.AssetID], h.AssetName)
		totals[class] += h.CurrentValue
		total += h.CurrentValue
	}
	return toSlices(totals, total)
}

// EffectiveAssetClass resolves the asset class actually used for
// allocation reporting. AMFI's own category is authoritative and kept
// as-is for "Equity", "Debt", "Hybrid", and "Solution Oriented" - those
// are already correctly bucketed by AMFI itself. But AMFI's "Other"
// bucket lumps every index fund, ETF, and fund-of-fund together
// regardless of what they actually invest in, which hides exactly the
// equity/debt/commodity split someone would want to see. For that
// bucket (and for anything with no AMFI category at all, e.g. a
// manually-added stock/ETF), the fund-name heuristic
// (GuessMarketCapSegment) is used to recover the real asset class: any
// cap-size segment (Large/Mid/Small/Multi/Flexi Cap) implies Equity,
// since cap-size is an equity-only concept; Debt and Commodity segments
// map straight across.
func EffectiveAssetClass(amfiClass, fundName string) string {
	switch amfiClass {
	case "Equity", "Debt", "Hybrid", "Solution Oriented":
		return amfiClass
	}

	switch GuessMarketCapSegment(fundName) {
	case "Large Cap", "Mid Cap", "Small Cap", "Multi Cap", "Flexi Cap":
		return "Equity"
	case "Debt":
		return "Debt"
	case "Commodity":
		return "Commodity"
	}

	if amfiClass != "" {
		return amfiClass // some other AMFI category we haven't special-cased, e.g. a future new bucket
	}
	return "Unclassified"
}

// AllocationByMarketCapSegment groups current value by a heuristic
// market-cap/asset-type segment derived from each holding's fund name
// (see GuessMarketCapSegment). This is NOT an official AMFI/SEBI
// classification - AMFI groups all index funds under one generic
// "Other Scheme - Index Funds" bucket regardless of what index they
// track, so for an index-fund-heavy portfolio the official category
// alone can't tell large-cap from small-cap. This heuristic reads the
// fund name instead to answer that specific question.
// AllocationByMarketCapSegment groups current value by cap-size segment.
// Where a real, entered CapComposition exists for a holding (see
// store.CapComposition), that fund's current value is split proportionally
// across Large/Mid/Small according to the actual entered percentages -
// this is the accurate path. Where no real composition has been entered,
// it falls back to the heuristic single-bucket guess from the fund's name
// (see GuessMarketCapSegment) - reasonable for a pure single-segment index
// tracker, an approximation otherwise.
func AllocationByMarketCapSegment(holdings []Holding, compositionByAsset map[string]store.CapComposition) []AllocationSlice {
	totals := make(map[string]float64)
	var total float64
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		if comp, ok := compositionByAsset[h.AssetID]; ok {
			sum := comp.Large + comp.Mid + comp.Small + comp.Cash
			if sum > 0 {
				totals["Large Cap"] += h.CurrentValue * comp.Large / sum
				totals["Mid Cap"] += h.CurrentValue * comp.Mid / sum
				totals["Small Cap"] += h.CurrentValue * comp.Small / sum
				totals["Cash"] += h.CurrentValue * comp.Cash / sum
				total += h.CurrentValue
				continue
			}
		}
		for label, weight := range GuessMarketCapSplit(h.AssetName) {
			totals[label] += h.CurrentValue * weight
		}
		total += h.CurrentValue
	}
	return toSlices(totals, total)
}

func toSlices(totals map[string]float64, total float64) []AllocationSlice {
	var out []AllocationSlice
	for label, value := range totals {
		pct := 0.0
		if total != 0 {
			pct = value / total * 100
		}
		out = append(out, AllocationSlice{Label: label, Value: round2(value), Percent: round2(pct)})
	}
	return out
}

// GuessMarketCapSegment infers a market-cap/asset-type segment from a
// fund's name using common NSE/BSE index and category naming
// conventions. This is a heuristic, not a certified classification -
// unusual or newly-launched fund names may not match any pattern and
// will return "Unclassified" rather than a guess presented as fact.
func GuessMarketCapSegment(fundName string) string {
	n := strings.ToLower(fundName)

	switch {
	case containsAny(n, "gold", "silver"):
		return "Commodity"
	case containsAny(n, "gilt", "g-sec", "government securities", "corporate bond", "liquid fund", "overnight fund", "banking and psu", "credit risk", "dynamic bond", "money market", "debt fund", "short duration", "ultra short"):
		return "Debt"
	case containsAny(n, "smallcap", "small cap"):
		return "Small Cap"
	case containsAny(n, "midcap", "mid cap"):
		return "Mid Cap"
	case containsAny(n, "500 momentum", "nifty 500", "nifty500"):
		return "Multi Cap"
	case containsAny(n, "multicap", "multi cap"):
		return "Multi Cap"
	case containsAny(n, "flexicap", "flexi cap"):
		return "Flexi Cap"
	case containsAny(n, "next 50", "nifty 50", "nifty50", "nifty bees", "sensex", "nifty 100", "nifty100", "bluechip", "blue chip", "large cap", "largecap"):
		return "Large Cap"
	default:
		return "Unclassified"
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// GuessMarketCapSplit generalizes GuessMarketCapSegment for fund names
// that describe a BLEND of two segments (e.g. "Large & Mid Cap Fund" -
// a real, common SEBI category) rather than one clean bucket.
// GuessMarketCapSegment alone would dump such a fund entirely into
// whichever single label matched first (in practice "Mid Cap", since
// that check runs before "Large Cap"), meaningfully overstating that
// one bucket and hiding the other half entirely - this was reported as
// "only Large and Mid cards show" even when Small/Cash-heavy funds were
// also held, because a blended fund's real Large-cap half was being
// silently absorbed into Mid Cap.
//
// Returns a weight distribution summing to 1.0 (or nil for
// "Unclassified", left for the caller to handle - see
// AllocationByMarketCapSegment). This is still a heuristic 50/50 split
// for blended names, not a verified fact - real CapComposition data
// (manual entry or the ETMoney fetch) always takes precedence over this
// and should be preferred wherever accuracy matters.
func GuessMarketCapSplit(fundName string) map[string]float64 {
	n := strings.ToLower(fundName)
	switch {
	case containsAny(n, "large & mid cap", "large and mid cap", "large & midcap", "large and midcap", "large/mid cap", "large-mid cap", "largemidcap"):
		return map[string]float64{"Large Cap": 0.5, "Mid Cap": 0.5}
	case containsAny(n, "mid & small cap", "mid and small cap", "mid & smallcap", "mid and smallcap", "mid/small cap", "mid-small cap", "midsmallcap"):
		return map[string]float64{"Mid Cap": 0.5, "Small Cap": 0.5}
	}
	seg := GuessMarketCapSegment(fundName)
	if seg == "Unclassified" {
		return map[string]float64{"Unclassified": 1.0}
	}
	return map[string]float64{seg: 1.0}
}

// AllocationDriftSlice compares one market-cap bucket's actual weight
// against the person's own chosen target.
type AllocationDriftSlice struct {
	Label  string
	Actual float64
	Target float64
	Drift  float64 // Actual - Target: positive means overweight, negative underweight
}

// AllocationDrift compares AllocationByMarketCapSegment's output against
// a TargetAllocation, across the four buckets a target can be set for
// (Large/Mid/Small/Cash). Any actual allocation in a bucket outside those
// four (e.g. "Multi Cap" from the heuristic fallback, or "Debt") isn't
// covered by a target and isn't included here - CapComposition entry is
// what resolves those into the four measured buckets in the first place.
func AllocationDrift(actual []AllocationSlice, target store.TargetAllocation) []AllocationDriftSlice {
	actualByLabel := make(map[string]float64, len(actual))
	for _, a := range actual {
		actualByLabel[a.Label] = a.Percent
	}

	buckets := []struct {
		label     string
		targetPct float64
	}{
		{"Large Cap", target.Large},
		{"Mid Cap", target.Mid},
		{"Small Cap", target.Small},
		{"Cash", target.Cash},
	}

	out := make([]AllocationDriftSlice, 0, len(buckets))
	for _, b := range buckets {
		actualPct := actualByLabel[b.label]
		out = append(out, AllocationDriftSlice{
			Label:  b.label,
			Actual: round2(actualPct),
			Target: round2(b.targetPct),
			Drift:  round2(actualPct - b.targetPct),
		})
	}
	return out
}

// AllocationByEquityOrigin splits the current value of Equity-classified
// holdings (per EffectiveAssetClass) into Indian vs. International,
// using each fund's real EquityOriginComposition where entered, falling
// back to 100% Indian otherwise (see EquityOriginComposition's doc
// comment for why that default is reasonable, not a guess presented as
// fact). Non-equity holdings (Debt, Commodity, Hybrid, etc.) are not
// included at all - this answers "of my equity, how much is
// international", not "of my whole portfolio".
func AllocationByEquityOrigin(holdings []Holding, classByAssetID map[string]string, compositionByAsset map[string]store.EquityOriginComposition) []AllocationSlice {
	totals := make(map[string]float64)
	var total float64
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		if EffectiveAssetClass(classByAssetID[h.AssetID], h.AssetName) != "Equity" {
			continue
		}
		if comp, ok := compositionByAsset[h.AssetID]; ok {
			sum := comp.Indian + comp.International
			if sum > 0 {
				totals["Indian Equity"] += h.CurrentValue * comp.Indian / sum
				totals["International Equity"] += h.CurrentValue * comp.International / sum
				total += h.CurrentValue
				continue
			}
		}
		totals["Indian Equity"] += h.CurrentValue
		total += h.CurrentValue
	}
	return toSlices(totals, total)
}

// AllocationByPortfolioClass groups the current value of ALL holdings
// into four top-level buckets: Equity, Debt, Commodity, Others. Built on
// EffectiveAssetClass; anything that isn't exactly Equity/Debt/Commodity
// (Hybrid, Solution Oriented, Unclassified, or any future AMFI category)
// is folded into "Others" as a placeholder, same spirit as the
// heuristic-fallback pattern used elsewhere in this file - a real
// per-fund override can refine this later the same way CapComposition
// refines the market-cap heuristic.
func AllocationByPortfolioClass(holdings []Holding, classByAssetID map[string]string) []AllocationSlice {
	totals := make(map[string]float64)
	var total float64
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		class := EffectiveAssetClass(classByAssetID[h.AssetID], h.AssetName)
		switch class {
		case "Equity", "Debt", "Commodity":
			totals[class] += h.CurrentValue
		default:
			totals["Others"] += h.CurrentValue
		}
		total += h.CurrentValue
	}
	return toSlices(totals, total)
}

// PortfolioClassDrift compares AllocationByPortfolioClass's output
// against a PortfolioClassTarget, across all four buckets - same
// actual-minus-target comparison as AllocationDrift, generalised for
// this classification's own label set.
func PortfolioClassDrift(actual []AllocationSlice, target store.PortfolioClassTarget) []AllocationDriftSlice {
	actualByLabel := make(map[string]float64, len(actual))
	for _, a := range actual {
		actualByLabel[a.Label] = a.Percent
	}

	buckets := []struct {
		label     string
		targetPct float64
	}{
		{"Equity", target.Equity},
		{"Debt", target.Debt},
		{"Commodity", target.Commodity},
		{"Others", target.Others},
	}

	out := make([]AllocationDriftSlice, 0, len(buckets))
	for _, b := range buckets {
		actualPct := actualByLabel[b.label]
		out = append(out, AllocationDriftSlice{
			Label:  b.label,
			Actual: round2(actualPct),
			Target: round2(b.targetPct),
			Drift:  round2(actualPct - b.targetPct),
		})
	}
	return out
}

// HoldingsInSegment returns the subset of holdings that contribute any
// nonzero amount to the given market-cap segment label (Large Cap/Mid
// Cap/Small Cap/Cash, or a heuristic fallback label like "Multi Cap"),
// using the EXACT same per-holding classification as
// AllocationByMarketCapSegment - composition-split where a real
// CapComposition has been entered, the fund-name heuristic otherwise.
// Deliberately not reimplemented in Kotlin: keeping this in one place
// means the donut, the drift bars, and this filter can never disagree
// about which segment a fund belongs to.
func HoldingsInSegment(holdings []Holding, compositionByAsset map[string]store.CapComposition, segmentLabel string) []Holding {
	var out []Holding
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		if comp, ok := compositionByAsset[h.AssetID]; ok {
			sum := comp.Large + comp.Mid + comp.Small + comp.Cash
			if sum > 0 {
				var segPct float64
				switch segmentLabel {
				case "Large Cap":
					segPct = comp.Large
				case "Mid Cap":
					segPct = comp.Mid
				case "Small Cap":
					segPct = comp.Small
				case "Cash":
					segPct = comp.Cash
				}
				if segPct > 0 {
					out = append(out, h)
				}
				continue
			}
		}
		if GuessMarketCapSegment(h.AssetName) == segmentLabel {
			out = append(out, h)
		}
	}
	return out
}
