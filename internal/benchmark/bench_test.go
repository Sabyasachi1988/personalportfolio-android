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
//	Scale                    JSON size   Marshal+Unmarshal   Weekly full-range   Daily 90-day window   Weekly, REPEATED (cached, unchanged data)
//	Current (15 assets, 2y)  0.66 MB     19 ms               255 ms              255 ms                6 ms
//	Future  (30 assets, 15y) 9.97 MB     300 ms              19.4 SECONDS        4.4 seconds           119 ms
//
// Three rounds of investigation went into these numbers, in order:
//
//  1. store.PriceAsOf/FXRateAsOf were doing a full linear scan over the
//     ENTIRE Prices/FXRates slice on every single call (once per asset
//     per checkpoint) - indexed by asset/currency with a sorted,
//     binary-searched lookup instead. This gave a real but modest
//     improvement (~34% faster at future scale) - it was NOT the
//     dominant cost, which the next finding explains.
//
//  2. The actual dominant cost turned out to be XIRR: it re-solves via
//     Newton-Raphson (up to 200 iterations, each an O(len(flows)) pass)
//     independently for EVERY checkpoint, and flows grows with total
//     transaction count over time. A "warm-start each checkpoint's
//     solve from the previous checkpoint's converged rate" optimization
//     was attempted and MEASURED to make things worse, not better - it
//     was reverted rather than shipped on unverified theory. This
//     remains a known, real, unsolved cost - see XIRR's own doc comment
//     for where the O(iterations x flows) cost lives.
//
//  3. The user (Saby) pointed out that a HISTORICAL checkpoint's
//     computation is fixed in time once nothing that feeds it changes
//     again - only the checkpoint representing "today" is genuinely
//     always moving. See ProgressionCache: historical points are
//     persisted to a sidecar file keyed by a fingerprint of exactly the
//     data that feeds a progression computation (invalidated whole-sale
//     on any real change to that data, deliberately coarse rather than
//     risk a subtly-wrong surgical invalidation), while the final
//     "today" point is always recomputed fresh, never cached. This is
//     what "weekly_full_range_repeated_with_cache" measures - the
//     REPEATED-OPEN case (browse/close/reopen without any data change
//     in between), which is the actual common case for this screen.
//     Result: 19.4s -> 119ms at future scale (~163x), 255ms -> 6ms at
//     current scale (~42x) - by far the largest win of the three, and
//     directly addresses the underlying (checkpoints x transactions x
//     prices) scaling problem for the case that matters most, without
//     needing to fix XIRR's own per-call cost or restructure storage.
//
// Two things NOT fixed by any of the above, left as known open items:
//   - XIRR's per-checkpoint cost itself (finding #2) - still paid in
//     full on every CACHE-MISS computation (first open, or after any
//     data change), just no longer paid redundantly on every repeat.
//   - The bounded 90-day daily-zoom view (finding #1's original
//     motivation) is NOT cached at all - deliberately out of scope,
//     since a zoomed window's `end` isn't guaranteed to be "today" (you
//     can zoom into a purely historical stretch), so there's no
//     "which point is always-fresh" convention to apply the same way.
//     It remains bounded-and-cheap on its own (see the numbers above),
//     just not sped up further by caching.
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
			finance.ComputeProgression(p, "", finance.AxisWholePortfolio, today, nil)
		}
	})

	b.Run("daily_90day_window", func(b *testing.B) {
		start := today.AddDate(0, 0, -90)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			finance.ComputeProgressionDailyRange(p, "", finance.AxisWholePortfolio, start, today)
		}
	})

	// The scenario the progression cache is actually for: the person
	// opens Progression, browses/scrubs/zooms (no data changes), closes
	// the app, reopens it later - repeated calls against UNCHANGED data.
	// First call is a real cold-cache computation (same cost as
	// weekly_full_range above); every call after that should only ever
	// compute ONE fresh point (today) plus a cheap fingerprint check,
	// regardless of how many historical checkpoints exist.
	b.Run("weekly_full_range_repeated_with_cache", func(b *testing.B) {
		cache := finance.LoadProgressionCache(b.TempDir() + "/progression-cache.json") // empty cache, same as a fresh install
		// Prime it once so the reported b.N loop measures steady-state
		// repeated-open behavior, not the one-time cold cost.
		finance.ComputeProgression(p, "", finance.AxisWholePortfolio, today, cache)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			finance.ComputeProgression(p, "", finance.AxisWholePortfolio, today, cache)
		}
	})
}
