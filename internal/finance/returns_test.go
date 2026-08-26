package finance

import (
	"testing"
	"time"

	"ledger/internal/store"
)

func TestComputeTrailingReturn_SimplePointToPoint(t *testing.T) {
	series := []store.PriceRecord{
		{AssetID: "nifty50", Date: "2024-01-01", Price: 100},
		{AssetID: "nifty50", Date: "2024-01-22", Price: 110},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	// A 21-day trailing window lands exactly on the 2024-01-01 price.
	got := ComputeTrailingReturn(series, 21, "3 Weeks", today)
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
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	// 30 days back is before the series even starts.
	got := ComputeTrailingReturn(series, 30, "1 Month", today)
	if got.HasData {
		t.Errorf("HasData = true, want false (series doesn't reach back 30 days)")
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
