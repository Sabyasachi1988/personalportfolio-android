package bridge

import (
	"encoding/json"
	"testing"

	"ledger/internal/casimport"
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
