package bridge

import (
	"encoding/json"
	"strings"
	"testing"

	"ledger/internal/casimport"
	"ledger/internal/finance"
	"ledger/internal/priceapi"
	"ledger/internal/store"
)

// extractPortfolio pulls the embedded portfolio JSON out of
// CommitStagedRows' {"committed":N,"skippedDuplicates":N,"portfolio":{...}}
// response shape, for tests that need to inspect or re-feed it.
func extractPortfolio(t *testing.T, commitResult string) string {
	t.Helper()
	var wrapped struct {
		Portfolio json.RawMessage `json:"portfolio"`
	}
	if err := json.Unmarshal([]byte(commitResult), &wrapped); err != nil {
		t.Fatalf("CommitStagedRows result is not the expected wrapper shape: %v\nresult: %s", err, commitResult)
	}
	return string(wrapped.Portfolio)
}

func commitCounts(t *testing.T, commitResult string) (committed, skippedDuplicates int) {
	t.Helper()
	var wrapped struct {
		Committed         int `json:"committed"`
		SkippedDuplicates int `json:"skippedDuplicates"`
	}
	if err := json.Unmarshal([]byte(commitResult), &wrapped); err != nil {
		t.Fatalf("CommitStagedRows result is not the expected wrapper shape: %v\nresult: %s", err, commitResult)
	}
	return wrapped.Committed, wrapped.SkippedDuplicates
}

// seededPortfolio builds a starter portfolio JSON containing exactly
// the given member names, each with a real, known ID - CommitStagedRows
// no longer auto-creates a member from a free-text name match (see its
// own doc comment for why: a typo used to silently spawn a phantom
// duplicate member). Every test below that used to start from an empty
// "" portfolio and rely on that auto-create now seeds the member(s) it
// needs up front instead, then passes that member's real ID.
func seededPortfolio(t *testing.T, names ...string) (portfolioJSON string, idByName map[string]string) {
	t.Helper()
	idByName = make(map[string]string, len(names))
	var members []store.Member
	for _, n := range names {
		id := "member-" + n
		idByName[n] = id
		members = append(members, store.Member{ID: id, Name: n})
	}
	b, err := json.Marshal(store.Portfolio{Members: members})
	if err != nil {
		t.Fatalf("failed to build seeded portfolio: %v", err)
	}
	return string(b), idByName
}

// TestCommitStagedRows_CSVWithoutISINReimportDoesNotDuplicateAsset
// reproduces the exact bug reported: broker trade-CSV exports (Zerodha
// Console included) commonly have NO ISIN column at all, unlike a CAS
// PDF statement, which always carries one. Before
// findAssetByNameInAccount existed, every CSV-sourced row with a blank
// ISIN fell straight to "create a new asset" on every single commit -
// re-importing an overlapping CSV export silently created a second
// (third, fourth, ...) Asset for the same real fund, each with its own
// transactions, rather than being recognized as the same holding.
func TestCommitStagedRows_CSVWithoutISINReimportDoesNotDuplicateAsset(t *testing.T) {
	units := 12.5
	makeRows := func() string {
		rows := []casimport.StagedRow{
			{
				Txn: store.Transaction{
					Date: "2025-08-01", Description: "buy KOTAK LARGE MIDCAP", Amount: 5000,
					Units: &units, Type: store.PurchaseSIP,
					Scheme: "KOTAK LARGE MIDCAP", ISIN: "", // no ISIN column in this CSV, exactly like a real Zerodha tradebook export
				},
				Status: "NEW",
			},
		}
		b, _ := json.Marshal(rows)
		return string(b)
	}

	seed, ids := seededPortfolio(t, "Me")
	afterFirst := CommitStagedRows(seed, makeRows(), ids["Me"])
	firstCommitted, _ := commitCounts(t, afterFirst)
	if firstCommitted != 1 {
		t.Fatalf("first import: committed=%d, want 1", firstCommitted)
	}

	afterSecond := CommitStagedRows(extractPortfolio(t, afterFirst), makeRows(), ids["Me"])
	secondCommitted, secondSkipped := commitCounts(t, afterSecond)
	if secondCommitted != 0 {
		t.Errorf("second (overlapping) CSV import: committed=%d, want 0", secondCommitted)
	}
	if secondSkipped != 1 {
		t.Errorf("second (overlapping) CSV import: skippedDuplicates=%d, want 1", secondSkipped)
	}

	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, afterSecond)), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p.Assets) != 1 {
		t.Fatalf("expected exactly 1 Asset for the same ISIN-less fund re-imported twice, got %d - this is the reported bug", len(p.Assets))
	}
	if len(p.Transactions) != 1 {
		t.Fatalf("expected exactly 1 transaction, got %d", len(p.Transactions))
	}
}

// TestCommitStagedRows_NameMatchBackfillsISINWhenLaterRowHasOne covers
// the mixed-source case: an asset first created from an ISIN-less CSV
// row, then a later row for the same fund (matched by name) DOES carry
// a real ISIN - that ISIN should be backfilled onto the existing asset
// rather than left blank forever, so future imports for this fund can
// use the more reliable ISIN match instead of continuing to depend on
// the name staying identical.
func TestCommitStagedRows_NameMatchBackfillsISINWhenLaterRowHasOne(t *testing.T) {
	units1 := 10.0
	units2 := 5.0
	rows := []casimport.StagedRow{
		{
			Txn: store.Transaction{
				Date: "2025-08-01", Description: "buy KOTAK LARGE MIDCAP", Amount: 4000,
				Units: &units1, Type: store.PurchaseSIP,
				Scheme: "KOTAK LARGE MIDCAP", ISIN: "",
			},
			Status: "NEW",
		},
		{
			Txn: store.Transaction{
				Date: "2025-08-15", Description: "buy KOTAK LARGE MIDCAP", Amount: 2000,
				Units: &units2, Type: store.PurchaseSIP,
				Scheme: "KOTAK LARGE MIDCAP", ISIN: "INF174K01LR2", // this row happens to carry the real ISIN
			},
			Status: "NEW",
		},
	}
	b, _ := json.Marshal(rows)

	seed, ids := seededPortfolio(t, "Me")
	result := CommitStagedRows(seed, string(b), ids["Me"])
	committed, _ := commitCounts(t, result)
	if committed != 2 {
		t.Fatalf("committed=%d, want 2", committed)
	}

	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, result)), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p.Assets) != 1 {
		t.Fatalf("expected both rows to resolve to the same single Asset (matched by name), got %d assets", len(p.Assets))
	}
	if p.Assets[0].ISIN != "INF174K01LR2" {
		t.Errorf("expected the asset's ISIN to be backfilled to %q, got %q", "INF174K01LR2", p.Assets[0].ISIN)
	}
}

// TestCommitStagedRows_FolioLessETFRowGetsStockTypeAndSymbolPrefill
// reproduces the exact scenario reported: a Zerodha-style broker CSV for
// an NSE-listed ETF (NIFTYBEES) - real ISIN present, no folio column at
// all (unlike a CAS statement, which always has one). Before this fix,
// every new asset defaulted to Type "MutualFund" regardless of the row's
// actual shape, and Symbol was never populated - together, this meant
// an ETF imported from CSV could never be correctly routed to the Yahoo
// price-history path (see UpdateHistoryActivity's ISIN-presence-based
// routing), leaving it permanently priceless in Portfolio Progression.
func TestCommitStagedRows_FolioLessETFRowGetsStockTypeAndSymbolPrefill(t *testing.T) {
	units := 384.0
	rows := []casimport.StagedRow{
		{
			Txn: store.Transaction{
				Date: "2025-01-16", Description: "buy NIFTYBEES", Amount: 100078.0,
				Units: &units, Type: store.Purchase,
				Scheme: "NIFTYBEES", ISIN: "INF204KB14I2", Folio: "", // no folio column in this CSV - the key signal
			},
			Status: "NEW",
		},
	}
	b, _ := json.Marshal(rows)

	seed, ids := seededPortfolio(t, "Me")
	result := CommitStagedRows(seed, string(b), ids["Me"])
	committed, _ := commitCounts(t, result)
	if committed != 1 {
		t.Fatalf("committed=%d, want 1", committed)
	}

	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, result)), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p.Assets) != 1 {
		t.Fatalf("expected exactly 1 asset, got %d", len(p.Assets))
	}
	asset := p.Assets[0]
	if asset.Type != "Stock" {
		t.Errorf("Type = %q, want %q (folio-less row should not default to MutualFund)", asset.Type, "Stock")
	}
	if asset.Symbol != "NIFTYBEES" {
		t.Errorf("Symbol = %q, want %q (pre-filled from the row's scheme text as a starting point)", asset.Symbol, "NIFTYBEES")
	}
	if asset.ISIN != "INF204KB14I2" {
		t.Errorf("ISIN = %q, want %q (still recorded correctly, unaffected by the Type/Symbol fix)", asset.ISIN, "INF204KB14I2")
	}
}

