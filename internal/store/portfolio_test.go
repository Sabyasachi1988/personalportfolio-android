package store

import (
	"os"
	"path/filepath"
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
