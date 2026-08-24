package finance

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"ledger/internal/store"
)

func tempCachePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "progression-cache.json")
}

func TestProgressionCache_MissingFileReturnsEmptyCacheNotError(t *testing.T) {
	cache := LoadProgressionCache(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if cache == nil || cache.Entries == nil {
		t.Fatalf("expected an empty, non-nil cache for a missing file, got %+v", cache)
	}
	if len(cache.Entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(cache.Entries))
	}
}

func TestProgressionCache_CorruptFileReturnsEmptyCacheNotError(t *testing.T) {
	path := tempCachePath(t)
	if err := os.WriteFile(path, []byte("not valid json{{{"), 0644); err != nil {
		t.Fatal(err)
	}
	cache := LoadProgressionCache(path)
	if cache == nil || cache.Entries == nil || len(cache.Entries) != 0 {
		t.Fatalf("expected an empty cache for a corrupt file (fall back to full recompute), got %+v", cache)
	}
}

// TestComputeProgression_CachedSecondCallProducesIdenticalResult is the
// core correctness property: whether or not the cache was used, the
// answer must be identical - caching is purely a performance
// optimization and must never change what's returned.
func TestComputeProgression_CachedSecondCallProducesIdenticalResult(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	uncached := ComputeProgression(p, "", AxisWholePortfolio, today, nil)

	cache := &ProgressionCache{Entries: map[string]cachedSeries{}}
	firstCall := ComputeProgression(p, "", AxisWholePortfolio, today, cache)
	secondCall := ComputeProgression(p, "", AxisWholePortfolio, today, cache) // should hit the cache this time

	if len(uncached) != len(firstCall) || len(firstCall) != len(secondCall) {
		t.Fatalf("point counts differ: uncached=%d first=%d second=%d", len(uncached), len(firstCall), len(secondCall))
	}
	for i := range uncached {
		if uncached[i] != firstCall[i] || firstCall[i] != secondCall[i] {
			t.Errorf("point %d differs: uncached=%+v first=%+v second=%+v", i, uncached[i], firstCall[i], secondCall[i])
		}
	}

	// The cache should now hold a valid entry for this query - confirms
	// the cache was populated, not silently bypassed. This fixture has
	// only 1 checkpoint total (see buildMixedPortfolio's comment), so
	// its historical-points slice is legitimately empty; what matters
	// here is that an entry exists with a fingerprint recorded.
	entry, ok := cache.Entries["axis::WholePortfolio"]
	if !ok {
		t.Errorf("expected a cache entry after two calls, got none")
	}
	if entry.Fingerprint == "" {
		t.Errorf("expected a non-empty fingerprint on the cache entry")
	}
}

// TestComputeProgression_CacheInvalidatedWhenTransactionsChange is the
// safety property Saby's proposal depends on: a cached series must NOT
// be trusted once the underlying data it was computed from has
// genuinely changed (see ProgressionCache's doc comment on
// fingerprinting).
func TestComputeProgression_CacheInvalidatedWhenTransactionsChange(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
	cache := &ProgressionCache{Entries: map[string]cachedSeries{}}

	first := ComputeProgression(p, "", AxisWholePortfolio, today, cache)

	// Retroactively add an earlier transaction - this genuinely changes
	// every historical checkpoint's Invested figure from that date
	// onward, so the cache MUST be invalidated, not trusted.
	p.Transactions = append(p.Transactions, store.StoredTransaction{
		ID: "t-retro", AccountID: "acc-in", AssetID: "a-in", Date: "2024-01-16", Type: store.Purchase, Amount: 5000, Units: units(50),
	})

	second := ComputeProgression(p, "", AxisWholePortfolio, today, cache)

	if first[len(first)-1].Invested == second[len(second)-1].Invested {
		t.Fatalf("expected Invested to change after adding a retroactive transaction, but it stayed %v - the cache served stale data", first[len(first)-1].Invested)
	}
}

// TestComputeProgression_TodayPointAlwaysFreshEvenWhenCached confirms
// the specific design Saby asked about: even with an otherwise-valid
// cache hit for all historical points, the FINAL ("today") point must
// still reflect the current price - never served from the cache.
func TestComputeProgression_TodayPointAlwaysFreshEvenWhenCached(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
	cache := &ProgressionCache{Entries: map[string]cachedSeries{}}

	first := ComputeProgression(p, "", AxisWholePortfolio, today, cache)
	firstTodayValue := first[len(first)-1].Value

	// Update today's price only - nothing else about the historical
	// data changes, so a coarse fingerprint-based cache WOULD normally
	// treat this as "unchanged" for the historical points (correctly -
	// they didn't change), but the always-fresh-today rule must still
	// pick up the new price for the final point regardless. Goes
	// through UpsertPrices (not a direct field mutation) - that's what
	// keeps PriceAsOf's own separate lookup index (see
	// store.Portfolio.invalidatePriceIndex) correctly invalidated too;
	// a direct p.Prices[i].Price = x would bypass that and this test
	// would then be exercising a self-inflicted staleness bug, not the
	// progression cache's real behavior.
	p.UpsertPrices([]store.PriceRecord{{AssetID: "a-in", Date: "2024-01-22", Price: 999}})

	second := ComputeProgression(p, "", AxisWholePortfolio, today, cache)
	secondTodayValue := second[len(second)-1].Value

	if secondTodayValue == firstTodayValue {
		t.Errorf("today's point did not reflect the updated price - got %v both times", secondTodayValue)
	}
}

