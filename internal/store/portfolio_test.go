package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portfolio.json")

	units := 5.409
	p := &Portfolio{
		Members:  []Member{{ID: "m1", Name: "Saby"}},
		Accounts: []Account{{ID: "a1", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets:   []Asset{{ID: "as1", AccountID: "a1", Name: "Nippon India Growth Mid Cap Fund", ISIN: "INF204K01E54", Type: "MutualFund"}},
		Transactions: []StoredTransaction{{
			ID: "t1", AccountID: "a1", AssetID: "as1", Date: "2025-07-01",
			Type: Purchase, Amount: 24998.75, Units: &units, Source: "CAS_IMPORT",
		}},
	}

	if err := Save(path, p); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Members) != 1 || loaded.Members[0].Name != "Saby" {
		t.Errorf("members not round-tripped: %+v", loaded.Members)
	}
	if len(loaded.Transactions) != 1 || loaded.Transactions[0].Units == nil || *loaded.Transactions[0].Units != 5.409 {
		t.Errorf("transaction units not round-tripped: %+v", loaded.Transactions)
	}
}

func TestLoadNormalizesNilTagsToEmptySlice(t *testing.T) {
	// Simulates a portfolio.json saved before Asset.Tags existed - no
	// "Tags" key at all for the asset, exactly the scenario Load() must
	// guard against (see its doc comment: a nil slice re-marshals to
	// JSON `null`, which is the same Gson-unsafe-allocation crash
	// GroupLabel/ETMoneyURL already hit once for a missing key).
	dir := t.TempDir()
	path := filepath.Join(dir, "portfolio.json")
	oldFormatJSON := `{"Assets":[{"ID":"as1","AccountID":"a1","Name":"Nippon India Growth Mid Cap Fund","ISIN":"INF204K01E54","Type":"MutualFund"}]}`
	if err := os.WriteFile(path, []byte(oldFormatJSON), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Assets[0].Tags == nil {
		t.Fatalf("Tags is nil after Load, want normalized empty slice")
	}
	if len(loaded.Assets[0].Tags) != 0 {
		t.Errorf("Tags = %v, want empty", loaded.Assets[0].Tags)
	}

	out, err := json.Marshal(loaded)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(out), `"Tags":null`) {
		t.Errorf("marshaled portfolio still contains \"Tags\":null - the exact landmine this normalization exists to prevent: %s", out)
	}
}

