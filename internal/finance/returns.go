package finance

import (
	"math"
	"sort"
	"time"

	"ledger/internal/store"
)

// TrailingReturn is a simple point-to-point % change over a fixed
// window ending today - used for the SHORT tenures (Day, 1 Month) in
// the Returns table, where a full rolling distribution (see
// RollingReturnStats) wouldn't mean much: a "rolling distribution of
// 1-day returns" is really just daily volatility, a different question
// than the multi-year "how consistent has this been" question rolling
// returns answer for longer tenures.
type TrailingReturn struct {
	Label   string
	Percent float64
	HasData bool // false if the series doesn't yet reach back to the window's start date
}

// RollingReturnStats summarizes the FULL distribution of annualized
// (CAGR) returns across every N-YEAR window in a price series' history -
// not just a single trailing figure. Concretely, for every point in the
// series that has at least N calendar years of history behind it, this
// computes the CAGR from (that point's date minus N years) to that
// point, then reports the median/min/max across every one of those
// overlapping windows. This is the standard meaning of "rolling
// returns" in Indian mutual-fund analysis - e.g. "3-year rolling
// returns" answers "no matter which 3-year period you'd picked, what
// return would you have gotten", not just "what was the return over the
// last 3 years specifically".
type RollingReturnStats struct {
	Label   string
	Median  float64 // annualized %, median across all windows
	Min     float64 // annualized %, worst window
	Max     float64 // annualized %, best window
	HasData bool    // false if the series doesn't have even ONE full N-year window yet
}

// ComputeTrailingReturn computes a simple (non-annualized) % change from
// `days` ago to today, using the series' own carry-forward price at each
// end (see priceOnOrBefore) - so a weekend/holiday gap doesn't count as
// "no data", same convention as store.Portfolio.PriceAsOf.
func ComputeTrailingReturn(series []store.PriceRecord, days int, label string, today time.Time) TrailingReturn {
	endDate := today.Format(dateLayout)
	startDate := today.AddDate(0, 0, -days).Format(dateLayout)

	endPrice, endOK := priceOnOrBefore(series, endDate)
	startPrice, startOK := priceOnOrBefore(series, startDate)
	if !endOK || !startOK || startPrice == 0 {
		return TrailingReturn{Label: label, HasData: false}
	}
	percent := (endPrice/startPrice - 1) * 100
	return TrailingReturn{Label: label, Percent: round2(percent), HasData: true}
}

// ComputeRollingReturnStats computes the full N-year rolling-return
// distribution - see RollingReturnStats' doc comment. Samples one
// window per ACTUAL data point in the series (as that window's END),
// not one per calendar day - the series only ever changes value on
// trading days anyway, so sampling every calendar day would just
// repeat the same CAGR many times over (via carry-forward) without
// adding a genuinely new window, while still costing the same lookup.
// Using every real data point as an anchor gives a dense, representative
// sample of every overlapping window that actually differs.
func ComputeRollingReturnStats(series []store.PriceRecord, years int, label string) RollingReturnStats {
	if len(series) == 0 {
		return RollingReturnStats{Label: label, HasData: false}
	}
	var cagrs []float64
	for _, end := range series {
		endDate, err := time.Parse(dateLayout, end.Date)
		if err != nil {
			continue
		}
		startDateStr := endDate.AddDate(-years, 0, 0).Format(dateLayout)
		startPrice, ok := priceOnOrBefore(series, startDateStr)
		if !ok || startPrice <= 0 || end.Price <= 0 {
			continue // this end point doesn't have a full N-year window behind it yet
		}
		cagr := (math.Pow(end.Price/startPrice, 1.0/float64(years)) - 1) * 100
		cagrs = append(cagrs, cagr)
	}
	if len(cagrs) == 0 {
		return RollingReturnStats{Label: label, HasData: false}
	}
	sort.Float64s(cagrs)
	return RollingReturnStats{
		Label:   label,
		Median:  round2(median(cagrs)),
		Min:     round2(cagrs[0]),
		Max:     round2(cagrs[len(cagrs)-1]),
		HasData: true,
	}
}

// priceOnOrBefore finds the latest price at or before `date` within a
// single already-sorted-ascending series (e.g. from
// store.Portfolio.PriceSeries) - the same carry-forward binary search as
// store.Portfolio.PriceAsOf, but operating on one already-fetched slice
// rather than the whole portfolio's index, since rolling-return
// computation calls this many times per series and the caller has
// already fetched the series once.
func priceOnOrBefore(series []store.PriceRecord, date string) (float64, bool) {
	idx := sort.Search(len(series), func(i int) bool { return series[i].Date > date })
	if idx == 0 {
		return 0, false
	}
	return series[idx-1].Price, true
}

func median(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