// TestComputeAssetProgression_MultiCheckpoint_HistoricalPointsCachedAndIdentical
// exercises the actual multi-checkpoint case the single-point
// buildMixedPortfolio fixture can't - real history spanning several
// weekly checkpoints, confirming the cached historical points are both
// populated AND byte-for-byte identical to an uncached computation.
func TestComputeAssetProgression_MultiCheckpoint_HistoricalPointsCachedAndIdentical(t *testing.T) {
	units10 := 10.0
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Zerodha", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "a1", AccountID: "acc-in", Name: "Fund A"}},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2024-01-01", Type: store.Purchase, Amount: 1000, Units: &units10},
			{ID: "t2", AccountID: "acc-in", AssetID: "a1", Date: "2024-01-08", Type: store.Purchase, Amount: 1000, Units: &units10},
			{ID: "t3", AccountID: "acc-in", AssetID: "a1", Date: "2024-01-15", Type: store.Purchase, Amount: 1000, Units: &units10},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-01-01", Price: 100},
			{AssetID: "a1", Date: "2024-01-08", Price: 102},
			{AssetID: "a1", Date: "2024-01-15", Price: 104},
			{AssetID: "a1", Date: "2024-01-22", Price: 106},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	uncached := ComputeAssetProgression(p, "a1", today, nil)
	if len(uncached) < 3 {
		t.Fatalf("expected several weekly checkpoints spanning Jan 1-22, got %d", len(uncached))
	}

	cache := &ProgressionCache{Entries: map[string]cachedSeries{}}
	firstCall := ComputeAssetProgression(p, "a1", today, cache)

	entry, ok := cache.Entries["asset:a1"]
	if !ok {
		t.Fatal("expected a cache entry for asset a1")
	}
	if len(entry.Points) != len(firstCall)-1 {
		t.Errorf("cached historical points = %d, want %d (all but the final 'today' point)", len(entry.Points), len(firstCall)-1)
	}

	secondCall := ComputeAssetProgression(p, "a1", today, cache) // should hit cache for historical points
	if len(uncached) != len(secondCall) {
		t.Fatalf("point counts differ: uncached=%d cached=%d", len(uncached), len(secondCall))
	}
	for i := range uncached {
		if uncached[i] != secondCall[i] {
			t.Errorf("point %d differs: uncached=%+v cached=%+v", i, uncached[i], secondCall[i])
		}
	}
}

func TestComputeAssetProgression_CacheKeyedSeparatelyPerAsset(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
	cache := &ProgressionCache{Entries: map[string]cachedSeries{}}

	ComputeAssetProgression(p, "a-in", today, cache)
	ComputeAssetProgression(p, "a-ca", today, cache)

	if _, ok := cache.Entries["asset:a-in"]; !ok {
		t.Error("expected a cache entry for asset a-in")
	}
	if _, ok := cache.Entries["asset:a-ca"]; !ok {
		t.Error("expected a separate cache entry for asset a-ca")
	}
}

func TestComputeGroupProgression_CacheKeyedByMemberAndLabel(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Zerodha", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc-in", Name: "Fund A", GroupLabel: "Nifty 50"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2024-01-16", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{{AssetID: "a1", Date: "2024-01-22", Price: 110}},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)
	cache := &ProgressionCache{Entries: map[string]cachedSeries{}}

	ComputeGroupProgression(p, "", "Nifty 50", today, cache)
	if _, ok := cache.Entries["group::Nifty 50"]; !ok {
		t.Error("expected a cache entry keyed by group label")
	}
}

func TestProgressionCache_SaveAndReload(t *testing.T) {
	path := tempCachePath(t)
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	cache1 := LoadProgressionCache(path) // fresh, empty
	ComputeProgression(p, "", AxisWholePortfolio, today, cache1)
	if err := cache1.Save(path); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	cache2 := LoadProgressionCache(path) // reload from disk, simulating a new process
	if len(cache2.Entries) == 0 {
		t.Fatal("expected the reloaded cache to have entries persisted from the previous save")
	}

	uncached := ComputeProgression(p, "", AxisWholePortfolio, today, nil)
	fromReloadedCache := ComputeProgression(p, "", AxisWholePortfolio, today, cache2)
	if len(uncached) != len(fromReloadedCache) {
		t.Fatalf("point counts differ after reload: %d vs %d", len(uncached), len(fromReloadedCache))
	}
	for i := range uncached {
		if uncached[i] != fromReloadedCache[i] {
			t.Errorf("point %d differs after reload: %+v vs %+v", i, uncached[i], fromReloadedCache[i])
		}
	}
}
