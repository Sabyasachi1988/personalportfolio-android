package finance

import (
	"math"
	"sort"

	"ledger/internal/store"
)

// FundMetrics bundles the benchmark-relative and standalone risk metrics
// shown on the Returns detail screen - Beta, Information Ratio, Up/Down
// Capture (all need a benchmark series to compare against) plus Max
// Drawdown (standalone, needs only the fund's own series). Each metric
// carries its own HasData flag rather than the struct having one overall
// flag, because Max Drawdown can be computable even when a benchmark
// comparison isn't (e.g. no default benchmark match, or too few
// overlapping dates) - collapsing that into a single flag would either
// wrongly hide Max Drawdown or wrongly claim the benchmark-relative
// figures are valid.
type FundMetrics struct {
	Beta               float64
	BetaHasData        bool
	InformationRatio   float64
	InfoRatioHasData   bool
	UpCapture          float64
	UpCaptureHasData   bool
	DownCapture        float64
	DownCaptureHasData bool
	MaxDrawdown        float64
	MaxDrawdownHasData bool
}

// ComputeMaxDrawdown finds the worst peak-to-trough decline in a price
// series - the largest percentage drop from any running peak to the
// lowest point reached before a new peak is set. Standalone: needs only
// the fund's own series, no benchmark. Returned as a negative percentage
// (e.g. -23.4 for a 23.4% decline), 0 if the series never declined from
// its running peak.
func ComputeMaxDrawdown(series []store.PriceRecord) (float64, bool) {
	if len(series) == 0 {
		return 0, false
	}
	peak := series[0].Price
	maxDD := 0.0
	for _, r := range series {
		if r.Price > peak {
			peak = r.Price
		}
		if peak > 0 {
			dd := (r.Price - peak) / peak * 100
			if dd < maxDD {
				maxDD = dd
			}
		}
	}
	return round2(maxDD), true
}

// alignedReturns finds every date present in BOTH series, then computes
// period-over-period % returns across that joined, date-sorted sequence.
// Deliberately date-intersection rather than assuming both series are
// daily with no gaps - a fund's NAV history and a benchmark's fetched
// history can easily have different missing dates (holidays, fetch
// gaps), and computing a "return" between two prices that aren't
// actually adjacent trading days for BOTH series would silently
// misstate volatility/covariance. Requires at least 2 overlapping dates
// to produce even one return.
func alignedReturns(a, b []store.PriceRecord) (fundReturns, benchReturns []float64) {
	pa := map[string]float64{}
	for _, r := range a {
		pa[r.Date] = r.Price
	}
	pb := map[string]float64{}
	for _, r := range b {
		pb[r.Date] = r.Price
	}
	var dates []string
	for d := range pa {
		if _, ok := pb[d]; ok {
			dates = append(dates, d)
		}
	}
	sort.Strings(dates)
	if len(dates) < 2 {
		return nil, nil
	}
	prevA, prevB := pa[dates[0]], pb[dates[0]]
	for _, d := range dates[1:] {
		curA, curB := pa[d], pb[d]
		if prevA > 0 && prevB > 0 {
			fundReturns = append(fundReturns, curA/prevA-1)
			benchReturns = append(benchReturns, curB/prevB-1)
		}
		prevA, prevB = curA, curB
	}
	return fundReturns, benchReturns
}

// ComputeBeta measures the fund's sensitivity to benchmark moves -
// Cov(fund, benchmark) / Var(benchmark) over every overlapping period
// (see alignedReturns). Beta of 1.0 means the fund moves in line with
// the benchmark on average; >1 means it amplifies benchmark moves, <1
// dampens them. Requires at least 10 overlapping return periods -
// fewer than that makes a covariance/variance estimate too noisy to
// call a real Beta rather than sampling error.
func ComputeBeta(fundSeries, benchSeries []store.PriceRecord) (float64, bool) {
	fundReturns, benchReturns := alignedReturns(fundSeries, benchSeries)
	if len(fundReturns) < 10 {
		return 0, false
	}
	meanFund := mean(fundReturns)
	meanBench := mean(benchReturns)
	var cov, varBench float64
	for i := range fundReturns {
		df := fundReturns[i] - meanFund
		db := benchReturns[i] - meanBench
		cov += df * db
		varBench += db * db
	}
	n := float64(len(fundReturns) - 1)
	if n <= 0 || varBench == 0 {
		return 0, false
	}
	cov /= n
	varBench /= n
	return round2(cov / varBench), true
}

