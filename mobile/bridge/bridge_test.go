package bridge

import (
	"encoding/json"
	"testing"

	"ledger/internal/casimport"
	"ledger/internal/finance"
	"ledger/internal/priceapi"
	"ledger/internal/store"
)

func TestCommitStagedRows_CreatesLinkedTransactions(t *testing.T) {
	units := 5.386
	rows := []casimport.StagedRow{
		{
			Txn: store.Transaction{
				Date: "2025-07-01", Description: "Purchase", Amount: 24998.75,
				Units: &units, Type: store.PurchaseSIP, AMC: "Nippon India Mutual Fund",
				Folio: "499388482035", Scheme: "NIPPON INDIA GROWTH MID CAP FUND", ISIN: "INF204K01E54",
			},
			Status: "NEW", SourceFolio: "499388482035", SourcePage: 3,
		},
		{
			// Should be skipped: not NEW.
			Txn:    store.Transaction{Date: "2025-07-02", ISIN: "INF204K01E54", Amount: 100},
			Status: "DUPLICATE",
		},
	}
	rowsJSON, _ := json.Marshal(rows)

	result := CommitStagedRows("", string(rowsJSON))

	var p store.Portfolio
	if err := json.Unmarshal([]byte(result), &p); err != nil {
		t.Fatalf("CommitStagedRows returned invalid JSON: %v\nresult: %s", err, result)
	}

	if len(p.Members) != 1 || p.Members[0].Name != "Me" {
		t.Fatalf("expected one Member named 'Me', got %+v", p.Members)
	}
	if len(p.Accounts) != 1 || p.Accounts[0].Name != "CAS Import" {
		t.Fatalf("expected one Account named 'CAS Import', got %+v", p.Accounts)
	}
	if len(p.Assets) != 1 || p.Assets[0].ISIN != "INF204K01E54" {
		t.Fatalf("expected one Asset with ISIN INF204K01E54, got %+v", p.Assets)
	}
	// Only the NEW row should have been committed - the DUPLICATE row must
	// not appear as a transaction.
	if len(p.Transactions) != 1 {
		t.Fatalf("expected exactly 1 committed transaction (NEW only), got %d", len(p.Transactions))
	}
	txn := p.Transactions[0]
	if txn.AssetID != p.Assets[0].ID {
		t.Errorf("transaction AssetID = %q, want %q (linked to the created asset)", txn.AssetID, p.Assets[0].ID)
	}
	if txn.Amount != 24998.75 {
		t.Errorf("transaction Amount = %v, want 24998.75", txn.Amount)
	}
	if txn.Source != "CAS_IMPORT" {
		t.Errorf("transaction Source = %q, want CAS_IMPORT", txn.Source)
	}
}