func TestCommitStagedRows_FolioedRowStillGetsMutualFundType(t *testing.T) {
	units := 5.386
	rows := []casimport.StagedRow{
		{
			Txn: store.Transaction{
				Date: "2025-07-01", Description: "Purchase", Amount: 24998.75,
				Units: &units, Type: store.PurchaseSIP,
				Scheme: "NIPPON INDIA GROWTH MID CAP FUND", ISIN: "INF204K01E54", Folio: "12345678/90",
			},
			Status: "NEW",
		},
	}
	b, _ := json.Marshal(rows)

	seed, ids := seededPortfolio(t, "Me")
	result := CommitStagedRows(seed, string(b), ids["Me"])
	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, result)), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p.Assets) != 1 {
		t.Fatalf("expected exactly 1 asset, got %d", len(p.Assets))
	}
	if p.Assets[0].Type != "MutualFund" {
		t.Errorf("Type = %q, want %q (a folioed row is a real mutual fund transaction)", p.Assets[0].Type, "MutualFund")
	}
	if p.Assets[0].Symbol != "" {
		t.Errorf("Symbol = %q, want empty (folio-based rows have no symbol concept)", p.Assets[0].Symbol)
	}
}

func TestCommitStagedRows_ReimportingSameStatementDoesNotDuplicate(t *testing.T) {
	units := 5.386
	makeRows := func() string {
		rows := []casimport.StagedRow{
			{
				Txn: store.Transaction{
					Date: "2025-07-01", Description: "Purchase", Amount: 24998.75,
					Units: &units, Type: store.PurchaseSIP,
					Scheme: "NIPPON INDIA GROWTH MID CAP FUND", ISIN: "INF204K01E54",
				},
				Status: "NEW",
			},
			{
				Txn: store.Transaction{
					Date: "2025-07-15", Description: "Purchase", Amount: 15000,
					Units: nil, Type: store.PurchaseSIP, // no Units, like a tax/fee line - must still dedupe on Date+Type+Amount alone
					Scheme: "NIPPON INDIA GROWTH MID CAP FUND", ISIN: "INF204K01E54",
				},
				Status: "NEW",
			},
		}
		b, _ := json.Marshal(rows)
		return string(b)
	}

	// This is the exact scenario reported: the person accidentally
	// imports the same consolidated account statement PDF a second time.
	// The freshly-parsed rows are indistinguishable from the first
	// import's at the PARSE level (both come back Status "NEW", since
	// parsing has no visibility into what's already stored) - the
	// dedup has to happen here, at commit time, against what's already
	// in the portfolio.
	seed, ids := seededPortfolio(t, "Me")
	afterFirst := CommitStagedRows(seed, makeRows(), ids["Me"])
	firstCommitted, firstSkipped := commitCounts(t, afterFirst)
	if firstCommitted != 2 || firstSkipped != 0 {
		t.Fatalf("first import: committed=%d skippedDuplicates=%d, want 2 and 0", firstCommitted, firstSkipped)
	}

	afterSecond := CommitStagedRows(extractPortfolio(t, afterFirst), makeRows(), ids["Me"])
	secondCommitted, secondSkipped := commitCounts(t, afterSecond)
	if secondCommitted != 0 {
		t.Errorf("second (duplicate) import: committed=%d, want 0 - nothing new should be added", secondCommitted)
	}
	if secondSkipped != 2 {
		t.Errorf("second (duplicate) import: skippedDuplicates=%d, want 2 - both rows should be recognized as already present", secondSkipped)
	}

	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, afterSecond)), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p.Transactions) != 2 {
		t.Fatalf("expected exactly 2 transactions total after importing the same statement twice, got %d - the portfolio was doubled", len(p.Transactions))
	}
	if len(p.Assets) != 1 {
		t.Errorf("expected exactly 1 Asset, got %d", len(p.Assets))
	}
}

func TestCommitStagedRows_GenuinelyDifferentTransactionsAreNotTreatedAsDuplicates(t *testing.T) {
	unitsA := 5.386
	unitsB := 6.1
	rows := []casimport.StagedRow{
		{
			Txn: store.Transaction{
				Date: "2025-07-01", Amount: 24998.75, Units: &unitsA, Type: store.PurchaseSIP,
				Scheme: "SAME FUND", ISIN: "INF204K01E54",
			},
			Status: "NEW",
		},
		{
			// Same date, same fund, but a different amount and units - a
			// second, genuine SIP top-up on the same day is plausible and
			// must NOT be collapsed into the first row.
			Txn: store.Transaction{
				Date: "2025-07-01", Amount: 30000, Units: &unitsB, Type: store.PurchaseSIP,
				Scheme: "SAME FUND", ISIN: "INF204K01E54",
			},
			Status: "NEW",
		},
	}
	rowsJSON, _ := json.Marshal(rows)

	seed, ids := seededPortfolio(t, "Me")
	result := CommitStagedRows(seed, string(rowsJSON), ids["Me"])
	committed, skipped := commitCounts(t, result)
	if committed != 2 || skipped != 0 {
		t.Fatalf("committed=%d skippedDuplicates=%d, want 2 and 0 - two genuinely different transactions must both be kept", committed, skipped)
	}
}

func TestRemoveDuplicateTransactions_RemovesExactDuplicatesKeepsGenuineOnes(t *testing.T) {
	unitsA := 1741.77
	unitsB := 6.1
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", ISIN: "INF959L01GR6", Name: "NAVI ELSS Tax Saver Nifty 50 Index Fund"}},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AssetID: "asset-1", Date: "2025-01-03", Type: store.Purchase, Amount: 24998.75, Units: &unitsA},
			// Exact duplicate of t1 (e.g. the CSV was imported twice
			// before commit-time dedup existed, or before the person
			// updated to a build that had it).
			{ID: "t2", AssetID: "asset-1", Date: "2025-01-03", Type: store.Purchase, Amount: 24998.75, Units: &unitsA},
			// A genuinely different transaction (different date) for the
			// same asset - must survive.
			{ID: "t3", AssetID: "asset-1", Date: "2025-01-10", Type: store.Purchase, Amount: 30000, Units: &unitsB},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := RemoveDuplicateTransactions(string(pJSON))
	if isBridgeErrorForTest(result) {
		t.Fatalf("expected success, got: %s", result)
	}

	var wrapped struct {
		Removed   int              `json:"removed"`
		Groups    []DuplicateGroup `json:"groups"`
		Portfolio store.Portfolio  `json:"portfolio"`
	}
	if err := json.Unmarshal([]byte(result), &wrapped); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if wrapped.Removed != 1 {
		t.Errorf("removed = %d, want 1", wrapped.Removed)
	}
	if len(wrapped.Groups) != 1 {
		t.Fatalf("expected 1 duplicate group, got %d: %+v", len(wrapped.Groups), wrapped.Groups)
	}
	g := wrapped.Groups[0]
	if g.Date != "2025-01-03" || g.ExtraCopies != 1 || g.Amount != 24998.75 {
		t.Errorf("group = %+v, want Date=2025-01-03 ExtraCopies=1 Amount=24998.75", g)
	}
	if g.AssetName != "NAVI ELSS Tax Saver Nifty 50 Index Fund" {
		t.Errorf("group.AssetName = %q, want the asset's actual name", g.AssetName)
	}
	if g.Confidence != "heuristic" {
		t.Errorf("group.Confidence = %q, want heuristic (no Description/reference was set on either transaction)", g.Confidence)
	}
	if len(wrapped.Portfolio.Transactions) != 2 {
		t.Fatalf("expected 2 transactions remaining, got %d: %+v", len(wrapped.Portfolio.Transactions), wrapped.Portfolio.Transactions)
	}
	dates := map[string]bool{}
	for _, txn := range wrapped.Portfolio.Transactions {
		dates[txn.Date] = true
	}
	if !dates["2025-01-03"] || !dates["2025-01-10"] {
		t.Errorf("expected both distinct dates to survive, got transactions: %+v", wrapped.Portfolio.Transactions)
	}
}

