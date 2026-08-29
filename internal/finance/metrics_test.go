package finance

import (
	"fmt"
	"math"
	"testing"

	"ledger/internal/store"
)

func TestComputeMaxDrawdown_TracksWorstPeakToTrough(t *testing.T) {
	series := []store.PriceRecord{
		{Date: "2024-01-01", Price: 100},
		{Date: "2024-01-02", Price: 120}, // new peak
		{Date: "2024-01-03", Price: 90},  // -25% from peak
		{Date: "2024-01-04", Price: 80},  // -33.33% from peak, worse
		{Date: "2024-01-05", Price: 110}, // recovers, but not past peak
	}
	got, ok := ComputeMaxDrawdown(series)
	if !ok {
		t.Fatalf("HasData = false, want true")
	}
	want := round2((80.0/120.0 - 1) * 100)
	if got != want {
		t.Errorf("MaxDrawdown = %v, want %v", got, want)
	}
}

func TestComputeMaxDrawdown_NeverDeclinesFromPeakReturnsZero(t *testing.T) {
	series := []store.PriceRecord{
		{Date: "2024-01-01", Price: 100},
		{Date: "2024-01-02", Price: 110},
		{Date: "2024-01-03", Price: 120},
	}
	got, ok := ComputeMaxDrawdown(series)
	if !ok || got != 0 {
		t.Errorf("MaxDrawdown = %v (ok=%v), want 0 (ok=true)", got, ok)
	}
}

// buildCorrelatedMonthlySeries builds two price series, ONE POINT PER
// CALENDAR MONTH, where the fund moves exactly `multiplier`x the
// benchmark's monthly % move - gives a known, exact expected Beta/
// Capture to assert against, rather than eyeballing a plausible-looking
// number. One point per month (not per day) matters here: ComputeBeta/
// ComputeInformationRatio/ComputeCaptureRatios now resample to
// month-end (see alignedMonthlyReturns' doc comment), so a test series
// with many same-month days would collapse to far fewer effective
// periods than `n` suggests.
func buildCorrelatedMonthlySeries(n int, multiplier float64) (fund, bench []store.PriceRecord) {
	fundPrice, benchPrice := 100.0, 100.0
	// Alternating up/down monthly moves so both ComputeCaptureRatios
	// directions get real data to work with, with a mean well above
	// assumedAnnualRiskFreeRate (~6.5%/yr, ~0.53%/mo) so Sharpe/Sortino
	// tests asserting a positive ratio aren't sensitive to exactly where
	// that constant sits.
	monthlyMoves := []float64{0.05, -0.02, 0.06, -0.03, 0.045, -0.04, 0.035, 0.04, -0.015, 0.05}
	for i := 0; i < n; i++ {
		move := monthlyMoves[i%len(monthlyMoves)]
		date := monthFor(i)
		fund = append(fund, store.PriceRecord{Date: date, Price: fundPrice})
		bench = append(bench, store.PriceRecord{Date: date, Price: benchPrice})
		fundPrice *= 1 + move*multiplier
		benchPrice *= 1 + move
	}
	return fund, bench
}

// monthFor returns the last day of month `i` (0-indexed from Jan 2020)
// in dateLayout format - a distinct calendar month per index, which is
// what buildCorrelatedMonthlySeries needs (lastPricePerMonth keeps the
// LAST record seen per month, so any day within the month works, but
// spreading across real distinct months is what actually exercises the
// monthly resampling instead of collapsing to one point).
func monthFor(i int) string {
	year := 2020 + i/12
	month := i%12 + 1
	return fmt.Sprintf("%04d-%02d-%02d", year, month, 15)
}

func TestComputeBeta_ExactMultiplierGivesExpectedBeta(t *testing.T) {
	fund, bench := buildCorrelatedMonthlySeries(18, 1.5)
	got, ok := ComputeBeta(fund, bench)
	if !ok {
		t.Fatalf("HasData = false, want true")
	}
	// Fund moves exactly 1.5x the benchmark every period, so Beta should
	// land at (very close to) 1.5.
	if got < 1.45 || got > 1.55 {
		t.Errorf("Beta = %v, want ~1.5", got)
	}
}