func TestCommitStagedRows_ReimportReusesExistingAssetAndAccount(t *testing.T) {
	units := 1.0
	makeRows := func(date string) string {
		rows := []casimport.StagedRow{{
			Txn: store.Transaction{
				Date: date, Amount: 100, Units: &units, Type: store.Purchase,
				Scheme: "SOME FUND", ISIN: "INF999999999",
			},
			Status: "NEW",
		}}
		b, _ := json.Marshal(rows)
		return string(b)
	}

	// First commit, starting from an empty portfolio.
	afterFirst := CommitStagedRows("", makeRows("2025-01-01"))

	// Second commit, starting from the first commit's own output - as the
	// real app would do on a second CAS import.
	afterSecond := CommitStagedRows(afterFirst, makeRows("2025-02-01"))

	var p store.Portfolio
	if err := json.Unmarshal([]byte(afterSecond), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(p.Members) != 1 {
		t.Errorf("expected exactly 1 Member after two imports, got %d (Member should be reused, not duplicated)", len(p.Members))
	}
	if len(p.Accounts) != 1 {
		t.Errorf("expected exactly 1 Account after two imports, got %d (Account should be reused, not duplicated)", len(p.Accounts))
	}
	if len(p.Assets) != 1 {
		t.Errorf("expected exactly 1 Asset after two imports (same ISIN), got %d (Asset should be matched by ISIN, not duplicated)", len(p.Assets))
	}
	if len(p.Transactions) != 2 {
		t.Errorf("expected 2 transactions after two imports, got %d", len(p.Transactions))
	}
}

func TestAmfiDateToISO(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantOK  bool
	}{
		{"20-Aug-2026", "2026-08-20", true},
		{"01-Jan-2025", "2025-01-01", true},
		{"not-a-date", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := amfiDateToISO(c.in)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("amfiDateToISO(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

func TestApplyAmfiRecords_MatchesByEitherISINColumn(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "asset-payout", ISIN: "INF111111111"},
			{ID: "asset-reinvest", ISIN: "INF222222222"},
			{ID: "asset-nomatch", ISIN: "INF999999999"},
		},
	}
	records := []priceapi.NavRecord{
		{ISINPayout: "INF111111111", ISINReinvest: "", NAV: 100.50, Date: "20-Aug-2026"},
		{ISINPayout: "", ISINReinvest: "INF222222222", NAV: 55.25, Date: "20-Aug-2026"},
		{ISINPayout: "INFNOTHING", ISINReinvest: "INFNOTHING2", NAV: 1.0, Date: "20-Aug-2026"},
	}

	matched := applyAmfiRecords(p, records)

	if matched != 2 {
		t.Fatalf("matched = %d, want 2", matched)
	}
	if len(p.Prices) != 2 {
		t.Fatalf("expected 2 PriceRecords, got %d", len(p.Prices))
	}

	priceFor := func(assetID string) (float64, bool) {
		for _, pr := range p.Prices {
			if pr.AssetID == assetID {
				return pr.Price, true
			}
		}
		return 0, false
	}

	if price, ok := priceFor("asset-payout"); !ok || price != 100.50 {
		t.Errorf("asset-payout price = %v, ok=%v, want 100.50", price, ok)
	}
	if price, ok := priceFor("asset-reinvest"); !ok || price != 55.25 {
		t.Errorf("asset-reinvest price = %v, ok=%v, want 55.25", price, ok)
	}
	if _, ok := priceFor("asset-nomatch"); ok {
		t.Errorf("asset-nomatch should have no price record, but got one")
	}
}

func TestApplyAmfiRecords_SameDayRefreshUpdatesInPlaceRatherThanDuplicating(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", ISIN: "INF111111111"}},
	}
	records1 := []priceapi.NavRecord{{ISINPayout: "INF111111111", NAV: 100.0, Date: "20-Aug-2026"}}
	records2 := []priceapi.NavRecord{{ISINPayout: "INF111111111", NAV: 101.5, Date: "20-Aug-2026"}} // same date, revised price

	applyAmfiRecords(p, records1)
	applyAmfiRecords(p, records2)

	if len(p.Prices) != 1 {
		t.Fatalf("expected exactly 1 PriceRecord after two same-day refreshes, got %d", len(p.Prices))
	}
	if p.Prices[0].Price != 101.5 {
		t.Errorf("price = %v, want 101.5 (the second refresh's value)", p.Prices[0].Price)
	}
}

func TestApplyAmfiRecords_DifferentDayAppendsNewRecord(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", ISIN: "INF111111111"}},
	}
	applyAmfiRecords(p, []priceapi.NavRecord{{ISINPayout: "INF111111111", NAV: 100.0, Date: "19-Aug-2026"}})
	applyAmfiRecords(p, []priceapi.NavRecord{{ISINPayout: "INF111111111", NAV: 102.0, Date: "20-Aug-2026"}})

	if len(p.Prices) != 2 {
		t.Fatalf("expected 2 PriceRecords across two different days, got %d", len(p.Prices))
	}

	holdings := finance.ComputeHoldings(&store.Portfolio{
		Assets: p.Assets,
		Prices: p.Prices,
		Transactions: []store.StoredTransaction{{
			AssetID: "asset-1", AccountID: "acc", Date: "2025-01-01", Amount: 100,
			Units: floatPtr(1), Type: store.Purchase,
		}},
	})
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	// ComputeHoldings must pick the LATER date's price (102.0), not
	// whichever was appended last by coincidence - this is the whole
	// reason amfiDateToISO exists (see its doc comment).
	if holdings[0].CurrentPrice != 102.0 {
		t.Errorf("CurrentPrice = %v, want 102.0 (the later date's price)", holdings[0].CurrentPrice)
	}
}