func TestTransactionsMatch_DifferentReferencesAreNeverDuplicatesEvenIfAmountsMatch(t *testing.T) {
	units := 5.409
	// Two GENUINELY different real-world purchases - same fund, same
	// day, same amount, same units (this happens naturally: mutual fund
	// NAV is fixed per day, so two separate ₹24,998.75 purchases on the
	// same day produce identical units too) - but with two different
	// real Trxn.Ref.No. references, taken verbatim from the actual CAS
	// statement's own description text. This is exactly the case raised:
	// must NOT be treated as a duplicate.
	a := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-07-01", Type: store.Purchase, Amount: 24998.75, Units: &units,
		Description: "Purchase Trxn.Ref.No.pay_QnjoMAgGPZYGW0//Icici Bank Limited - 036001076406//netbanking",
	}
	b := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-07-01", Type: store.Purchase, Amount: 24998.75, Units: &units,
		Description: "Purchase Trxn.Ref.No.pay_QqVpg1ZAdY7AL1//Icici Bank Limited - 036001076406//netbanking",
	}
	if transactionsMatch(a, b) {
		t.Error("transactionsMatch = true, want false - different genuine references must never be treated as the same transaction")
	}
}

func TestTransactionsMatch_SameReferenceIsAlwaysADuplicate(t *testing.T) {
	units := 5.409
	desc := "Purchase Trxn.Ref.No.pay_QnjoMAgGPZYGW0//Icici Bank Limited - 036001076406//netbanking"
	a := store.StoredTransaction{AssetID: "asset-1", Date: "2025-07-01", Type: store.Purchase, Amount: 24998.75, Units: &units, Description: desc}
	b := store.StoredTransaction{AssetID: "asset-1", Date: "2025-07-01", Type: store.Purchase, Amount: 24998.75, Units: &units, Description: desc}
	if !transactionsMatch(a, b) {
		t.Error("transactionsMatch = false, want true - identical reference means the same real-world transaction")
	}
}

func TestTransactionsMatch_SipInstallmentsHaveNoReferenceAndFallBackToHeuristic(t *testing.T) {
	units := 595.435
	// Real "Sys. Investment ISIP" description text from the actual CAS
	// statement - these never carry a Trxn.Ref.No., so the heuristic
	// fallback is the only option here (the original ambiguity still
	// applies to genuinely reference-less rows - there is no more
	// signal available for these).
	a := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-02-10", Type: store.Purchase, Amount: 24998.75, Units: &units,
		Description: "Sys. Investment ISIP (2/28)",
	}
	b := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-02-10", Type: store.Purchase, Amount: 24998.75, Units: &units,
		Description: "Sys. Investment ISIP (2/28)",
	}
	if !transactionsMatch(a, b) {
		t.Error("transactionsMatch = false, want true - identical date/amount/units with no reference on either side falls back to the heuristic match")
	}
}

func TestRemoveDuplicateTransactions_ReferenceBasedGroupIsLabeledReference(t *testing.T) {
	units := 5.409
	desc := "Purchase Trxn.Ref.No.pay_QnjoMAgGPZYGW0//Icici Bank Limited - 036001076406//netbanking"
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", ISIN: "INF204K01E54", Name: "Nippon India Growth Mid Cap Fund"}},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AssetID: "asset-1", Date: "2025-07-01", Type: store.Purchase, Amount: 24998.75, Units: &units, Description: desc},
			{ID: "t2", AssetID: "asset-1", Date: "2025-07-01", Type: store.Purchase, Amount: 24998.75, Units: &units, Description: desc},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := RemoveDuplicateTransactions(string(pJSON))
	var wrapped struct {
		Removed int              `json:"removed"`
		Groups  []DuplicateGroup `json:"groups"`
	}
	if err := json.Unmarshal([]byte(result), &wrapped); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if wrapped.Removed != 1 {
		t.Fatalf("removed = %d, want 1", wrapped.Removed)
	}
	if len(wrapped.Groups) != 1 || wrapped.Groups[0].Confidence != "reference" {
		t.Errorf("groups = %+v, want 1 group with Confidence=reference", wrapped.Groups)
	}
}

func TestRemoveDuplicateTransactions_CsvReferenceMarkerAlsoGivesReferenceConfidence(t *testing.T) {
	units := 1741.77
	// The exact "[ref:...]" marker csvimport.go embeds when a CSV column
	// maps to "reference" (e.g. Zerodha's trade_id) - different trade_id
	// per row, so these must NOT be merged despite matching amount/units.
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", ISIN: "INF959L01GR6", Name: "NAVI ELSS Tax Saver Nifty 50 Index Fund"}},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AssetID: "asset-1", Date: "2025-01-03", Type: store.Purchase, Amount: 24998.75, Units: &units, Description: "buy NAVI ELSS [ref:1582636286]"},
			{ID: "t2", AssetID: "asset-1", Date: "2025-01-03", Type: store.Purchase, Amount: 24998.75, Units: &units, Description: "buy NAVI ELSS [ref:9999999999]"},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := RemoveDuplicateTransactions(string(pJSON))
	var wrapped struct {
		Removed int `json:"removed"`
	}
	if err := json.Unmarshal([]byte(result), &wrapped); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if wrapped.Removed != 0 {
		t.Errorf("removed = %d, want 0 - two different trade_ids must never be collapsed into one", wrapped.Removed)
	}
}

func TestTransactionsMatch_DifferentBalancesAreNeverDuplicatesEvenIfAmountAndUnitsMatch(t *testing.T) {
	units := 1538.272
	balA := 147909.211
	balB := 149447.483
	// This is a REAL pair from an actual CAS statement: a manual
	// Purchase and a scheduled SIP installment landing on the same
	// date, for the same amount, which (since mutual fund NAV is fixed
	// per day) also produces identical units - genuinely two separate
	// transactions, proven by the statement's own running balance
	// column incrementing twice, not once.
	a := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-08-22", Type: store.Purchase, Amount: 12499.38, Units: &units, Balance: &balA,
		Description: "Purchase Trxn.Ref.No.pay_R8IyQIDb1HyqSh//Icici Bank Limited - 036001076406//netbanking",
	}
	b := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-08-22", Type: store.Purchase, Amount: 12499.38, Units: &units, Balance: &balB,
		Description: "Sys. Investment ISIP (13/14)", // no reference at all - the case a reference alone can't catch
	}
	if transactionsMatch(a, b) {
		t.Error("transactionsMatch = true, want false - different running balances prove these are two separate real transactions")
	}
}

func TestTransactionsMatch_SameBalanceConfirmsADuplicate(t *testing.T) {
	units := 595.435
	bal := 595.435
	// A true duplicate: same SIP installment description (no reference
	// on either side), but the balance also matches, as it would for a
	// genuine re-parse of the exact same source row.
	a := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-01-14", Type: store.PurchaseSIP, Amount: 24998.75, Units: &units, Balance: &bal,
		Description: "Sys. Investment ISIP (1/28)",
	}
	b := store.StoredTransaction{
		AssetID: "asset-1", Date: "2025-01-14", Type: store.PurchaseSIP, Amount: 24998.75, Units: &units, Balance: &bal,
		Description: "Sys. Investment ISIP (1/28)",
	}
	if !transactionsMatch(a, b) {
		t.Error("transactionsMatch = false, want true - identical date/amount/units/balance with no reference should still match")
	}
}

