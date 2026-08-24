// Package mobile_bench holds manual performance benchmarks - NOT run as
// part of the normal CI test suite (go test ./internal/... runs this
// package but finds zero Test functions here, which is fine and fast;
// the actual benchmarks only run when explicitly invoked with `go test
// -bench=. ./internal/benchmark/...`).
//
// RESULTS FROM THE 2026-08-24 RUN (Xeon-class sandbox CPU - a real phone
// SoC would likely be 2-4x slower single-thread, so treat these as a
// lower bound, not an on-device measurement):
//
//	Scale                    JSON size   Marshal+Unmarshal   Weekly full-range   Daily 90-day window
//	Current (15 assets, 2y)  0.66 MB     13 ms               297 ms              265 ms
//	Future  (30 assets, 15y) 9.97 MB     283 ms              29.8 SECONDS        5.4 seconds
//
// Two conclusions drawn from this:
//
//  1. The bounded 90-day daily view is NOT meaningfully more expensive
//     than what's already running today (weekly full-range) - both
//     produce a similar point count (~86-90), and computeProgressionPoint's
//     cost is dominated by per-point transaction/price scanning, not by
//     which calendar granularity picked the dates. This is why the
//     zoomed-daily-detail feature was judged safe to build.
//
//  2. The EXISTING weekly full-range view has its own, separate, more
//     urgent scaling problem: computeProgressionPoint rescans the
//     portfolio's full transaction history for every checkpoint, so its
//     cost grows with (checkpoints x transactions x prices) - at the
//     15-year/30-asset projection this hits ~30 seconds even on a fast
//     desktop CPU, which would be an on-device ANR (Android kills a
//     main-thread-blocked app after ~5s unresponsive). This is NOT
//     something the daily-view feature introduced - it already existed
//     as a latent risk. Moving Progression's computation to a background
//     thread (see ProgressionActivity) avoids the ANR crash, but doesn't
//     fix the underlying scaling - a real fix would mean either caching
//     computed points incrementally (only recompute what's new since the
//     last save) or moving off one-JSON-blob-per-load storage entirely.
//     Flagged here as a known future problem, out of scope for this
//     feature.
package mobile_bench

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"ledger/internal/finance"
	"ledger/internal/store"
)

// buildSyntheticPortfolio constructs a portfolio with `assetCount` assets,
// each with `yearsOfHistory` years of REALISTIC daily price data (247
// trading days/year, matching the real TigZig fixture density seen in
// internal/priceapi/history_test.go - not a guessed number), plus a
// modest set of transactions per asset. This mirrors what "Update Price
// History" already stores for real today - the question this benchmark
// answers is how expensive that data is to work with at portfolio-wide
// scale, not whether it can be fetched (it already is).
func buildSyntheticPortfolio(assetCount, yearsOfHistory int) *store.Portfolio {
	p := &store.Portfolio{
		Members: []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{
			{ID: "acc1", MemberID: "m1", Name: "Zerodha", Currency: "INR"},
		},
	}

	startDate := time.Date(2026-yearsOfHistory, 1, 1, 0, 0, 0, 0, time.UTC)
	tradingDaysPerYear := 247

	for a := 0; a < assetCount; a++ {
		assetID := fmt.Sprintf("asset-%d", a)
		p.Assets = append(p.Assets, store.Asset{
			ID: assetID, AccountID: "acc1",
			Name: fmt.Sprintf("Synthetic Fund %d", a), ISIN: fmt.Sprintf("INF%09d", a),
			Type: "MutualFund",
		})

		// A handful of transactions spread across the history, not one
		// per day - this matches real usage (SIPs, occasional lump
		// sums), not daily trading.
		for m := 0; m < yearsOfHistory*12; m++ {
			txnDate := startDate.AddDate(0, m, 1)
			units := 10.0
			p.Transactions = append(p.Transactions, store.StoredTransaction{
				ID: fmt.Sprintf("txn-%d-%d", a, m), AccountID: "acc1", AssetID: assetID,
				Date: txnDate.Format("2006-01-02"), Type: store.Purchase, Amount: 5000, Units: &units,
			})
		}

		// The actual bulk of the data: daily price records, exactly the
		// shape UpdateHistoricalNav/UpdateHistoricalPrice already store.
		totalDays := yearsOfHistory * tradingDaysPerYear
		price := 100.0
		d := startDate
		for i := 0; i < totalDays; i++ {
			// Skip weekends to approximate real trading-day spacing
			// (not load-bearing for the benchmark's realism beyond
			// getting the total count right, which tradingDaysPerYear
			// already anchors).
			for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
				d = d.AddDate(0, 0, 1)
			}
			p.Prices = append(p.Prices, store.PriceRecord{
				AssetID: assetID, Date: d.Format("2006-01-02"), Price: price, Source: "AMFI",
			})
			price += 0.05
			d = d.AddDate(0, 0, 1)
		}
	}

	return p
}

// Scale A: Saby's stated current real-world scale (~15 holdings, ~2
// years of history).
func BenchmarkMarshalUnmarshal_CurrentScale(b *testing.B) {
	p := buildSyntheticPortfolio(15, 2)
	benchmarkMarshalUnmarshal(b, p)
}

// Scale B: a plausible future scale per Saby's stated 15-year relocation
// horizon, with some portfolio growth (30 holdings instead of 15).
func BenchmarkMarshalUnmarshal_FutureScale15Years(b *testing.B) {
	p := buildSyntheticPortfolio(30, 15)
	benchmarkMarshalUnmarshal(b, p)
}

func benchmarkMarshalUnmarshal(b *testing.B, p *store.Portfolio) {
	data, err := json.Marshal(p)
	if err != nil {
		b.Fatalf("marshal failed: %v", err)
	}
	b.Logf("portfolio: %d assets, %d transactions, %d price records, JSON size: %.2f MB",
		len(p.Assets), len(p.Transactions), len(p.Prices), float64(len(data))/1024/1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		out, err := json.Marshal(p)
		if err != nil {
			b.Fatalf("marshal failed: %v", err)
		}
		var roundTrip store.Portfolio
		if err := json.Unmarshal(out, &roundTrip); err != nil {
			b.Fatalf("unmarshal failed: %v", err)
		}
	}
}

// Compares the existing weekly-checkpoint computation (full history)
// against a bounded ~90-day DAILY window - the actual new cost the
// zoomed-daily-view feature would add on top of what's already
// happening, at both scales.
func BenchmarkComputeProgression_CurrentScale(b *testing.B) {
	p := buildSyntheticPortfolio(15, 2)
	benchmarkProgression(b, p)
}

func BenchmarkComputeProgression_FutureScale15Years(b *testing.B) {
	p := buildSyntheticPortfolio(30, 15)
	benchmarkProgression(b, p)
}

func benchmarkProgression(b *testing.B, p *store.Portfolio) {
	today := time.Now()

	b.Run("weekly_full_range", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			finance.ComputeProgression(p, "", finance.AxisWholePortfolio, today)
		}
	})

	b.Run("daily_90day_window", func(b *testing.B) {
		start := today.AddDate(0, 0, -90)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			finance.ComputeProgressionDailyRange(p, "", finance.AxisWholePortfolio, start, today)
		}
	})
}