func TestAssetEffectiveTag(t *testing.T) {
	cases := []struct {
		name       string
		tags       []string
		primaryTag string
		want       string
	}{
		{"no tags at all", nil, "", ""},
		{"single tag, no override", []string{"Mid Cap"}, "", "Mid Cap"},
		{"several tags, no override falls back to first (insertion order)", []string{"Mid Cap", "Growth", "Long Term"}, "", "Mid Cap"},
		{"override present in tags wins over first", []string{"Mid Cap", "Growth", "Long Term"}, "Growth", "Growth"},
		{"stale override no longer in tags falls back to first", []string{"Mid Cap", "Growth"}, "Long Term", "Mid Cap"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := Asset{Tags: c.tags, PrimaryTag: c.primaryTag}
			if got := a.EffectiveTag(); got != c.want {
				t.Errorf("EffectiveTag() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSetAssetTagsAndAllTags(t *testing.T) {
	p := &Portfolio{
		Assets: []Asset{
			{ID: "as1", Name: "Nippon India Growth Mid Cap Fund"},
			{ID: "as2", Name: "HDFC Mid Cap Opportunities Fund"},
		},
	}

	p.SetAssetTags("as1", []string{"Mid Cap", "Growth"})
	p.SetAssetTags("as2", []string{"Mid Cap", "Long Term"})

	if got := p.Assets[0].Tags; len(got) != 2 || got[0] != "Mid Cap" || got[1] != "Growth" {
		t.Errorf("as1 Tags = %v, want [Mid Cap Growth] in that order", got)
	}

	allTags := p.AllTags()
	want := []string{"Growth", "Long Term", "Mid Cap"} // alphabetical, deduped
	if len(allTags) != len(want) {
		t.Fatalf("AllTags() = %v, want %v", allTags, want)
	}
	for i, tag := range want {
		if allTags[i] != tag {
			t.Errorf("AllTags()[%d] = %q, want %q", i, allTags[i], tag)
		}
	}

	// Clearing (nil, matching a "select nothing" save from the UI) must
	// normalize to an empty, non-nil slice - same reasoning as Load().
	p.SetAssetTags("as1", nil)
	if p.Assets[0].Tags == nil {
		t.Errorf("SetAssetTags(nil) left Tags nil, want normalized empty slice")
	}
}

func TestSetAssetPrimaryTag(t *testing.T) {
	p := &Portfolio{Assets: []Asset{{ID: "as1", Tags: []string{"Mid Cap", "Growth"}}}}
	p.SetAssetPrimaryTag("as1", "Growth")
	if p.Assets[0].PrimaryTag != "Growth" {
		t.Errorf("PrimaryTag = %q, want Growth", p.Assets[0].PrimaryTag)
	}
	p.SetAssetPrimaryTag("as1", "")
	if p.Assets[0].PrimaryTag != "" {
		t.Errorf("PrimaryTag = %q, want cleared to empty", p.Assets[0].PrimaryTag)
	}
	// No-op for an unknown asset ID - should not panic or affect anything.
	p.SetAssetPrimaryTag("nonexistent", "Growth")
}

func TestLoadMissingFileReturnsEmptyPortfolio(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if len(p.Members) != 0 {
		t.Errorf("expected empty portfolio, got %+v", p)
	}
}

func TestSaveCreatesBackupOfPreviousVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "portfolio.json")

	if err := Save(path, &Portfolio{Members: []Member{{ID: "m1", Name: "First"}}}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := Save(path, &Portfolio{Members: []Member{{ID: "m1", Name: "Second"}}}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	backups, err := os.ReadDir(filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatalf("reading backups dir: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup (of the first save), got %d", len(backups))
	}

	loaded, _ := Load(path)
	if loaded.Members[0].Name != "Second" {
		t.Errorf("current file should have the latest save, got %+v", loaded.Members)
	}
}

func TestUpsertPrices_ReplacesSameDateInsteadOfDuplicating(t *testing.T) {
	p := &Portfolio{
		Prices: []PriceRecord{
			{AssetID: "a1", Date: "2026-01-01", Price: 100, Source: "MANUAL"},
		},
	}
	p.UpsertPrices([]PriceRecord{
		{AssetID: "a1", Date: "2026-01-01", Price: 105, Source: "TIGZIG_HISTORY"}, // same date - should replace, not duplicate
		{AssetID: "a1", Date: "2026-01-02", Price: 106, Source: "TIGZIG_HISTORY"}, // new date - should append
	})
	if len(p.Prices) != 2 {
		t.Fatalf("expected 2 price records after upsert, got %d: %+v", len(p.Prices), p.Prices)
	}
	price, ok := p.PriceAsOf("a1", "2026-01-01")
	if !ok || price != 105 {
		t.Errorf("PriceAsOf(2026-01-01) = %v, %v; want 105, true (should reflect the replacement)", price, ok)
	}
}

// TestPriceAsOf_IndexInvalidatedAfterUpsert specifically exercises the
// build-cache-then-mutate-then-read sequence: PriceAsOf is called FIRST
// (forcing the lazy index to build), then UpsertPrices mutates the
// underlying data, then PriceAsOf is called again - it must reflect the
// mutation, not serve a stale cached index from before the upsert. This
// is the correctness property the indexed lookup (see PriceAsOf's doc
// comment) depends on that a naive linear scan never needed to worry
// about.
func TestPriceAsOf_IndexInvalidatedAfterUpsert(t *testing.T) {
	p := &Portfolio{
		Prices: []PriceRecord{
			{AssetID: "a1", Date: "2026-01-01", Price: 100},
		},
	}

	// Force the index to build against the ORIGINAL data.
	price, ok := p.PriceAsOf("a1", "2026-01-01")
	if !ok || price != 100 {
		t.Fatalf("PriceAsOf before upsert = %v, %v; want 100, true", price, ok)
	}

	// Mutate after the index already exists.
	p.UpsertPrices([]PriceRecord{
		{AssetID: "a1", Date: "2026-01-01", Price: 999},
		{AssetID: "a1", Date: "2026-01-15", Price: 150},
	})

	price, ok = p.PriceAsOf("a1", "2026-01-01")
	if !ok || price != 999 {
		t.Errorf("PriceAsOf after upsert (same date) = %v, %v; want 999, true - stale cached index", price, ok)
	}
	price, ok = p.PriceAsOf("a1", "2026-01-15")
	if !ok || price != 150 {
		t.Errorf("PriceAsOf after upsert (new date) = %v, %v; want 150, true - new record missing from stale index", price, ok)
	}
}

// TestFXRateAsOf_IndexInvalidatedAfterUpsert is FXRateAsOf's counterpart
// to the above.
func TestFXRateAsOf_IndexInvalidatedAfterUpsert(t *testing.T) {
	p := &Portfolio{
		FXRates: []FXRate{
			{Currency: "CAD", Date: "2026-01-01", INRPerUnit: 60.0},
		},
	}

	rate, ok := p.FXRateAsOf("CAD", "2026-01-01")
	if !ok || rate != 60.0 {
		t.Fatalf("FXRateAsOf before upsert = %v, %v; want 60.0, true", rate, ok)
	}

	p.UpsertFXRates([]FXRate{
		{Currency: "CAD", Date: "2026-01-01", INRPerUnit: 61.5},
	})

	rate, ok = p.FXRateAsOf("CAD", "2026-01-01")
	if !ok || rate != 61.5 {
		t.Errorf("FXRateAsOf after upsert = %v, %v; want 61.5, true - stale cached index", rate, ok)
	}
}

func TestPriceAsOf_IgnoresPricesAfterTheDate(t *testing.T) {
	p := &Portfolio{
		Prices: []PriceRecord{
			{AssetID: "a1", Date: "2026-01-01", Price: 100},
			{AssetID: "a1", Date: "2026-01-10", Price: 110},
			{AssetID: "a1", Date: "2026-01-20", Price: 120},
		},
	}
	// A date between two known points should pick the earlier one, not
	// the nearer one - this is "what was it worth on this date", not
	// nearest-neighbour interpolation.
	price, ok := p.PriceAsOf("a1", "2026-01-15")
	if !ok || price != 110 {
		t.Errorf("PriceAsOf(2026-01-15) = %v, %v; want 110, true", price, ok)
	}
	// A date before any known price should find nothing.
	_, ok = p.PriceAsOf("a1", "2025-12-01")
	if ok {
		t.Errorf("PriceAsOf(2025-12-01) should find nothing before the first known price")
	}
}

func TestFXRateAsOf_INRIsAlwaysOne(t *testing.T) {
	p := &Portfolio{}
	rate, ok := p.FXRateAsOf("INR", "2020-01-01")
	if !ok || rate != 1.0 {
		t.Errorf("FXRateAsOf(INR) = %v, %v; want 1.0, true even with no stored rates", rate, ok)
	}
}

func TestUpsertFXRates_ReplacesSameDateInsteadOfDuplicating(t *testing.T) {
	p := &Portfolio{
		FXRates: []FXRate{
			{Date: "2026-01-01", Currency: "CAD", INRPerUnit: 60.0},
		},
	}
	p.UpsertFXRates([]FXRate{
		{Date: "2026-01-01", Currency: "CAD", INRPerUnit: 61.5}, // same date - should replace
		{Date: "2026-01-02", Currency: "CAD", INRPerUnit: 61.7}, // new date - should append
	})
	if len(p.FXRates) != 2 {
		t.Fatalf("expected 2 FX rate records after upsert, got %d: %+v", len(p.FXRates), p.FXRates)
	}
	rate, ok := p.FXRateAsOf("CAD", "2026-01-01")
	if !ok || rate != 61.5 {
		t.Errorf("FXRateAsOf(CAD, 2026-01-01) = %v, %v; want 61.5, true (should reflect the replacement)", rate, ok)
	}
}

func TestFindAssetByISIN(t *testing.T) {
	p := &Portfolio{Assets: []Asset{
		{ID: "a1", ISIN: "INF204K01E54", Name: "Growth Mid Cap"},
		{ID: "a2", ISIN: "", Name: "No ISIN Asset"},
	}}
	found, ok := p.FindAssetByISIN("INF204K01E54")
	if !ok || found.ID != "a1" {
		t.Errorf("expected to find a1, got %+v ok=%v", found, ok)
	}
	if _, ok := p.FindAssetByISIN(""); ok {
		t.Error("empty ISIN should never match, even against an asset with an empty ISIN field")
	}
	if _, ok := p.FindAssetByISIN("NOPE"); ok {
		t.Error("unknown ISIN should not match")
	}
}

func TestCapComposition_SetThenGetReturnsLatest(t *testing.T) {
	p := &Portfolio{}
	if _, ok := p.GetCapComposition("ast1"); ok {
		t.Fatal("expected no composition before any Set")
	}

	p.SetCapComposition("ast1", 20.24, 69.56, 10.20, 0, "2026-08-21", "Factsheet Aug 2026")
	got, ok := p.GetCapComposition("ast1")
	if !ok {
		t.Fatal("expected composition after Set")
	}
	if got.Large != 20.24 || got.Mid != 69.56 || got.Small != 10.20 {
		t.Errorf("got %+v", got)
	}

	// Setting again overwrites in place rather than accumulating history.
	p.SetCapComposition("ast1", 25, 60, 10, 5, "2026-09-21", "Factsheet Sep 2026")
	got, _ = p.GetCapComposition("ast1")
	if got.Large != 25 || got.AsOf != "2026-09-21" {
		t.Errorf("expected overwritten values, got %+v", got)
	}
	if len(p.CapCompositions) != 1 {
		t.Errorf("expected exactly 1 stored composition (overwrite, not append), got %d", len(p.CapCompositions))
	}
}

func TestAddAndRemoveBenchmark(t *testing.T) {
	p := &Portfolio{}

	b := p.AddBenchmark("Nifty 50", "^NSEI")
	if b.ID == "" {
		t.Fatal("expected a generated ID")
	}
	if b.Name != "Nifty 50" || b.YahooTicker != "^NSEI" {
		t.Errorf("got %+v", b)
	}
	if len(p.Benchmarks) != 1 {
		t.Fatalf("expected 1 benchmark, got %d", len(p.Benchmarks))
	}

	p.RemoveBenchmark(b.ID)
	if len(p.Benchmarks) != 0 {
		t.Errorf("expected 0 benchmarks after remove, got %d", len(p.Benchmarks))
	}

	// No-op for an unknown ID - should not panic.
	p.RemoveBenchmark("nonexistent")
}

func TestPriceSeries_ReturnsSortedRecordsForOneKey(t *testing.T) {
	p := &Portfolio{
		Prices: []PriceRecord{
			{AssetID: "nifty50", Date: "2024-01-20", Price: 200},
			{AssetID: "as1", Date: "2024-01-15", Price: 100},
			{AssetID: "nifty50", Date: "2024-01-10", Price: 190},
		},
	}

	series := p.PriceSeries("nifty50")
	if len(series) != 2 {
		t.Fatalf("expected 2 records for nifty50, got %d", len(series))
	}
	if series[0].Date != "2024-01-10" || series[1].Date != "2024-01-20" {
		t.Errorf("expected ascending date order, got %+v", series)
	}

	if len(p.PriceSeries("nonexistent")) != 0 {
		t.Error("expected empty series for an unknown key")
	}
}