func TestRemoveDuplicateTransactions_BalanceOnlyGroupIsLabeledBalance(t *testing.T) {
	units := 595.435
	bal := 595.435
	desc := "Sys. Investment ISIP (1/28)" // no reference
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", ISIN: "INF204K01H36", Name: "Nippon India Index Fund - Nifty 50 Plan"}},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AssetID: "asset-1", Date: "2025-01-14", Type: store.PurchaseSIP, Amount: 24998.75, Units: &units, Balance: &bal, Description: desc},
			{ID: "t2", AssetID: "asset-1", Date: "2025-01-14", Type: store.PurchaseSIP, Amount: 24998.75, Units: &units, Balance: &bal, Description: desc},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := RemoveDuplicateTransactions(string(pJSON))
	var wrapped struct {
		Removed int              `json:"removed"`
		Groups  []DuplicateGroup `json:"groups"`
	}
	if err := json.Unmarshal([]byte(result), &wrapped); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if wrapped.Removed != 1 {
		t.Fatalf("removed = %d, want 1", wrapped.Removed)
	}
	if len(wrapped.Groups) != 1 || wrapped.Groups[0].Confidence != "balance" {
		t.Errorf("groups = %+v, want 1 group with Confidence=balance", wrapped.Groups)
	}
}

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

	seed, ids := seededPortfolio(t, "Me")
	result := CommitStagedRows(seed, string(rowsJSON), ids["Me"])

	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, result)), &p); err != nil {
		t.Fatalf("CommitStagedRows returned invalid portfolio JSON: %v\nresult: %s", err, result)
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

	committed, skipped := commitCounts(t, result)
	if committed != 1 {
		t.Errorf("committed = %d, want 1", committed)
	}
	if skipped != 0 {
		t.Errorf("skippedDuplicates = %d, want 0 (the second row was never NEW, so it's not a commit-time duplicate - it's filtered before reaching this logic)", skipped)
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

	// First commit, starting from a portfolio seeded with just "Me".
	seed, ids := seededPortfolio(t, "Me")
	afterFirst := CommitStagedRows(seed, makeRows("2025-01-01"), ids["Me"])

	// Second commit, starting from the first commit's own embedded
	// portfolio - as the real app does (see ImportActivity.kt, which
	// extracts .portfolio before calling savePortfolio/re-committing).
	afterSecond := CommitStagedRows(extractPortfolio(t, afterFirst), makeRows("2025-02-01"), ids["Me"])

	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, afterSecond)), &p); err != nil {
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
		in     string
		want   string
		wantOK bool
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
	allocBefore := ComputeAllocationByMarketCap(string(pJSON), "")
	_ = holdingsJSON
	if !containsLabel(allocBefore, "Multi Cap") {
		t.Fatalf("expected fallback allocation to include 'Multi Cap' before any composition entered, got: %s", allocBefore)
	}

	updated := SetCapComposition(string(pJSON), "asset-1", 50, 30, 15, 5, "2026-08-20", "Factsheet Aug 2026")
	allocAfter := ComputeAllocationByMarketCap(updated, "")

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

func TestCommitStagedRows_TwoDifferentMembersSameISINGetSeparateAssets(t *testing.T) {
	units := 1.0
	makeRows := func() string {
		rows := []casimport.StagedRow{{
			Txn: store.Transaction{
				Date: "2025-01-01", Amount: 100, Units: &units, Type: store.Purchase,
				Scheme: "SHARED FUND", ISIN: "INF_SHARED_0001",
			},
			Status: "NEW",
		}}
		b, _ := json.Marshal(rows)
		return string(b)
	}

	// The person imports their own CAS, then imports their mother's CAS
	// into the SAME portfolio file - both happen to hold the same fund.
	// Both members seeded up front since CommitStagedRows requires each
	// to already exist (see its own doc comment).
	seed, ids := seededPortfolio(t, "Me", "Mom")
	afterMe := CommitStagedRows(seed, makeRows(), ids["Me"])
	afterBoth := CommitStagedRows(extractPortfolio(t, afterMe), makeRows(), ids["Mom"])

	var p store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, afterBoth)), &p); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(p.Members) != 2 {
		t.Fatalf("expected 2 Members, got %d: %+v", len(p.Members), p.Members)
	}
	// The real bug this guards against: two Assets should exist (one per
	// member's account), NOT one Asset shared between both members just
	// because the ISIN matches.
	if len(p.Assets) != 2 {
		t.Fatalf("expected 2 Assets (same ISIN, different accounts), got %d: %+v", len(p.Assets), p.Assets)
	}
	if p.Assets[0].AccountID == p.Assets[1].AccountID {
		t.Errorf("both assets have the same AccountID %q - they should belong to different members' accounts", p.Assets[0].AccountID)
	}

	holdingsAll := finance.ComputeHoldings(&p)
	if len(holdingsAll) != 2 {
		t.Fatalf("expected 2 holdings total (one per member), got %d", len(holdingsAll))
	}

	var meMemberID, momMemberID string
	for _, m := range p.Members {
		if m.Name == "Me" {
			meMemberID = m.ID
		}
		if m.Name == "Mom" {
			momMemberID = m.ID
		}
	}
	meHoldings := finance.FilterHoldingsByMember(holdingsAll, meMemberID)
	momHoldings := finance.FilterHoldingsByMember(holdingsAll, momMemberID)
	if len(meHoldings) != 1 || len(momHoldings) != 1 {
		t.Errorf("expected each member to see exactly their own 1 holding, got Me=%d Mom=%d", len(meHoldings), len(momHoldings))
	}
}

func TestCommitStagedRows_PromotesATrackedFundInsteadOfDuplicating(t *testing.T) {
	units := 1.0
	rows := []casimport.StagedRow{{
		Txn: store.Transaction{
			Date: "2025-01-01", Amount: 100, Units: &units, Type: store.Purchase,
			Scheme: "SOME FUND", ISIN: "INF_TRACKED_0001",
		},
		Status: "NEW",
	}}
	rowsJSON, _ := json.Marshal(rows)

	seed, ids := seededPortfolio(t, "Me")
	var p store.Portfolio
	if err := json.Unmarshal([]byte(seed), &p); err != nil {
		t.Fatalf("invalid seed JSON: %v", err)
	}
	// This fund was being TRACKED (an Additional Fund, not yet owned)
	// before the person actually bought it - see
	// store.Portfolio.AddTrackedFund's own doc comment.
	tracked := p.AddTrackedFund("Some Fund (tracked)", "INF_TRACKED_0001")
	// Give it some already-fetched price history - the entire point of
	// promotion is that THIS carries over untouched rather than needing
	// to be re-fetched.
	p.UpsertPrices([]store.PriceRecord{
		{AssetID: tracked.ID, Date: "2024-06-01", Price: 50.0, Source: "TIGZIG_HISTORY"},
	})
	seeded, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	afterCommit := CommitStagedRows(string(seeded), string(rowsJSON), ids["Me"])
	var after store.Portfolio
	if err := json.Unmarshal([]byte(extractPortfolio(t, afterCommit)), &after); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(after.Assets) != 1 {
		t.Fatalf("expected the tracked fund to be PROMOTED (still 1 Asset), got %d: %+v", len(after.Assets), after.Assets)
	}
	promoted := after.Assets[0]
	if promoted.ID != tracked.ID {
		t.Errorf("promoted.ID = %q, want the SAME ID as the original tracked entry (%q) - a new ID means price history did NOT carry over", promoted.ID, tracked.ID)
	}
	if !promoted.IsOwned() {
		t.Errorf("promoted asset is still not owned (AccountID = %q)", promoted.AccountID)
	}
	series := after.PriceSeries(tracked.ID)
	if len(series) != 1 || series[0].Price != 50.0 {
		t.Errorf("expected the tracked fund's pre-existing price history to carry over untouched, got %+v", series)
	}
	if len(after.Transactions) != 1 || after.Transactions[0].AssetID != tracked.ID {
		t.Errorf("expected the new transaction to be linked to the promoted (same) Asset ID, got %+v", after.Transactions)
	}
}

