package finance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"ledger/internal/store"
)

// ProgressionCache persists computed HISTORICAL progression checkpoints
// across separate Bridge calls (each of which unmarshals a fresh
// *store.Portfolio with no memory of any previous call - see
// PriceAsOf/FXRateAsOf's index caching for the same underlying
// limitation at a smaller scope). Without this, every single screen
// load recomputes the ENTIRE checkpoint series from scratch, even
// though a past checkpoint's cash flows - and therefore its XIRR,
// invested amount, and value - are genuinely fixed once nothing that
// feeds into them has changed.
//
// This is keyed by a query identifier (which axis/member/asset/group
// this series is for) plus a content Fingerprint covering exactly the
// portfolio data that actually affects a progression computation
// (Assets, Transactions, Prices, FXRates, EquityOriginCompositions -
// NOT things like member names or cap-composition entries, which don't
// feed this calculation at all). ANY change to the fingerprinted data
// invalidates that query's entire cached series - this is deliberately
// coarse (whole-series invalidation, not surgical per-point
// invalidation) because it's simple enough to be confidently correct;
// a subtly-wrong surgical invalidation scheme would risk showing a
// wrong historical number, which is worse than an occasional
// unnecessary full recompute.
//
// The FINAL point of any series (whatever "today" resolves to for that
// call) is NEVER read from or written to this cache - see
// computeProgressionSeriesCached's doc comment. Only everything before
// it is genuinely fixed-in-time and safe to persist.
type ProgressionCache struct {
	Entries map[string]cachedSeries `json:"entries"`
}

type cachedSeries struct {
	Fingerprint string             `json:"fingerprint"`
	Points      []ProgressionPoint `json:"points"` // historical points only, never includes the final "today" point
}

// LoadProgressionCache reads the cache file at path, or returns an
// empty (not nil) cache if it doesn't exist yet or fails to parse -
// a missing or corrupt cache is never a hard error, just a full cache
// miss (same "fall back to computing everything" behavior as if
// caching didn't exist at all).
func LoadProgressionCache(path string) *ProgressionCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return &ProgressionCache{Entries: map[string]cachedSeries{}}
	}
	var c ProgressionCache
	if err := json.Unmarshal(data, &c); err != nil || c.Entries == nil {
		return &ProgressionCache{Entries: map[string]cachedSeries{}}
	}
	return &c
}

// Save writes the cache to path. Errors are the caller's to decide how
// to handle (this is a pure performance cache, not user data - a save
// failure should never block or corrupt anything else, but the caller
// may still want to know).
func (c *ProgressionCache) Save(path string) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// progressionFingerprint hashes exactly the portfolio data that feeds a
// progression computation - see ProgressionCache's doc comment for why
// this specific field list and not the whole portfolio.
func progressionFingerprint(p *store.Portfolio) string {
	// A struct literal (not the raw fields) so field ORDER in the
	// source doesn't matter and json.Marshal's own field order is
	// stable/deterministic (Go's encoding/json always marshals struct
	// fields in declaration order) - two calls against equal data
	// always hash identically.
	relevant := struct {
		Assets                   []store.Asset
		Transactions             []store.StoredTransaction
		Prices                   []store.PriceRecord
		FXRates                  []store.FXRate
		EquityOriginCompositions []store.EquityOriginComposition
	}{p.Assets, p.Transactions, p.Prices, p.FXRates, p.EquityOriginCompositions}

	data, err := json.Marshal(relevant)
	if err != nil {
		// Extremely unlikely (these are all plain data structs), but if
		// it ever happened, returning a fingerprint that can never match
		// a cached one just means "always recompute" - safe, not wrong.
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// computeProgressionSeriesCached is computeProgressionSeries' cache-aware
// counterpart, used by the WEEKLY (not daily-zoom-range) progression
// entry points - ComputeProgression, ComputeAssetProgression,
// ComputeGroupProgression. Daily zoomed-window queries deliberately
// don't use this: they're already bounded to ~90 days (cheap - see
// internal/benchmark), and critically, a zoomed window's `end` date
// isn't guaranteed to be "today" (you can zoom into a past stretch
// entirely), so there's no single "always-fresh final point" convention
// that would make sense there the way it does for a series that always
// runs up to today.
//
// Splits `dates` into everything-but-the-last (historical, cacheable)
// and the last date (always computed fresh, never cached or read from
// cache) - see ProgressionCache's doc comment for why the final point
// is treated specially. If the cache has a valid (fingerprint-matching)
// entry for `cacheKey` covering exactly the same historical dates,
// those are reused as-is; otherwise every historical point is computed
// fresh and the cache entry is replaced (not merged - see the coarse-
// invalidation reasoning in ProgressionCache's doc comment).
func computeProgressionSeriesCached(
	p *store.Portfolio,
	accountByID map[string]store.Account,
	assetByID map[string]store.Asset,
	included map[string]bool,
	weights map[string]float64,
	dates []string,
	cache *ProgressionCache,
	cacheKey string,
) []ProgressionPoint {
	if len(dates) == 0 {
		return nil
	}
	historicalDates := dates[:len(dates)-1]
	todayDate := dates[len(dates)-1]

	fingerprint := progressionFingerprint(p)
	var historicalPoints []ProgressionPoint

	if cached, ok := cache.Entries[cacheKey]; ok && cached.Fingerprint == fingerprint && len(cached.Points) == len(historicalDates) && datesMatch(cached.Points, historicalDates) {
		historicalPoints = cached.Points
	} else {
		historicalPoints = make([]ProgressionPoint, 0, len(historicalDates))
		for _, date := range historicalDates {
			historicalPoints = append(historicalPoints, computeProgressionPoint(p, accountByID, assetByID, included, weights, date))
		}
		cache.Entries[cacheKey] = cachedSeries{Fingerprint: fingerprint, Points: historicalPoints}
	}

	todayPoint := computeProgressionPoint(p, accountByID, assetByID, included, weights, todayDate)
	points := make([]ProgressionPoint, 0, len(dates))
	points = append(points, historicalPoints...)
	points = append(points, todayPoint)
	return points
}

// datesMatch guards against a cache entry whose Points happen to match
// in COUNT but were computed for a different set of dates (e.g. the
// requested weekly grid shifted because the portfolio's earliest
// transaction date changed) - checked in addition to the fingerprint
// and length, cheaply, before trusting a cache hit.
func datesMatch(points []ProgressionPoint, dates []string) bool {
	for i, d := range dates {
		if points[i].Date != d {
			return false
		}
	}
	return true
}