func TestComputeBeta_TooFewOverlappingPeriodsReportsNoData(t *testing.T) {
	fund, bench := buildCorrelatedMonthlySeries(10, 1.0)
	_, ok := ComputeBeta(fund, bench)
	if ok {
		t.Errorf("HasData = true, want false (fewer than 12 overlapping monthly periods)")
	}
}

func TestComputeInformationRatio_IdenticalSeriesHasNoTrackingError(t *testing.T) {
	fund, bench := buildCorrelatedMonthlySeries(18, 1.0)
	// Identical returns every period -> zero excess-return stddev ->
	// undefined IR (division by zero avoided, not a real ratio).
	_, ok := ComputeInformationRatio(fund, bench)
	if ok {
		t.Errorf("HasData = true, want false (zero tracking error is undefined, not zero)")
	}
}

func TestComputeInformationRatio_ConsistentOutperformanceIsPositive(t *testing.T) {
	fund, bench := buildCorrelatedMonthlySeries(18, 1.2)
	got, ok := ComputeInformationRatio(fund, bench)
	if !ok {
		t.Fatalf("HasData = false, want true")
	}
	if got <= 0 {
		t.Errorf("InformationRatio = %v, want positive (fund consistently beats benchmark)", got)
	}
}

func TestComputeCaptureRatios_AmplifiedFundCapturesMoreBothWays(t *testing.T) {
	fund, bench := buildCorrelatedMonthlySeries(18, 1.5)
	up, down, upOK, downOK := ComputeCaptureRatios(fund, bench)
	if !upOK || !downOK {
		t.Fatalf("expected both capture ratios to have data, got upOK=%v downOK=%v", upOK, downOK)
	}
	// A 1.5x-amplified fund should show materially more than 100%
	// capture in both directions - not pinned to exactly ~150% since
	// compounding a per-period 1.5x amplification across many periods
	// naturally drifts the cumulative ratio somewhat above a flat 150
	// (compounding is super-linear), but it should be clearly, robustly
	// above 100 either way.
	if up < 130 || up > 200 {
		t.Errorf("UpCapture = %v, want clearly >100, roughly 130-200", up)
	}
	if down < 130 || down > 200 {
		t.Errorf("DownCapture = %v, want clearly >100, roughly 130-200", down)
	}
}

func TestComputeCaptureRatios_TooFewMonthlyPeriodsReportsNoData(t *testing.T) {
	// Only 2 down-months in this short a series - below the new 6-month
	// minimum, so Down Capture specifically should report no data even
	// though there's enough data overall for Beta/IR (18 months, but
	// most of them up-months per the alternating pattern).
	fund, bench := buildCorrelatedMonthlySeries(8, 1.0)
	_, _, _, downOK := ComputeCaptureRatios(fund, bench)
	if downOK {
		t.Errorf("DownCapture HasData = true, want false (fewer than 6 down-months)")
	}
}

func TestComputeSharpeRatio_StrongConsistentGrowthIsPositive(t *testing.T) {
	fund, _ := buildCorrelatedMonthlySeries(18, 1.0)
	got, ok := ComputeSharpeRatio(fund)
	if !ok {
		t.Fatalf("HasData = false, want true")
	}
	if got <= 0 {
		t.Errorf("Sharpe = %v, want positive for a fund with strong average growth", got)
	}
}

func TestComputeSharpeRatio_TooFewMonthsReportsNoData(t *testing.T) {
	fund, _ := buildCorrelatedMonthlySeries(6, 1.0)
	_, ok := ComputeSharpeRatio(fund)
	if ok {
		t.Errorf("HasData = true, want false (fewer than 12 months)")
	}
}

func TestComputeSortinoRatio_HasDataWhenDownMonthsExist(t *testing.T) {
	fund, _ := buildCorrelatedMonthlySeries(18, 1.0)
	got, ok := ComputeSortinoRatio(fund)
	if !ok {
		t.Fatalf("HasData = false, want true (series has real down-months)")
	}
	if got <= 0 {
		t.Errorf("Sortino = %v, want positive for a fund with strong average growth", got)
	}
}

func TestComputeSortinoRatio_TooFewMonthsReportsNoData(t *testing.T) {
	fund, _ := buildCorrelatedMonthlySeries(6, 1.0)
	_, ok := ComputeSortinoRatio(fund)
	if ok {
		t.Errorf("HasData = true, want false (fewer than 12 months)")
	}
}