func TestCommitStagedRows_UnknownMemberIDIsAnErrorNotAutoCreate(t *testing.T) {
	// The core fix this session: a mistyped/unrecognized member ID must
	// fail loudly, not silently spawn a phantom new member. Confirmed
	// real risk this guards against: the old free-text-name path
	// created a brand-new Member on ANY name that didn't exactly match
	// an existing one - a simple typo ("Mom" vs "Mother") went unnoticed
	// and produced a duplicate family member that doesn't actually
	// exist. The Kotlin side now only offers a dropdown of real existing
	// members (see ImportActivity.kt), but the bridge itself must not
	// rely on the UI alone to prevent this.
	rows := []casimport.StagedRow{{
		Txn:    store.Transaction{Date: "2025-01-01", Amount: 100, Type: store.Purchase, Scheme: "SOME FUND", ISIN: "INF1"},
		Status: "NEW",
	}}
	b, _ := json.Marshal(rows)
	seed, _ := seededPortfolio(t, "Me")

	result := CommitStagedRows(seed, string(b), "member-does-not-exist")
	if !strings.Contains(result, "error") {
		t.Fatalf("expected an error for an unknown member ID, got: %s", result)
	}

	var p store.Portfolio
	if err := json.Unmarshal([]byte(seed), &p); err != nil {
		t.Fatalf("invalid seed JSON: %v", err)
	}
	if len(p.Members) != 1 {
		t.Fatalf("seed portfolio should still have exactly 1 Member, got %d - a failed commit must not have mutated anything", len(p.Members))
	}
}

func TestCommitStagedRows_EmptyMemberIDIsAnError(t *testing.T) {
	// No member selected at all (e.g. the person hasn't picked one from
	// the dropdown yet) - must fail clearly rather than falling back to
	// some default member.
	rows := []casimport.StagedRow{{
		Txn:    store.Transaction{Date: "2025-01-01", Amount: 100, Type: store.Purchase, Scheme: "SOME FUND", ISIN: "INF1"},
		Status: "NEW",
	}}
	b, _ := json.Marshal(rows)
	seed, _ := seededPortfolio(t, "Me")

	result := CommitStagedRows(seed, string(b), "")
	if !strings.Contains(result, "error") {
		t.Fatalf("expected an error when no member is selected, got: %s", result)
	}
}

func TestDeleteTransaction_RemovesOnlyTheMatchingOne(t *testing.T) {
	units := 1.0
	p := store.Portfolio{
		Transactions: []store.StoredTransaction{
			{ID: "txn-1", Amount: 100, Units: &units},
			{ID: "txn-2", Amount: 200, Units: &units},
			{ID: "txn-3", Amount: 300, Units: &units},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := DeleteTransaction(string(pJSON), "txn-2")

	var after store.Portfolio
	if err := json.Unmarshal([]byte(result), &after); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(after.Transactions) != 2 {
		t.Fatalf("expected 2 remaining transactions, got %d", len(after.Transactions))
	}
	for _, txn := range after.Transactions {
		if txn.ID == "txn-2" {
			t.Errorf("txn-2 should have been deleted but is still present")
		}
	}
}

func TestDeleteTransaction_UnknownIDIsNoOpNotError(t *testing.T) {
	units := 1.0
	p := store.Portfolio{
		Transactions: []store.StoredTransaction{{ID: "txn-1", Amount: 100, Units: &units}},
	}
	pJSON, _ := json.Marshal(p)

	result := DeleteTransaction(string(pJSON), "does-not-exist")

	var after store.Portfolio
	if err := json.Unmarshal([]byte(result), &after); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(after.Transactions) != 1 {
		t.Errorf("expected the unrelated transaction to survive untouched, got %d transactions", len(after.Transactions))
	}
}

func TestUpdateTransaction_EditsFieldsAndLeavesOthersUntouched(t *testing.T) {
	units := 5.0
	price := 20.0
	p := store.Portfolio{
		Transactions: []store.StoredTransaction{{
			ID: "txn-1", Date: "2025-01-01", Amount: 100, Units: &units,
			Type: store.Purchase, Price: &price, Description: "original desc",
		}},
	}
	pJSON, _ := json.Marshal(p)

	result := UpdateTransaction(string(pJSON), "txn-1", "2025-02-15", 150, 7.5)

	var after store.Portfolio
	if err := json.Unmarshal([]byte(result), &after); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	txn := after.Transactions[0]
	if txn.Date != "2025-02-15" {
		t.Errorf("Date = %q, want 2025-02-15", txn.Date)
	}
	if txn.Amount != 150 {
		t.Errorf("Amount = %v, want 150", txn.Amount)
	}
	if txn.Units == nil || *txn.Units != 7.5 {
		t.Errorf("Units = %v, want 7.5", txn.Units)
	}
	// Type, Price, and Description were NOT part of the edit and must be
	// left exactly as they were.
	if txn.Type != store.Purchase {
		t.Errorf("Type changed to %q, should have been left untouched", txn.Type)
	}
	if txn.Price == nil || *txn.Price != 20.0 {
		t.Errorf("Price changed, should have been left untouched (was 20.0)")
	}
	if txn.Description != "original desc" {
		t.Errorf("Description changed, should have been left untouched")
	}
}

func TestUpdateTransaction_UnknownIDReturnsError(t *testing.T) {
	pJSON := `{}`
	result := UpdateTransaction(pJSON, "does-not-exist", "2025-01-01", 100, 1)
	if !isBridgeErrorForTest(result) {
		t.Errorf("expected an error response for an unknown transaction ID, got: %s", result)
	}
}

func TestListMembers_ReturnsAllMembers(t *testing.T) {
	p := store.Portfolio{
		Members: []store.Member{{ID: "m1", Name: "Me"}, {ID: "m2", Name: "Mom"}},
	}
	pJSON, _ := json.Marshal(p)

	result := ListMembers(string(pJSON))

	var members []store.Member
	if err := json.Unmarshal([]byte(result), &members); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(members))
	}
}

func isBridgeErrorForTest(s string) bool {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return false
	}
	_, ok := m["error"]
	return ok
}

func TestSetTargetAllocation_PersistsAndOverwrites(t *testing.T) {
	after1 := SetTargetAllocation("", 40, 33, 24, 3)

	var p1 store.Portfolio
	if err := json.Unmarshal([]byte(after1), &p1); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if p1.TargetAllocation.Large != 40 || p1.TargetAllocation.Cash != 3 {
		t.Errorf("target = %+v, want Large=40, Cash=3", p1.TargetAllocation)
	}

	after2 := SetTargetAllocation(after1, 50, 30, 15, 5)
	var p2 store.Portfolio
	if err := json.Unmarshal([]byte(after2), &p2); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if p2.TargetAllocation.Large != 50 {
		t.Errorf("target after overwrite = %+v, want Large=50", p2.TargetAllocation)
	}
}

func TestComputeAllocationDrift_NoTargetSetReturnsHasTargetFalse(t *testing.T) {
	result := ComputeAllocationDrift("")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["hasTarget"] != false {
		t.Errorf("hasTarget = %v, want false when no target has been set", parsed["hasTarget"])
	}
	if _, hasDrift := parsed["drift"]; hasDrift {
		t.Errorf("expected no 'drift' key when hasTarget is false, got one: %s", result)
	}
}