func floatPtr(f float64) *float64 { return &f }

func TestSetCapComposition_CreatesThenOverwritesInPlace(t *testing.T) {
	after1 := SetCapComposition("", "asset-1", 60, 30, 10, 0, "2026-08-01", "Factsheet Aug 2026")

	var p1 store.Portfolio
	if err := json.Unmarshal([]byte(after1), &p1); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p1.CapCompositions) != 1 {
		t.Fatalf("expected 1 CapComposition, got %d", len(p1.CapCompositions))
	}
	if p1.CapCompositions[0].Large != 60 || p1.CapCompositions[0].Mid != 30 || p1.CapCompositions[0].Small != 10 {
		t.Errorf("composition = %+v, want 60/30/10", p1.CapCompositions[0])
	}

	// Re-entering for the same asset should overwrite, not duplicate -
	// matches the desktop app's "there is only ever one current entry per
	// asset" design (see SetCapComposition's own doc comment in
	// internal/store/portfolio.go).
	after2 := SetCapComposition(after1, "asset-1", 50, 30, 15, 5, "2026-09-01", "Factsheet Sep 2026")

	var p2 store.Portfolio
	if err := json.Unmarshal([]byte(after2), &p2); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p2.CapCompositions) != 1 {
		t.Fatalf("expected still exactly 1 CapComposition after overwrite, got %d", len(p2.CapCompositions))
	}
	if p2.CapCompositions[0].Large != 50 || p2.CapCompositions[0].Cash != 5 {
		t.Errorf("composition after overwrite = %+v, want Large=50, Cash=5", p2.CapCompositions[0])
	}
}

func TestSetCapComposition_ActuallyChangesAllocationOutput(t *testing.T) {
	units := 100.0
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", Name: "SOME MULTI CAP FUND"}},
		Prices: []store.PriceRecord{{AssetID: "asset-1", Date: "2026-08-20", Price: 10}},
		Transactions: []store.StoredTransaction{{
			AssetID: "asset-1", AccountID: "acc", Date: "2025-01-01", Amount: 1000,
			Units: floatPtr(units), Type: store.Purchase,
		}},
	}
	pJSON, _ := json.Marshal(p)

	// Before any composition is entered: falls back to the single-bucket
	// heuristic, which (correctly) can't do better than "Multi Cap" for a
	// fund name like this - this IS the gap the person asked to fix.
	holdingsJSON := ComputeHoldings(string(pJSON))
	allocBefore := ComputeAllocationByMarketCap(string(pJSON))
	_ = holdingsJSON
	if !containsLabel(allocBefore, "Multi Cap") {
		t.Fatalf("expected fallback allocation to include 'Multi Cap' before any composition entered, got: %s", allocBefore)
	}

	updated := SetCapComposition(string(pJSON), "asset-1", 50, 30, 15, 5, "2026-08-20", "Factsheet Aug 2026")
	allocAfter := ComputeAllocationByMarketCap(updated)

	if containsLabel(allocAfter, "Multi Cap") {
		t.Errorf("expected 'Multi Cap' to disappear once a real composition is entered, got: %s", allocAfter)
	}
	if !containsLabel(allocAfter, "Large Cap") || !containsLabel(allocAfter, "Cash") {
		t.Errorf("expected Large Cap and Cash buckets after composition entered, got: %s", allocAfter)
	}
}

func containsLabel(allocationJSON string, label string) bool {
	var slices []map[string]any
	if err := json.Unmarshal([]byte(allocationJSON), &slices); err != nil {
		return false
	}
	for _, s := range slices {
		if s["Label"] == label {
			return true
		}
	}
	return false
}
