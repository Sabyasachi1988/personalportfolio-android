package finance

import (
	"math"
	"sort"
	"time"

	"ledger/internal/store"
)

// TrailingReturn is a point-to-point return from N years/days before
// the series' own LATEST actual data point through that point -
// deliberately NOT anchored to literal calendar "today". Anchoring to
// today was a real, confirmed bug: if a fund's price hasn't been
// re-fetched today (very common - AMFI/exchange data usually lags by a
// day or more), both "today" and "N days before today" resolve via
// carry-forward to the EXACT SAME cached price, making the return
// compute as precisely 0% every time, which read as "today's return is
// always zero" - not a rare edge case but the default state whenever
// data isn't freshly fetched same-day. Anchoring to the series' own
// latest point instead means "the most recent real return this data can
// tell you" - if the market's already closed and NAV is in, that's
// today's move; if not, it's the last real move we know about, exactly
// as it should be. See ComputeTrailingReturn (simple, sub-year tenures)
// vs. ComputeTrailingReturnForYears (annualized, 1Y+ tenures) for which
// formula is used when.
type TrailingReturn struct {
	Label   string
	Percent float64
	HasData bool // false if the series doesn't have enough history before its own latest point
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
// last 3 years specifically". Already anchored to real data points (see
// its own loop below), so it never had the "today" bug TrailingReturn
// just got fixed for.
type RollingReturnStats struct {
	Label   string
	Median  float64 // annualized %, median across all windows
	Min     float64 // annualized %, worst window
	Max     float64 // annualized %, best window
	HasData bool    // false if the series doesn't have even ONE full N-year window yet
}

// ComputeTrailingReturn computes the SIMPLE (non-annualized) % change
// over the `days`-day window ending at the series' own latest actual
// data point - see TrailingReturn's doc comment for why "latest actual
// point", never literal calendar today. Deliberately NOT annualized,
// unlike ComputeTrailingReturnForYears below: annualizing a 1-day or
// 1-month move (compounding it as if it continued for a full year)
// produces a wild, not-actually-meaningful extrapolation - a 0.5% daily
// gain "annualizes" to over +500%, which is arithmetically correct but
// not what "today's return" means to anyone. Simple point-to-point is
// the standard convention for sub-year figures; only 1Y+ tenures get
// annualized (see ComputeTrailingReturnForYears), matching how fund
// factsheets report these.
func ComputeTrailingReturn(series []store.PriceRecord, days int, label string) TrailingReturn {
	if len(series) == 0 {
		return TrailingReturn{Label: label, HasData: false}
	}
	latest := series[len(series)-1] // PriceSeries returns records sorted ascending by date
	endDate, err := time.Parse(dateLayout, latest.Date)
	if err != nil {
		return TrailingReturn{Label: label, HasData: false}
	}
	startDateStr := endDate.AddDate(0, 0, -days).Format(dateLayout)
	startPrice, ok := priceOnOrBefore(series, startDateStr)
	if !ok || startPrice <= 0 {
		return TrailingReturn{Label: label, HasData: false}
	}
	percent := (latest.Price/startPrice - 1) * 100
	return TrailingReturn{Label: label, Percent: round2(percent), HasData: true}
}

// ComputeTrailingReturnForYears is ComputeTrailingReturn's calendar-year
// counterpart, used for the 1/3/5/10-Year tenures so the Returns table
// can show a trailing figure ALONGSIDE each tenure's rolling
// distribution (see RollingReturnStats) - "what actually happened over
// the most recent N years" next to "what a random N-year window has
// historically looked like". Same latest-actual-point anchoring as
// ComputeTrailingReturn; kept as a separate function rather than a
// days-based call (years*365) because calendar-year arithmetic
// (AddDate(-years, 0, 0)) handles leap years correctly, which a flat
// years*365 day count would drift on over a 10-year window.
func ComputeTrailingReturnForYears(series []store.PriceRecord, years int, label string) TrailingReturn {
	if len(series) == 0 {
		return TrailingReturn{Label: label, HasData: false}
	}
	latest := series[len(series)-1]
	endDate, err := time.Parse(dateLayout, latest.Date)
	if err != nil {
		return TrailingReturn{Label: label, HasData: false}
	}
	startDateStr := endDate.AddDate(-years, 0, 0).Format(dateLayout)
	startPrice, ok := priceOnOrBefore(series, startDateStr)
	if !ok || startPrice <= 0 || latest.Price <= 0 {
		return TrailingReturn{Label: label, HasData: false}
	}
	percent := (math.Pow(latest.Price/startPrice, 1.0/float64(years)) - 1) * 100
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