func TestComputeStandardDeviation_HigherVolatilityGivesHigherStdDev(t *testing.T) {
	steady, _ := buildCorrelatedMonthlySeries(18, 1.0)
	volatile, _ := buildCorrelatedMonthlySeries(18, 3.0) // 3x the monthly swings, same direction pattern
	steadySD, ok1 := ComputeStandardDeviation(steady)
	volatileSD, ok2 := ComputeStandardDeviation(volatile)
	if !ok1 || !ok2 {
		t.Fatalf("HasData = (%v, %v), want (true, true)", ok1, ok2)
	}
	if volatileSD <= steadySD {
		t.Errorf("volatile StdDev = %v, steady StdDev = %v - want volatile strictly higher", volatileSD, steadySD)
	}
}

func TestComputeStandardDeviation_TooFewMonthsReportsNoData(t *testing.T) {
	fund, _ := buildCorrelatedMonthlySeries(6, 1.0)
	_, ok := ComputeStandardDeviation(fund)
	if ok {
		t.Errorf("HasData = true, want false (fewer than 12 months)")
	}
}

func TestComputeAlpha_OutperformingBeyondItsBetaIsPositive(t *testing.T) {
	// Fund moves 1.0x the benchmark's swings (so Beta ~= 1, expected
	// return ~= benchmark's own return under CAPM) but starts from a
	// higher baseline compounding - constructed via an EXTRA flat
	// multiplicative bump applied every period on top of the 1:1
	// correlated moves, so its realized return exceeds what a Beta-1
	// fund should have earned. That gap is exactly what Alpha should
	// pick up as positive.
	fund, bench := buildCorrelatedMonthlySeries(18, 1.0)
	for i := range fund {
		// Compound an extra 0.5%/month of pure outperformance into the
		// fund alone, leaving the benchmark untouched - this decouples
		// "beats its own Beta-implied expectation" from "just has a
		// higher multiplier", which would also inflate Beta itself.
		fund[i].Price *= math.Pow(1.005, float64(i))
	}
	got, ok := ComputeAlpha(fund, bench)
	if !ok {
		t.Fatalf("HasData = false, want true")
	}
	if got <= 0 {
		t.Errorf("Alpha = %v, want positive for a fund with genuine outperformance beyond its Beta", got)
	}
}

func TestComputeAlpha_TooFewOverlappingPeriodsReportsNoData(t *testing.T) {
	fund, bench := buildCorrelatedMonthlySeries(10, 1.0)
	_, ok := ComputeAlpha(fund, bench)
	if ok {
		t.Errorf("HasData = true, want false (fewer than 12 overlapping monthly periods)")
	}
}

func TestDefaultBenchmarkTicker_MatchesKnownSegments(t *testing.T) {
	cases := []struct {
		fundName string
		want     string
	}{
		{"HDFC Nifty 50 Index Fund", "^NSEI"},
		{"Motilal Oswal Midcap Fund", "NIFTYMIDCAP150.NS"},
		{"SBI Small Cap Fund", "NIFTYSMLCAP250.NS"},
		{"Parag Parikh Flexi Cap Fund", "^CRSLDX"},
		{"ICICI Prudential Gold ETF", ""},
		{"Some Completely Unrecognized Fund", ""},
	}
	for _, c := range cases {
		got := DefaultBenchmarkTicker(c.fundName)
		if got != c.want {
			t.Errorf("DefaultBenchmarkTicker(%q) = %q, want %q", c.fundName, got, c.want)
		}
	}
}

func TestDefaultBenchmarkTRIName_MatchesKnownSegments(t *testing.T) {
	cases := []struct {
		fundName string
		want     string
	}{
		{"HDFC Nifty 50 Index Fund", "NIFTY 50"},
		{"Motilal Oswal Midcap Fund", "NIFTY MIDCAP 150"},
		{"SBI Small Cap Fund", "NIFTY SMALLCAP 250"},
		{"Parag Parikh Flexi Cap Fund", "NIFTY 500"},
		{"ICICI Prudential Gold ETF", ""},
		{"Some Completely Unrecognized Fund", ""},
	}
	for _, c := range cases {
		got := DefaultBenchmarkTRIName(c.fundName)
		if got != c.want {
			t.Errorf("DefaultBenchmarkTRIName(%q) = %q, want %q", c.fundName, got, c.want)
		}
	}
}