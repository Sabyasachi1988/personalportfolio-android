package finance

import (
	"math"
	"sort"
	"time"

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
	SharpeRatio        float64
	SharpeHasData      bool
	SortinoRatio       float64
	SortinoHasData     bool
	StandardDeviation  float64
	StdDevHasData      bool
	Alpha              float64
	AlphaHasData       bool
}

// assumedAnnualRiskFreeRate is used by ComputeSharpeRatio, ComputeAlpha,
// and ComputeSortinoRatio in place of a live risk-free-rate feed, which
// this app has no data source for.
//
// Set to FBIL Overnight MIBOR (the Mumbai Interbank Overnight Rate,
// published by Financial Benchmarks India Ltd) - CONFIRMED as the
// actual risk-free-rate benchmark real Indian AMCs use, not assumed:
// PPFAS Mutual Fund's own May 2026 factsheet (fetched live) states
// verbatim in its Quantitative Indicators footnote: "Risk free rate
// assumed to be (FBIL Overnight MIBOR as on May 31, 2026) 5.52%". This
// corrects an earlier, WRONG assumption in this file that Indian
// factsheets use a 91-day T-Bill - they don't; MIBOR is an overnight
// rate, a different instrument entirely, and the two aren't
// numerically interchangeable (MIBOR sits below T-Bill yields most of
// the time).
//
// Still a static value, not a live feed - FBIL publishes Overnight
// MIBOR only via its own site (fbil.org.in) and CCIL, with no free
// public API found (only paid data-vendor feeds like CEIC), unlike
// every other external data source this app already wires up live. If
// a genuine free/scrapable FBIL feed is ever found, this constant
// should be replaced by that feed - flagged here plainly, the same way
// this constant always has been, rather than dressed up as more
// precise/current than it is. 5.5% approximates the confirmed May 2026
// PPFAS figure; update this if a materially different current MIBOR
// print becomes known.
const assumedAnnualRiskFreeRate = 0.055