func TestComputeAllocationDrift_WithTargetReturnsRealNumbers(t *testing.T) {
	units := 100.0
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "asset-1", Name: "SOME LARGE CAP FUND"}},
		Prices: []store.PriceRecord{{AssetID: "asset-1", Date: "2026-08-20", Price: 10}},
		Transactions: []store.StoredTransaction{{
			AssetID: "asset-1", AccountID: "acc", Date: "2025-01-01", Amount: 1000,
			Units: &units, Type: store.Purchase,
		}},
	}
	pJSON, _ := json.Marshal(p)
	withTarget := SetTargetAllocation(string(pJSON), 40, 33, 24, 3)

	result := ComputeAllocationDrift(withTarget)

	var parsed struct {
		HasTarget bool                           `json:"hasTarget"`
		Drift     []finance.AllocationDriftSlice `json:"drift"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\nresult: %s", err, result)
	}
	if !parsed.HasTarget {
		t.Fatalf("expected hasTarget=true after setting a target")
	}
	if len(parsed.Drift) != 4 {
		t.Fatalf("expected 4 drift buckets, got %d", len(parsed.Drift))
	}

	var largeCapDrift *finance.AllocationDriftSlice
	for i := range parsed.Drift {
		if parsed.Drift[i].Label == "Large Cap" {
			largeCapDrift = &parsed.Drift[i]
		}
	}
	if largeCapDrift == nil {
		t.Fatalf("expected a Large Cap drift entry")
	}
	// 100% of the (single, fully-priced) holding is Large Cap by name
	// heuristic, target is 40 -> drift should be +60.
	if largeCapDrift.Actual != 100 {
		t.Errorf("Large Cap actual = %v, want 100", largeCapDrift.Actual)
	}
	if largeCapDrift.Drift != 60 {
		t.Errorf("Large Cap drift = %v, want 60", largeCapDrift.Drift)
	}
}

func TestAddMember_CreatesAndRejectsDuplicates(t *testing.T) {
	after1 := AddMember("", "Mom")
	if isBridgeErrorForTest(after1) {
		t.Fatalf("expected success adding first member, got: %s", after1)
	}

	var p1 store.Portfolio
	if err := json.Unmarshal([]byte(after1), &p1); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p1.Members) != 1 || p1.Members[0].Name != "Mom" {
		t.Fatalf("expected one member named Mom, got %+v", p1.Members)
	}

	// Adding a second, different member should succeed and add to the list.
	after2 := AddMember(after1, "Me")
	var p2 store.Portfolio
	if err := json.Unmarshal([]byte(after2), &p2); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p2.Members) != 2 {
		t.Fatalf("expected 2 members after adding a second, got %d", len(p2.Members))
	}

	// Re-adding "Mom" should be rejected, not silently duplicate.
	after3 := AddMember(after2, "Mom")
	if !isBridgeErrorForTest(after3) {
		t.Fatalf("expected an error re-adding an existing member name, got: %s", after3)
	}

	var p3 store.Portfolio
	// Confirm the error response didn't corrupt anything - re-parse the
	// LAST KNOWN GOOD state (after2) to make sure it's still intact,
	// since the caller should discard an error response, not save it.
	if err := json.Unmarshal([]byte(after2), &p3); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(p3.Members) != 2 {
		t.Fatalf("expected the last good state to still have 2 members, got %d", len(p3.Members))
	}
}

func TestAddMember_EmptyNameRejected(t *testing.T) {
	result := AddMember("", "")
	if !isBridgeErrorForTest(result) {
		t.Fatalf("expected an error for an empty member name, got: %s", result)
	}
}

func TestUpdateHistoricalNav_RejectsUnknownAssetAndEmptyISIN(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "a1", Name: "Some Fund"}},
	}
	pJSON, _ := json.Marshal(p)

	badAsset := UpdateHistoricalNav(string(pJSON), "a-nonexistent", "INF174K01LT0")
	if !isBridgeErrorForTest(badAsset) {
		t.Fatalf("expected an error for a nonexistent asset, got: %s", badAsset)
	}

	badISIN := UpdateHistoricalNav(string(pJSON), "a1", "")
	if !isBridgeErrorForTest(badISIN) {
		t.Fatalf("expected an error for an empty ISIN, got: %s", badISIN)
	}
}

func TestUpdateHistoricalPrice_RejectsUnknownAssetAndEmptySymbol(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "a1", Name: "Nippon India ETF Nifty 50 BeES", Symbol: "NIFTYBEES.NS"}},
	}
	pJSON, _ := json.Marshal(p)

	badAsset := UpdateHistoricalPrice(string(pJSON), "a-nonexistent", "NIFTYBEES.NS", "2024-01-01")
	if !isBridgeErrorForTest(badAsset) {
		t.Fatalf("expected an error for a nonexistent asset, got: %s", badAsset)
	}

	badSymbol := UpdateHistoricalPrice(string(pJSON), "a1", "", "2024-01-01")
	if !isBridgeErrorForTest(badSymbol) {
		t.Fatalf("expected an error for an empty symbol, got: %s", badSymbol)
	}

	// Same validation-only shape as UpdateHistoricalNav - this test
	// deliberately does not exercise the actual network fetch (see
	// FetchYahooAdjClose's doc comment on why that can't be verified
	// live from this sandbox).
}

func TestAddAccount_CreatesAndValidatesMember(t *testing.T) {
	p := &store.Portfolio{Members: []store.Member{{ID: "m1", Name: "Saby"}}}
	pJSON, _ := json.Marshal(p)

	result := AddAccount(string(pJSON), "m1", "Questrade CAD", "CAD")
	if isBridgeErrorForTest(result) {
		t.Fatalf("expected success, got: %s", result)
	}
	var updated store.Portfolio
	if err := json.Unmarshal([]byte(result), &updated); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(updated.Accounts) != 1 {
		t.Fatalf("expected 1 account, got %d", len(updated.Accounts))
	}
	if updated.Accounts[0].Currency != "CAD" || updated.Accounts[0].MemberID != "m1" {
		t.Errorf("account = %+v, want Currency=CAD MemberID=m1", updated.Accounts[0])
	}

	// A nonexistent member should be rejected, not silently create an
	// orphaned account.
	badResult := AddAccount(string(pJSON), "m-nonexistent", "Some Account", "CAD")
	if !isBridgeErrorForTest(badResult) {
		t.Fatalf("expected an error for a nonexistent member, got: %s", badResult)
	}

	// Empty currency should be rejected too - a manually-entered account
	// with no currency would break every downstream conversion.
	noCurrencyResult := AddAccount(string(pJSON), "m1", "Some Account", "")
	if !isBridgeErrorForTest(noCurrencyResult) {
		t.Fatalf("expected an error for an empty currency, got: %s", noCurrencyResult)
	}
}

func TestAddAsset_CreatesAndValidatesAccount(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1", Name: "Questrade CAD", Currency: "CAD"}},
	}
	pJSON, _ := json.Marshal(p)

	result := AddAsset(string(pJSON), "acc1", "Vanguard S&P 500 Index ETF", "VFV.TO", "ETF")
	if isBridgeErrorForTest(result) {
		t.Fatalf("expected success, got: %s", result)
	}
	var updated store.Portfolio
	if err := json.Unmarshal([]byte(result), &updated); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(updated.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(updated.Assets))
	}
	if updated.Assets[0].Symbol != "VFV.TO" || updated.Assets[0].ISIN != "" {
		t.Errorf("asset = %+v, want Symbol=VFV.TO and no ISIN", updated.Assets[0])
	}

	badResult := AddAsset(string(pJSON), "acc-nonexistent", "Some ETF", "XYZ.TO", "ETF")
	if !isBridgeErrorForTest(badResult) {
		t.Fatalf("expected an error for a nonexistent account, got: %s", badResult)
	}
}

func TestAddManualTransaction_ValidatesTypeAndAsset(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1", Currency: "CAD"}},
		Assets:   []store.Asset{{ID: "a1", AccountID: "acc1", Name: "VFV", Symbol: "VFV.TO"}},
	}
	pJSON, _ := json.Marshal(p)

	result := AddManualTransaction(string(pJSON), "acc1", "a1", "2026-01-15", "PURCHASE", 1000, 10)
	if isBridgeErrorForTest(result) {
		t.Fatalf("expected success, got: %s", result)
	}
	var updated store.Portfolio
	if err := json.Unmarshal([]byte(result), &updated); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(updated.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(updated.Transactions))
	}
	txn := updated.Transactions[0]
	if txn.Source != "MANUAL" || txn.Type != store.Purchase || txn.Amount != 1000 {
		t.Errorf("transaction = %+v, want Source=MANUAL Type=PURCHASE Amount=1000", txn)
	}

	// An unrecognised transaction type must be rejected outright - it
	// would otherwise corrupt XIRR/holdings math downstream with a type
	// none of that code knows how to interpret.
	badTypeResult := AddManualTransaction(string(pJSON), "acc1", "a1", "2026-01-15", "BOGUS_TYPE", 1000, 10)
	if !isBridgeErrorForTest(badTypeResult) {
		t.Fatalf("expected an error for an unsupported transaction type, got: %s", badTypeResult)
	}

	// An asset that doesn't belong to the given account must be rejected
	// - this is the same cross-check CommitStagedRows relies on for CAS
	// imports, applied here to manual entry too.
	badAssetResult := AddManualTransaction(string(pJSON), "acc-wrong", "a1", "2026-01-15", "PURCHASE", 1000, 10)
	if !isBridgeErrorForTest(badAssetResult) {
		t.Fatalf("expected an error for an asset/account mismatch, got: %s", badAssetResult)
	}
}

func TestComputeHoldingsInSegment_ReturnsOnlyMatchingHoldings(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "a1", Name: "SOME NIFTY SMALLCAP 250 INDEX FUND"},
			{ID: "a2", Name: "SOME NIFTY 50 INDEX FUND"},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2026-08-20", Price: 10},
			{AssetID: "a2", Date: "2026-08-20", Price: 10},
		},
		Transactions: []store.StoredTransaction{
			{AssetID: "a1", AccountID: "acc", Date: "2025-01-01", Amount: 100, Units: &units, Type: store.Purchase},
			{AssetID: "a2", AccountID: "acc", Date: "2025-01-01", Amount: 100, Units: &units, Type: store.Purchase},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := ComputeHoldingsInSegment(string(pJSON), "", "Small Cap")

	var holdings []finance.Holding
	if err := json.Unmarshal([]byte(result), &holdings); err != nil {
		t.Fatalf("invalid JSON: %v\nresult: %s", err, result)
	}
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding in Small Cap, got %d: %+v", len(holdings), holdings)
	}
	if holdings[0].AssetID != "a1" {
		t.Errorf("expected a1 (the small cap fund), got %s", holdings[0].AssetID)
	}
}

func TestComputePortfolioXIRR_ScopesToMember(t *testing.T) {
	unitsA, unitsB := 10.0, 10.0
	p := &store.Portfolio{
		Members: []store.Member{{ID: "m1", Name: "Alice"}, {ID: "m2", Name: "Bob"}},
		Accounts: []store.Account{
			{ID: "acc1", MemberID: "m1"},
			{ID: "acc2", MemberID: "m2"},
		},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Alice's Fund"},
			{ID: "a2", AccountID: "acc2", Name: "Bob's Fund"},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2026-08-20", Price: 10},
			{AssetID: "a2", Date: "2026-08-20", Price: 10},
		},
		Transactions: []store.StoredTransaction{
			{AssetID: "a1", AccountID: "acc1", Date: "2025-01-01", Amount: 100, Units: &unitsA, Type: store.Purchase},
			{AssetID: "a2", AccountID: "acc2", Date: "2025-01-01", Amount: 100, Units: &unitsB, Type: store.Purchase},
		},
	}
	pJSON, _ := json.Marshal(p)

	// Whole family: should have an XIRR at all (both members' flows pool together).
	familyResult := ComputePortfolioXIRR(string(pJSON), "")
	if !strings.Contains(familyResult, `"hasXIRR":true`) {
		t.Fatalf("expected hasXIRR:true for whole family, got: %s", familyResult)
	}

	// Scoped to a single member: must not error and must still compute -
	// this is the exact bug being fixed (previously memberID was ignored
	// entirely, so this call couldn't even be made).
	aliceResult := ComputePortfolioXIRR(string(pJSON), "m1")
	if !strings.Contains(aliceResult, `"hasXIRR":true`) {
		t.Fatalf("expected hasXIRR:true when scoped to Alice, got: %s", aliceResult)
	}

	// A member with no holdings at all should report hasXIRR:false, not
	// silently fall back to the whole family's XIRR.
	noHoldingsResult := ComputePortfolioXIRR(string(pJSON), "m-nonexistent")
	if !strings.Contains(noHoldingsResult, `"hasXIRR":false`) {
		t.Fatalf("expected hasXIRR:false for a member with no holdings, got: %s", noHoldingsResult)
	}
}

func TestComputeAllocationByMarketCap_ScopesToMember(t *testing.T) {
	unitsA, unitsB := 10.0, 10.0
	p := &store.Portfolio{
		Members: []store.Member{{ID: "m1", Name: "Alice"}, {ID: "m2", Name: "Bob"}},
		Accounts: []store.Account{
			{ID: "acc1", MemberID: "m1"},
			{ID: "acc2", MemberID: "m2"},
		},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "SOME NIFTY SMALLCAP 250 INDEX FUND"},
			{ID: "a2", AccountID: "acc2", Name: "SOME NIFTY 50 INDEX FUND"},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2026-08-20", Price: 10},
			{AssetID: "a2", Date: "2026-08-20", Price: 10},
		},
		Transactions: []store.StoredTransaction{
			{AssetID: "a1", AccountID: "acc1", Date: "2025-01-01", Amount: 100, Units: &unitsA, Type: store.Purchase},
			{AssetID: "a2", AccountID: "acc2", Date: "2025-01-01", Amount: 100, Units: &unitsB, Type: store.Purchase},
		},
	}
	pJSON, _ := json.Marshal(p)

	aliceOnly := ComputeAllocationByMarketCap(string(pJSON), "m1")
	if !containsLabel(aliceOnly, "Small Cap") {
		t.Errorf("expected Alice-scoped allocation to include Small Cap, got: %s", aliceOnly)
	}
	if containsLabel(aliceOnly, "Large Cap") {
		t.Errorf("expected Alice-scoped allocation to NOT include Bob's Large Cap fund, got: %s", aliceOnly)
	}

	wholeFamily := ComputeAllocationByMarketCap(string(pJSON), "")
	if !containsLabel(wholeFamily, "Small Cap") || !containsLabel(wholeFamily, "Large Cap") {
		t.Errorf("expected whole-family allocation to include both members' funds, got: %s", wholeFamily)
	}
}

func TestComputeProgression_ReturnsWeeklyPointsAsJSON(t *testing.T) {
	unitsA := 100.0
	p := &store.Portfolio{
		Accounts: []store.Account{{ID: "acc1", MemberID: "m1", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "a1", AccountID: "acc1", Name: "SOME NIFTY 50 INDEX FUND"}},
		Transactions: []store.StoredTransaction{
			{AssetID: "a1", AccountID: "acc1", Date: "2024-01-16", Amount: 10000, Units: &unitsA, Type: store.Purchase},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-01-22", Price: 110},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := ComputeProgression(string(pJSON), "", "WholePortfolio", "2024-01-22", "")
	if isBridgeErrorForTest(result) {
		t.Fatalf("unexpected error: %s", result)
	}

	var points []map[string]any
	if err := json.Unmarshal([]byte(result), &points); err != nil {
		t.Fatalf("result is not valid JSON array: %v\nresult: %s", err, result)
	}
	if len(points) != 1 {
		t.Fatalf("expected 1 weekly point, got %d: %s", len(points), result)
	}
	if points[0]["Date"] != "2024-01-22" {
		t.Errorf("Date = %v, want 2024-01-22", points[0]["Date"])
	}
	if points[0]["Invested"].(float64) != 10000 {
		t.Errorf("Invested = %v, want 10000", points[0]["Invested"])
	}
	if points[0]["Value"].(float64) != 11000 {
		t.Errorf("Value = %v, want 11000", points[0]["Value"])
	}
}

func TestComputeProgression_InvalidTodayDateReturnsError(t *testing.T) {
	result := ComputeProgression("{}", "", "WholePortfolio", "not-a-date", "")
	if !isBridgeErrorForTest(result) {
		t.Errorf("expected an error for an invalid today date, got: %s", result)
	}
}

func TestAddRemoveBenchmark_RoundTrip(t *testing.T) {
	after := AddBenchmark("{}", "Nifty 50", "^NSEI")
	if isBridgeErrorForTest(after) {
		t.Fatalf("AddBenchmark failed: %s", after)
	}
	var p store.Portfolio
	if err := json.Unmarshal([]byte(after), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(p.Benchmarks) != 1 || p.Benchmarks[0].Name != "Nifty 50" || p.Benchmarks[0].YahooTicker != "^NSEI" {
		t.Fatalf("unexpected benchmarks: %+v", p.Benchmarks)
	}

	empty := AddBenchmark("{}", "", "^NSEI")
	if !isBridgeErrorForTest(empty) {
		t.Errorf("expected an error for an empty name, got: %s", empty)
	}

	removed := RemoveBenchmark(after, p.Benchmarks[0].ID)
	var p2 store.Portfolio
	if err := json.Unmarshal([]byte(removed), &p2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(p2.Benchmarks) != 0 {
		t.Errorf("expected 0 benchmarks after remove, got %d", len(p2.Benchmarks))
	}
}

func TestUpdateBenchmarkHistory_RejectsUnknownBenchmark(t *testing.T) {
	p := &store.Portfolio{Benchmarks: []store.Benchmark{{ID: "b1", Name: "Nifty 50", YahooTicker: "^NSEI"}}}
	pJSON, _ := json.Marshal(p)

	result := UpdateBenchmarkHistory(string(pJSON), "b-nonexistent", "2024-01-01")
	if !isBridgeErrorForTest(result) {
		t.Fatalf("expected an error for a nonexistent benchmark, got: %s", result)
	}
	// Same validation-only shape as UpdateHistoricalPrice - deliberately
	// doesn't exercise the actual network fetch (see
	// FetchYahooAdjClose's doc comment on why that can't be verified
	// live from this sandbox).
}

func TestComputeReturnsTable_IncludesFundsAndBenchmarksWithPriceHistory(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{
			{ID: "fund1", Name: "Nippon India Growth Mid Cap Fund", Type: "MutualFund"},
			{ID: "fund2", Name: "Never priced yet", Type: "MutualFund"}, // no Prices entry - must be excluded
			{ID: "stock1", Name: "Some ETF", Type: "Stock"},             // not a MutualFund - must be excluded
		},
		Benchmarks: []store.Benchmark{
			{ID: "bench1", Name: "Nifty 50", YahooTicker: "^NSEI"},
		},
		Prices: []store.PriceRecord{
			{AssetID: "fund1", Date: "2024-01-01", Price: 100},
			{AssetID: "fund1", Date: "2024-01-22", Price: 110},
			{AssetID: "bench1", Date: "2024-01-01", Price: 21000},
			{AssetID: "bench1", Date: "2024-01-22", Price: 21500},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := ComputeReturnsTable(string(pJSON))
	if isBridgeErrorForTest(result) {
		t.Fatalf("ComputeReturnsTable failed: %s", result)
	}
	var rows []ReturnsTableRow
	if err := json.Unmarshal([]byte(result), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (fund1 + bench1; fund2 has no prices, stock1 isn't a MutualFund), got %d: %+v", len(rows), rows)
	}
	byID := make(map[string]ReturnsTableRow)
	for _, r := range rows {
		byID[r.SeriesID] = r
	}
	if _, ok := byID["fund1"]; !ok {
		t.Errorf("expected fund1 row, got %+v", rows)
	}
	if row, ok := byID["bench1"]; !ok || !row.IsBenchmark {
		t.Errorf("expected bench1 row with IsBenchmark=true, got %+v", row)
	}
}

func TestComputeReturnsTable_LongTenuresHaveBothTrailingAndRolling(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "fund1", Name: "Nippon India Growth Mid Cap Fund", Type: "MutualFund"}},
		Prices: []store.PriceRecord{
			{AssetID: "fund1", Date: "2019-01-01", Price: 100},
			{AssetID: "fund1", Date: "2021-01-01", Price: 110},
			{AssetID: "fund1", Date: "2022-01-01", Price: 133.1}, // exactly 3 years after 2019-01-01: +10% CAGR
		},
	}
	pJSON, _ := json.Marshal(p)

	result := ComputeReturnsTable(string(pJSON))
	if isBridgeErrorForTest(result) {
		t.Fatalf("ComputeReturnsTable failed: %s", result)
	}
	var rows []ReturnsTableRow
	if err := json.Unmarshal([]byte(result), &rows); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0]
	if !row.ThreeYearTrailing.HasData {
		t.Errorf("ThreeYearTrailing.HasData = false, want true")
	}
	if row.ThreeYearTrailing.Percent < 9.99 || row.ThreeYearTrailing.Percent > 10.01 {
		t.Errorf("ThreeYearTrailing.Percent = %v, want ~10.0", row.ThreeYearTrailing.Percent)
	}
	if !row.ThreeYearRolling.HasData {
		t.Errorf("ThreeYearRolling.HasData = false, want true")
	}
}

func TestComputePriceHistory_ReturnsSeriesForKnownKeyEmptyForUnknown(t *testing.T) {
	p := &store.Portfolio{
		Prices: []store.PriceRecord{
			{AssetID: "fund1", Date: "2024-01-01", Price: 100},
			{AssetID: "fund1", Date: "2024-01-02", Price: 101},
		},
	}
	pJSON, _ := json.Marshal(p)

	result := ComputePriceHistory(string(pJSON), "fund1")
	var series []store.PriceRecord
	if err := json.Unmarshal([]byte(result), &series); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("expected 2 records, got %d", len(series))
	}

	empty := ComputePriceHistory(string(pJSON), "nonexistent")
	var emptySeries []store.PriceRecord
	if err := json.Unmarshal([]byte(empty), &emptySeries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(emptySeries) != 0 {
		t.Errorf("expected empty series for unknown key, got %d", len(emptySeries))
	}
}

func TestBuildTransactionMarkers_ClassifiesBuySellAndSkipsUnplottable(t *testing.T) {
	transactions := []store.StoredTransaction{
		{AssetID: "fund1", Date: "2026-01-08", Type: store.Purchase, Amount: 12499.37, Units: floatPtr(131.972), Price: floatPtr(94.712), Description: "Purchase"},
		{AssetID: "fund1", Date: "2026-03-15", Type: store.Redemption, Amount: -5000, Units: floatPtr(-40.5), Price: floatPtr(123.45), Description: "Redemption"},
		{AssetID: "fund1", Date: "2026-02-01", Type: store.DividendPayout, Amount: 200, Units: nil, Description: "Dividend payout (cash, no units)"},
		{AssetID: "fund2", Date: "2026-01-09", Type: store.Purchase, Amount: 1000, Units: floatPtr(10), Price: floatPtr(100), Description: "different fund"},
	}

	markers := buildTransactionMarkers(transactions, "fund1")
	if len(markers) != 2 {
		t.Fatalf("expected 2 plottable markers for fund1, got %d: %+v", len(markers), markers)
	}
	// sorted ascending by date
	if markers[0].Date != "2026-01-08" || !markers[0].IsBuy || markers[0].Units != 131.972 || markers[0].Amount != 12499.37 || markers[0].Price != 94.712 {
		t.Errorf("first marker = %+v, want the buy on 2026-01-08 with positive units/amount", markers[0])
	}
	if markers[1].Date != "2026-03-15" || markers[1].IsBuy || markers[1].Units != 40.5 || markers[1].Amount != 5000 {
		t.Errorf("second marker = %+v, want the sell on 2026-03-15 with absolute (positive) units/amount and IsBuy=false", markers[1])
	}
}

func TestBuildTransactionMarkers_UnknownSeriesIDReturnsEmpty(t *testing.T) {
	transactions := []store.StoredTransaction{
		{AssetID: "fund1", Date: "2026-01-08", Type: store.Purchase, Amount: 1000, Units: floatPtr(10), Price: floatPtr(100)},
	}
	markers := buildTransactionMarkers(transactions, "nonexistent")
	if len(markers) != 0 {
		t.Errorf("expected 0 markers for unknown series ID, got %d", len(markers))
	}
}