// ComputeInformationRatio measures excess-return CONSISTENCY relative
// to the benchmark - mean(fund-benchmark excess return) divided by the
// standard deviation of that excess return (tracking error), across
// every overlapping period (see alignedReturns). A higher IR means the
// fund beat its benchmark more consistently, not just by more on
// average. Annualized by sqrt(252) - Indian mutual fund/benchmark NAV
// history is published on trading days, so overlapping periods are
// approximately daily even though alignedReturns doesn't enforce exact
// daily spacing (gaps are simply skipped, not filled) - 252 is the
// standard trading-day-per-year convention, same one used implicitly
// throughout Indian fund-factsheet risk metrics. Requires at least 10
// overlapping periods, same reasoning as ComputeBeta.
func ComputeInformationRatio(fundSeries, benchSeries []store.PriceRecord) (float64, bool) {
	fundReturns, benchReturns := alignedReturns(fundSeries, benchSeries)
	if len(fundReturns) < 10 {
		return 0, false
	}
	excess := make([]float64, len(fundReturns))
	for i := range fundReturns {
		excess[i] = fundReturns[i] - benchReturns[i]
	}
	meanExcess := mean(excess)
	sd := stddev(excess, meanExcess)
	if sd == 0 {
		return 0, false
	}
	ir := meanExcess / sd * math.Sqrt(252)
	return round2(ir), true
}

// captureRatio computes the fund's compounded return during periods
// when the benchmark moved in the given direction (up: benchmark return
// > 0, down: benchmark return < 0), as a percentage of the benchmark's
// own compounded return over those SAME periods - the standard
// Up/Down Capture Ratio definition. 100 means the fund matched the
// benchmark exactly during those periods; for Up Capture, higher is
// better (captured more of the upside); for Down Capture, LOWER is
// better (lost less during benchmark declines) - the caller/display
// layer is responsible for that sign convention, this function just
// computes the ratio. Requires at least 5 matching periods in the
// requested direction and a non-zero benchmark compounded move, or the
// ratio is undefined (e.g. a benchmark that never declined in the
// overlapping window can't produce a Down Capture figure).
func captureRatio(fundReturns, benchReturns []float64, up bool) (float64, bool) {
	fundCum := 1.0
	benchCum := 1.0
	count := 0
	for i := range benchReturns {
		if (up && benchReturns[i] > 0) || (!up && benchReturns[i] < 0) {
			fundCum *= 1 + fundReturns[i]
			benchCum *= 1 + benchReturns[i]
			count++
		}
	}
	if count < 5 {
		return 0, false
	}
	benchTotal := benchCum - 1
	if benchTotal == 0 {
		return 0, false
	}
	fundTotal := fundCum - 1
	return round2(fundTotal / benchTotal * 100), true
}

// ComputeCaptureRatios returns both Up and Down Capture in one pass over
// the same aligned return series (see captureRatio's doc comment for the
// definition and the >=5-period / non-zero-benchmark-move requirements,
// which apply independently to each direction).
func ComputeCaptureRatios(fundSeries, benchSeries []store.PriceRecord) (up, down float64, upOK, downOK bool) {
	fundReturns, benchReturns := alignedReturns(fundSeries, benchSeries)
	up, upOK = captureRatio(fundReturns, benchReturns, true)
	down, downOK = captureRatio(fundReturns, benchReturns, false)
	return up, down, upOK, downOK
}

// DefaultBenchmarkTicker suggests which of the app's known/quick-add
// benchmark indices (see BenchmarksActivity.knownIndices on the Kotlin
// side, mirrored here) best matches a fund by market-cap classification,
// reusing GuessMarketCapSegment - the SAME heuristic already powering
// the Allocation screen's Market Cap view, rather than inventing a
// second, possibly-inconsistent classification just for this. Returns
// empty string for segments with no sensible equity-index match (Debt,
// Commodity, Unclassified) - the caller/UI should fall back to "no
// default, pick manually" rather than force a nonsensical equity
// comparison onto a debt or gold fund.
func DefaultBenchmarkTicker(fundName string) string {
	switch GuessMarketCapSegment(fundName) {
	case "Large Cap":
		return "^NSEI" // Nifty 50
	case "Mid Cap":
		return "NIFTYMIDCAP150.NS" // Nifty Midcap 150
	case "Small Cap":
		return "NIFTYSMLCAP250.NS" // Nifty Smallcap 250
	case "Multi Cap", "Flexi Cap":
		return "^CRSLDX" // Nifty 500
	default: // Debt, Commodity, Unclassified
		return ""
	}
}

func mean(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func stddev(xs []float64, m float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	var sumSq float64
	for _, x := range xs {
		d := x - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(xs)-1))
}
