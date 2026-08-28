package finance

import (
	"testing"

	"ledger/internal/store"
)

func TestComputeTrailingReturn_SimplePointToPoint(t *testing.T) {
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2024-01-01", Price: 100},
		{AssetID: "nifty50", Date: "2024-01-22", Price: 110},
	}

	// A 21-day trailing window lands exactly on the 2024-01-01 price.
	got := ComputeTrailingReturn(series, 21, "3 Weeks")
	if !got.HasData {
		t.Fatalf("HasData = false, want true")
	}
	want := round2((110.0/100.0 - 1) * 100)
	if got.Percent != want {
		t.Errorf("Percent = %v, want %v", got.Percent, want)
	}
}

func TestComputeTrailingReturn_InsufficientHistoryReportsNoData(t *testing.T) {
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2024-01-20", Price: 100},
		{AssetID: "nifty50", Date: "2024-01-22", Price: 110},
	}

	// 30 days back is before the series even starts.
	got := ComputeTrailingReturn(series, 30, "1 Month")
	if got.HasData {
		t.Errorf("HasData = true, want false (series doesn't reach back 30 days)")
	}
}

func TestComputeTrailingReturn_AnchorsToLatestActualPointNotCalendarToday(t *testing.T) {
	// The confirmed bug this test guards against: if a price hasn't been
	// re-fetched "today" (i.e. the series' latest point is a few days
	// stale relative to wall-clock time), a today-anchored calculation
	// would query the SAME carried-forward price for both ends of the
	// window and silently report 0% forever. There is no `today`
	// parameter anymore specifically so this can't happen - the anchor
	// is always the series' own latest point, whatever real move that
	// represents.
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2024-01-15", Price: 100},
		{AssetID: "nifty50", Date: "2024-01-20", Price: 105}, // the latest available point - "stale" relative to any later wall-clock "today"
	}

	got := ComputeTrailingReturn(series, 1, "Day")
	if !got.HasData {
		t.Fatalf("HasData = false, want true")
	}
	if got.Percent == 0 {
		t.Errorf("Percent = 0, want a real nonzero move - this is exactly the bug being fixed")
	}
}

func TestComputeTrailingReturnForYears_Annualizes(t *testing.T) {
	// A single 3-year window: 100 -> 133.1 is exactly +10% CAGR
	// (1.1^3 = 1.331), not +33.1% simple return - confirms annualization
	// for 1Y+ trailing tenures, matching RollingReturnStats' units.
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2020-01-01", Price: 100},
		{AssetID: "nifty50", Date: "2023-01-01", Price: 133.1},
	}

	got := ComputeTrailingReturnForYears(series, 3, "3 Year")
	if !got.HasData {
		t.Fatalf("HasData = false, want true")
	}
	if got.Percent < 9.99 || got.Percent > 10.01 {
		t.Errorf("Percent = %v, want ~10.0 (annualized CAGR, not the 33.1%% simple return)", got.Percent)
	}
}

func TestComputeTrailingReturnForYears_InsufficientHistoryReportsNoData(t *testing.T) {
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2024-01-01", Price: 100},
		{AssetID: "nifty50", Date: "2024-06-01", Price: 105},
	}
	got := ComputeTrailingReturnForYears(series, 1, "1 Year")
	if got.HasData {
		t.Errorf("HasData = true, want false (only 6 months of history for a 1-year window)")
	}
}

func TestComputeRollingReturnStats_MedianMinMaxAcrossOverlappingWindows(t *testing.T) {
	// A series with two distinct available 1-year windows.
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2020-01-01", Price: 100},
		{AssetID: "nifty50", Date: "2021-01-01", Price: 110}, // window 1: 100->110, +10%
		{AssetID: "nifty50", Date: "2022-01-01", Price: 132}, // window 2: 110->132, +20%
	}

	stats := ComputeRollingReturnStats(series, 1, "1 Year")
	if !stats.HasData {
		t.Fatalf("HasData = false, want true")
	}
	// Two windows: +10% and +20% - median of two is their average.
	wantMedian := round2((10.0 + 20.0) / 2)
	if stats.Median != wantMedian {
		t.Errorf("Median = %v, want %v", stats.Median, wantMedian)
	}
	if stats.Min != 10.0 {
		t.Errorf("Min = %v, want 10.0", stats.Min)
	}
	if stats.Max != 20.0 {
		t.Errorf("Max = %v, want 20.0", stats.Max)
	}
}

