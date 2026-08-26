package finance

import (
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

// buildCorrelatedSeries builds two price series over the same dates
// where fund moves exactly `multiplier`x the benchmark's daily % move -
// gives a known, exact expected Beta to assert against, rather than
// eyeballing a plausible-looking number.
func buildCorrelatedSeries(n int, multiplier float64) (fund, bench []store.PriceRecord) {
	fundPrice, benchPrice := 100.0, 100.0
	dailyMoves := []float64{0.01, -0.005, 0.02, -0.01, 0.015, -0.02, 0.005, 0.01, -0.008, 0.012}
	for i := 0; i < n; i++ {
		move := dailyMoves[i%len(dailyMoves)]
		date := dateFor(i)
		fund = append(fund, store.PriceRecord{Date: date, Price: fundPrice})
		bench = append(bench, store.PriceRecord{Date: date, Price: benchPrice})
		fundPrice *= 1 + move*multiplier
		benchPrice *= 1 + move
	}
	return fund, bench
}

func dateFor(i int) string {
	// Simple sequential dates, format matches dateLayout ("2006-01-02").
	day := 1 + i
	return "2024-01-" + padDay(day)
}

func padDay(d int) string {
	if d < 10 {
		return "0" + itoa(d)
	}
	return itoa(d)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestComputeBeta_ExactMultiplierGivesExpectedBeta(t *testing.T) {
	fund, bench := buildCorrelatedSeries(15, 1.5)
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
	fund, bench := buildCorrelatedSeries(5, 1.0)
	_, ok := ComputeBeta(fund, bench)
	if ok {
		t.Errorf("HasData = true, want false (fewer than 10 overlapping periods)")
	}
}

func TestComputeInformationRatio_IdenticalSeriesHasNoTrackingError(t *testing.T) {
	fund, bench := buildCorrelatedSeries(15, 1.0)
	// Identical returns every period -> zero excess-return stddev ->
	// undefined IR (division by zero avoided, not a real ratio).
	_, ok := ComputeInformationRatio(fund, bench)
	if ok {
		t.Errorf("HasData = true, want false (zero tracking error is undefined, not zero)")
	}
}

func TestComputeInformationRatio_ConsistentOutperformanceIsPositive(t *testing.T) {
	fund, bench := buildCorrelatedSeries(15, 1.2)
	got, ok := ComputeInformationRatio(fund, bench)
	if !ok {
		t.Fatalf("HasData = false, want true")
	}
	if got <= 0 {
		t.Errorf("InformationRatio = %v, want positive (fund consistently beats benchmark)", got)
	}
}

func TestComputeCaptureRatios_AmplifiedFundCapturesMoreBothWays(t *testing.T) {
	fund, bench := buildCorrelatedSeries(15, 1.5)
	up, down, upOK, downOK := ComputeCaptureRatios(fund, bench)
	if !upOK || !downOK {
		t.Fatalf("expected both capture ratios to have data, got upOK=%v downOK=%v", upOK, downOK)
	}
	// A 1.5x-amplified fund should show ~150% capture in both directions.
	if up < 140 || up > 160 {
		t.Errorf("UpCapture = %v, want ~150", up)
	}
	if down < 140 || down > 160 {
		t.Errorf("DownCapture = %v, want ~150", down)
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
