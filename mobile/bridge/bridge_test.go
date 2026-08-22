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