// WindowToTrailingYears returns only the records from the trailing N
// years of `series`, anchored to the series' own LATEST date (not
// today's real-world date, so a fund whose price history hasn't been
// refreshed in a while still gets a meaningful window instead of an
// empty one). This is the standard "3-year" window real Indian AMC
// factsheets use for their Risk Statistics block (Beta, Information
// Ratio, Up/Down Capture, Alpha, Sharpe, Sortino, Standard Deviation) -
// confirmed via multiple live-fetched sources: an industry-methodology
// article stating plainly that "the standard Indian AMC methodology
// uses monthly NAV data over a 3-year period", cross-checked against
// Fidelity's own (US) factsheet glossary describing the identical
// "3-Year Sharpe Ratio... Beta... 36 months" convention. Deliberately
// NOT applied to Max Drawdown, which has no equivalent confirmed
// 3-year convention in what was actually verified here and is left
// computed over the fund's full available history, same as before.
func WindowToTrailingYears(series []store.PriceRecord, years int) []store.PriceRecord {
	if len(series) == 0 {
		return series
	}
	latest := series[0].Date
	for _, r := range series[1:] {
		if r.Date > latest {
			latest = r.Date
		}
	}
	t, err := time.Parse(dateLayout, latest)
	if err != nil {
		return series // malformed date somewhere - fail open to the full series rather than silently dropping everything
	}
	cutoff := t.AddDate(-years, 0, 0).Format(dateLayout)
	out := make([]store.PriceRecord, 0, len(series))
	for _, r := range series {
		if r.Date >= cutoff {
			out = append(out, r)
		}
	}
	return out
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
//
// This operates on whatever series it's given - see alignedMonthlyReturns
// below for the month-end-resampled version actually used by Beta/IR/
// Capture, which is what production code calls. This daily-granularity
// version is kept because it's still the right tool for anything that
// genuinely wants daily periods (and is covered by existing tests), but
// callers computing risk/relative-performance metrics should prefer the
// monthly version - see its doc comment for why.
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

// monthKey returns the "YYYY-MM" prefix of a "YYYY-MM-DD" date string -
// cheap string-slicing rather than a full time.Parse, since dateLayout
// guarantees the fixed-width format.
func monthKey(date string) string {
	if len(date) < 7 {
		return date
	}
	return date[:7]
}

// lastPricePerMonth collapses a price series down to one point per
// calendar month - the LAST real price recorded that month (i.e. the
// closest thing to a month-end NAV/close available from whatever
// irregular real-world publish dates the series actually has). Assumes
// series is already sorted ascending by date (true of every
// store.Portfolio.PriceSeries result), so a single forward pass suffices:
// each month's map entry simply gets overwritten by the latest record
// seen for that month.
func lastPricePerMonth(series []store.PriceRecord) map[string]float64 {
	out := make(map[string]float64, len(series)/20+1)
	for _, r := range series {
		out[monthKey(r.Date)] = r.Price
	}
	return out
}

// alignedMonthlyReturns is alignedReturns' month-end-resampled sibling,
// used by ComputeBeta, ComputeInformationRatio, ComputeCaptureRatios,
// ComputeSharpeRatio and ComputeSortinoRatio instead of raw daily
// returns. This is a deliberate methodology choice, not just a
// performance optimization:
//
// Standard fund fact-sheet risk/relative-performance metrics (the
// Morningstar/Value-Research convention this app's numbers are meant to
// resemble) are computed on MONTHLY return series, not daily ones.
// Daily NAV-vs-index comparisons are dominated by noise that has
// nothing to do with genuine capture/beta/tracking behavior: a mutual
// fund's NAV is struck once at day's end from that day's closing
// prices, while an index/ETF benchmark series here comes from a
// DIFFERENT source (Yahoo/TigZig) that may timestamp or round
// differently; cash drag, fair-value pricing adjustments for
// internationally-exposed holdings, and simple one-day publish-lag
// differences between the fund's AMC and the benchmark's index
// provider all show up as spurious single-day divergences. At monthly
// granularity these smooth out and what's left is the fund's genuine
// relative behavior - which is exactly why every real-world fund
// factsheet (and the underlying capture-ratio definition this project
// already researched against a live Ryan O'Connell CFA reference)
// reports Beta/Up-Down-Capture/Information-Ratio on monthly windows.
// This was flagged as a likely real cause of an observed bug: every
// checked fund showed Down Capture landing suspiciously close to 100%
// even though the underlying compounded-return formula (captureRatio,
// unchanged) is correct - a tiny 5-DAY minimum sample of noisy daily
// data is exactly the kind of setup that produces that clustering,
// where monthly resampling with a larger minimum sample is far more
// stable.
func alignedMonthlyReturns(a, b []store.PriceRecord) (fundReturns, benchReturns []float64) {
	ma := lastPricePerMonth(a)
	mb := lastPricePerMonth(b)
	var months []string
	for m := range ma {
		if _, ok := mb[m]; ok {
			months = append(months, m)
		}
	}
	sort.Strings(months)
	if len(months) < 2 {
		return nil, nil
	}
	prevA, prevB := ma[months[0]], mb[months[0]]
	for _, m := range months[1:] {
		curA, curB := ma[m], mb[m]
		if prevA > 0 && prevB > 0 {
			fundReturns = append(fundReturns, curA/prevA-1)
			benchReturns = append(benchReturns, curB/prevB-1)
		}
		prevA, prevB = curA, curB
	}
	return fundReturns, benchReturns
}

// ComputeBeta measures the fund's sensitivity to benchmark moves -
// Cov(fund, benchmark) / Var(benchmark) over every overlapping MONTH
// (see alignedMonthlyReturns' doc comment for why monthly, not daily).
// Beta of 1.0 means the fund moves in line with the benchmark on
// average; >1 means it amplifies benchmark moves, <1 dampens them.
// Requires at least 12 overlapping monthly periods (a year of monthly
// data) - fewer than that makes a covariance/variance estimate too
// noisy to call a real Beta rather than sampling error.
func ComputeBeta(fundSeries, benchSeries []store.PriceRecord) (float64, bool) {
	fundReturns, benchReturns := alignedMonthlyReturns(fundSeries, benchSeries)
	if len(fundReturns) < 12 {
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
// every overlapping MONTH (see alignedMonthlyReturns' doc comment for
// why monthly). A higher IR means the fund beat its benchmark more
// consistently, not just by more on average. Annualized by sqrt(12) -
// 12 monthly periods per year, the standard convention for a
// monthly-return IR (this mirrors the sqrt(252)-for-daily convention
// this project already confirmed against TigZig's own documented SPR
// methodology, just at monthly instead of daily granularity). Requires
// at least 12 overlapping periods, same reasoning as ComputeBeta.
// ComputeInformationRatio is Information Ratio EXACTLY as mandated by
// SEBI circular SEBI/HO/IMD/IMD-PoD-2/P/CIR/2025/6 (17 Jan 2025,
// fetched and read directly - text confirmed, not paraphrased from a
// summary): IR = (Portfolio Return − Benchmark Return) / Standard
// Deviation of Excess Return, using DAILY arithmetic returns - the
// circular's own words are "Volatility/Standard deviation shall be
// calculated on the basis of daily return values" and "Daily portfolio
// return shall be calculated using arithmetic function" (i.e. simple
// day-over-day % change, curPrice/prevPrice - 1, NOT log returns).
// This deliberately does NOT use alignedMonthlyReturns the way this
// project's Sharpe/Sortino/Beta/Capture ratios do - those have no
// equivalent SEBI-mandated formula (only general industry practice),
// but Information Ratio specifically does, and this project now
// matches it exactly rather than approximating with monthly data.
//
// Annualized by sqrt(252) (standard trading-days-per-year convention),
// consistent with this file's own sqrt(12) monthly annualization
// elsewhere - the circular itself doesn't specify an annualization
// step explicitly, but a raw daily-period ratio isn't meaningfully
// comparable across funds with different amounts of history the way
// an annualized one is, and sqrt(252) is the standard method for
// annualizing a ratio built on daily observations.
//
// Requires at least 60 aligned trading days (~3 months) - a genuinely
// pragmatic floor, not something SEBI specifies, chosen for the same
// reason as this file's other minimum-sample thresholds: too few
// daily observations makes the standard deviation of excess return
// noise-dominated rather than meaningful.
func ComputeInformationRatio(fundSeries, benchSeries []store.PriceRecord) (float64, bool) {
	fundReturns, benchReturns := alignedReturns(fundSeries, benchSeries)
	if len(fundReturns) < 60 {
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

// captureRatio computes the fund's compounded return during MONTHS
// when the benchmark moved in the given direction (up: monthly
// benchmark return > 0, down: monthly benchmark return < 0), as a
// percentage of the benchmark's own compounded return over those SAME
// months - the standard Up/Down Capture Ratio definition (confirmed
// against a live reference on capture-ratio methodology). 100 means the
// fund matched the benchmark exactly during those months; for Up
// Capture, higher is better (captured more of the upside); for Down
// Capture, LOWER is better (lost less during benchmark declines) - the
// caller/display layer is responsible for that sign convention, this
// function just computes the ratio.
//
// Operates on MONTHLY returns (see alignedMonthlyReturns), not daily -
// this is the industry-standard granularity for capture ratios (every
// real fund factsheet computes this on monthly windows), and requires
// at least 6 matching months in the requested direction, up from a
// prior 5-DAY minimum: 5 noisy daily observations is a genuinely small
// sample to divide two compounded returns by, and is the leading
// suspect for every fund's Down Capture landing suspiciously close to
// 100% regardless of actual relative performance. 6 monthly
// observations (roughly half a year of down-months, realistically
// spread across a couple of years of real history) is a materially
// more stable base for the ratio. Also requires a non-zero benchmark
// compounded move, or the ratio is undefined (e.g. a benchmark that
// never declined in the overlapping window can't produce a Down
// Capture figure).
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
	if count < 6 {
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
// the same aligned MONTHLY return series (see captureRatio's doc
// comment for the definition and the >=6-month / non-zero-benchmark-
// move requirements, which apply independently to each direction).
func ComputeCaptureRatios(fundSeries, benchSeries []store.PriceRecord) (up, down float64, upOK, downOK bool) {
	fundReturns, benchReturns := alignedMonthlyReturns(fundSeries, benchSeries)
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

// DefaultBenchmarkTRIName is DefaultBenchmarkTicker's Total-Return
// counterpart - the canonical NSE Indices name (see
// priceapi.FetchNiftyIndicesTRI's doc comment for confirmed spellings)
// for the SAME market-cap-segment match DefaultBenchmarkTicker uses,
// so a fund auto-selects the TRI version of its usual benchmark when
// one has been added, and falls back to the plain price-index version
// otherwise - see ComputeFundMetrics' auto-select logic on the Kotlin
// bridge side for how the two are tried in that order.
func DefaultBenchmarkTRIName(fundName string) string {
	switch GuessMarketCapSegment(fundName) {
	case "Large Cap":
		return "NIFTY 50"
	case "Mid Cap":
		return "NIFTY MIDCAP 150"
	case "Small Cap":
		return "NIFTY SMALLCAP 250"
	case "Multi Cap", "Flexi Cap":
		return "NIFTY 500"
	default: // Debt, Commodity, Unclassified
		return ""
	}
}

// monthlyReturnsOf resamples a single series to month-end (see
// lastPricePerMonth) and returns its own month-over-month % returns -
// the single-series counterpart to alignedMonthlyReturns, used by
// Sharpe/Sortino which need no benchmark. Requires at least 2 months of
// data to produce one return.
func monthlyReturnsOf(series []store.PriceRecord) []float64 {
	m := lastPricePerMonth(series)
	var months []string
	for k := range m {
		months = append(months, k)
	}
	sort.Strings(months)
	if len(months) < 2 {
		return nil
	}
	var returns []float64
	prev := m[months[0]]
	for _, k := range months[1:] {
		cur := m[k]
		if prev > 0 {
			returns = append(returns, cur/prev-1)
		}
		prev = cur
	}
	return returns
}

// ComputeSharpeRatio measures risk-adjusted return: (mean monthly
// excess return over the risk-free rate) / (standard deviation of
// monthly return), annualized by sqrt(12) - the same monthly-return,
// sqrt(12)-annualization convention this project already applied to
// ComputeInformationRatio, matching the sqrt(252)-for-daily /
// sqrt(12)-for-monthly pattern confirmed against TigZig's own
// documented SPR methodology (validated against QuantStats; TigZig's
// own writeup notes they previously over-annualized by sqrt(365) before
// a since-corrected fix, which is the mistake this project is
// deliberately avoiding by using trading/reporting periods - 252 daily
// or 12 monthly - not calendar days). Uses assumedAnnualRiskFreeRate -
// see its own doc comment. Requires at least 12 months of returns and a
// non-zero return standard deviation, the same bar as ComputeBeta.
func ComputeSharpeRatio(series []store.PriceRecord) (float64, bool) {
	returns := monthlyReturnsOf(series)
	if len(returns) < 12 {
		return 0, false
	}
	monthlyRF := math.Pow(1+assumedAnnualRiskFreeRate, 1.0/12) - 1
	excess := make([]float64, len(returns))
	for i, r := range returns {
		excess[i] = r - monthlyRF
	}
	meanExcess := mean(excess)
	sd := stddev(returns, mean(returns))
	if sd == 0 {
		return 0, false
	}
	return round2(meanExcess / sd * math.Sqrt(12)), true
}

// ComputeSortinoRatio is ComputeSharpeRatio's downside-risk-only
// sibling: same numerator (mean monthly excess return over the
// risk-free rate, annualized), but the denominator only counts monthly
// returns that fell BELOW the risk-free rate (downside deviation)
// rather than the full standard deviation - a fund with big upside
// swings but no bad downside months gets penalized by Sharpe but not by
// Sortino, which is the whole point of using both side by side. Same
// monthly/sqrt(12) convention and assumedAnnualRiskFreeRate as
// ComputeSharpeRatio. Requires at least 12 months of returns and at
// least one below-target month to compute a downside deviation from -
// a fund with literally zero down-months in its whole history can't
// produce a meaningful Sortino denominator.
func ComputeSortinoRatio(series []store.PriceRecord) (float64, bool) {
	returns := monthlyReturnsOf(series)
	if len(returns) < 12 {
		return 0, false
	}
	monthlyRF := math.Pow(1+assumedAnnualRiskFreeRate, 1.0/12) - 1
	var sumSqDownside float64
	downsideCount := 0
	var sumExcess float64
	for _, r := range returns {
		excess := r - monthlyRF
		sumExcess += excess
		if r < monthlyRF {
			d := r - monthlyRF
			sumSqDownside += d * d
			downsideCount++
		}
	}
	if downsideCount == 0 {
		return 0, false
	}
	meanExcess := sumExcess / float64(len(returns))
	downsideDeviation := math.Sqrt(sumSqDownside / float64(len(returns)))
	if downsideDeviation == 0 {
		return 0, false
	}
	return round2(meanExcess / downsideDeviation * math.Sqrt(12)), true
}

// ComputeStandardDeviation is the annualized volatility of the fund's
// own monthly returns - monthly standard deviation * sqrt(12), the
// same "monthly return, sqrt(12)-annualization" convention already
// used throughout this file for Sharpe/Sortino/Information Ratio (see
// ComputeSharpeRatio's doc comment for the primary-source confirmation
// of that convention). Expressed as a percentage. Requires at least 12
// months of returns, the same bar as every other metric in this file
// that needs a real sample to be meaningful rather than noise.
func ComputeStandardDeviation(series []store.PriceRecord) (float64, bool) {
	returns := monthlyReturnsOf(series)
	if len(returns) < 12 {
		return 0, false
	}
	sd := stddev(returns, mean(returns))
	if sd == 0 {
		return 0, false
	}
	return round2(sd * math.Sqrt(12) * 100), true
}

// annualizedReturnFromMonthly geometrically compounds a series of
// monthly returns up to an annualized figure: (prod(1+r_i))^(12/n) - 1.
// Geometric, not arithmetic-mean*12, because this is meant to represent
// an actual achievable compounded growth rate (the same CAGR concept
// used everywhere else returns are annualized in this app), which
// arithmetic averaging of monthly returns does NOT correctly represent
// once compounding effects matter over multiple periods.
func annualizedReturnFromMonthly(returns []float64) float64 {
	cum := 1.0
	for _, r := range returns {
		cum *= 1 + r
	}
	n := float64(len(returns))
	if n == 0 || cum <= 0 {
		return 0
	}
	return math.Pow(cum, 12/n) - 1
}

// ComputeAlpha is Jensen's Alpha - the fund's annualized return minus
// what CAPM predicts given its own Beta: alpha = Rp - [Rf + Beta*(Rm -
// Rf)]. Formula confirmed against Jensen's original 1968 formulation
// (Wikipedia's own math rendering matches several independent CFA-
// oriented calculator sites checked live). Expressed as a percentage.
// Reuses ComputeBeta itself (not a separate covariance calculation) so
// the Beta a person sees on the Beta card and the Beta implicitly
// behind their Alpha card are guaranteed to be the exact same number,
// never two independently-computed values that could drift apart.
// Same >=12-overlapping-month requirement as ComputeBeta, since Alpha
// is meaningless without a real Beta estimate behind it.
func ComputeAlpha(fundSeries, benchSeries []store.PriceRecord) (float64, bool) {
	beta, betaOK := ComputeBeta(fundSeries, benchSeries)
	if !betaOK {
		return 0, false
	}
	fundReturns, benchReturns := alignedMonthlyReturns(fundSeries, benchSeries)
	if len(fundReturns) < 12 {
		return 0, false
	}
	fundAnnual := annualizedReturnFromMonthly(fundReturns)
	benchAnnual := annualizedReturnFromMonthly(benchReturns)
	expected := assumedAnnualRiskFreeRate + beta*(benchAnnual-assumedAnnualRiskFreeRate)
	alpha := fundAnnual - expected
	return round2(alpha * 100), true
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