func TestComputeRollingReturnStats_AnnualizesMultiYearWindows(t *testing.T) {
	// A single 3-year window: 100 -> 133.1 is exactly +10% CAGR
	// (1.1^3 = 1.331), not +33.1% simple return - confirms annualization,
	// not simple point-to-point, for a multi-year tenure.
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2020-01-01", Price: 100},
		{AssetID: "nifty50", Date: "2023-01-01", Price: 133.1},
	}

	stats := ComputeRollingReturnStats(series, 3, "3 Year")
	if !stats.HasData {
		t.Fatalf("HasData = false, want true")
	}
	if stats.Median < 9.99 || stats.Median > 10.01 {
		t.Errorf("Median = %v, want ~10.0 (annualized CAGR, not the 33.1%% simple return)", stats.Median)
	}
}

func TestComputeRollingReturnStats_InsufficientHistoryReportsNoData(t *testing.T) {
	// Only 6 months of history - no 1-year window exists yet.
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2024-01-01", Price: 100},
		{AssetID: "nifty50", Date: "2024-06-01", Price: 105},
	}

	stats := ComputeRollingReturnStats(series, 1, "1 Year")
	if stats.HasData {
		t.Errorf("HasData = true, want false (only 6 months of history for a 1-year window)")
	}
}

func TestComputeRollingReturnStats_EmptySeriesReportsNoData(t *testing.T) {
	stats := ComputeRollingReturnStats(nil, 1, "1 Year")
	if stats.HasData {
		t.Errorf("HasData = true, want false for an empty series")
	}
}

func TestComputeTrailingReturnForCustomYears_AnnualizesFractionalWindow(t *testing.T) {
	// A single 2.5-year window: 100 -> 100*(1.10^2.5) is exactly +10%
	// CAGR - confirms fractional-year annualization matches the same
	// math as ComputeTrailingReturnForYears' whole-year case. Start
	// date set well before the actual int(2.5*365.25)-day window so
	// priceOnOrBefore always finds a price at the computed start,
	// regardless of the exact day-count rounding.
	series := []store.PriceRecord{
		{AssetID: "fund1", Date: "2021-01-01", Price: 100},
		{AssetID: "fund1", Date: "2024-01-01", Price: 126.906}, // ~100*1.10^2.5 (100*1.10^2.5 = 126.906)
	}

	got := ComputeTrailingReturnForCustomYears(series, 2.5, "2.5 Year")
	if !got.HasData {
		t.Fatalf("HasData = false, want true")
	}
	if got.Percent < 9.9 || got.Percent > 10.1 {
		t.Errorf("Percent = %v, want ~10.0 (annualized CAGR)", got.Percent)
	}
}

func TestComputeTrailingReturnForCustomYears_NonPositiveYearsReportsNoData(t *testing.T) {
	series := []store.PriceRecord{
		{AssetID: "fund1", Date: "2024-01-01", Price: 100},
		{AssetID: "fund1", Date: "2024-06-01", Price: 105},
	}
	for _, years := range []float64{0, -1.5} {
		got := ComputeTrailingReturnForCustomYears(series, years, "bad")
		if got.HasData {
			t.Errorf("years=%v: HasData = true, want false", years)
		}
	}
}

func TestComputeRollingReturnStatsForCustomYears_AnnualizesFractionalWindow(t *testing.T) {
	series := []store.PriceRecord{
		{AssetID: "fund1", Date: "2021-01-01", Price: 100},
		{AssetID: "fund1", Date: "2024-01-01", Price: 126.906},
	}

	stats := ComputeRollingReturnStatsForCustomYears(series, 2.5, "2.5 Year")
	if !stats.HasData {
		t.Fatalf("HasData = false, want true")
	}
	if stats.Median < 9.9 || stats.Median > 10.1 {
		t.Errorf("Median = %v, want ~10.0", stats.Median)
	}
}

func TestComputeRollingReturnStatsForCustomYears_InsufficientHistoryReportsNoData(t *testing.T) {
	series := []store.PriceRecord{
		{AssetID: "fund1", Date: "2024-01-01", Price: 100},
		{AssetID: "fund1", Date: "2024-06-01", Price: 105},
	}
	stats := ComputeRollingReturnStatsForCustomYears(series, 2, "2 Year")
	if stats.HasData {
		t.Errorf("HasData = true, want false (only ~5 months of history for a 2-year window)")
	}
}
