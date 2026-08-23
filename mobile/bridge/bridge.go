// Package bridge is the ONLY package meant to be handed to `gomobile bind`.
//
// Design principle: gobind's struct/generics marshaling across the JNI
// boundary is limited and has historically been fragile across Go/NDK
// version bumps, so this package exposes the smallest possible surface —
// a handful of functions taking/returning plain strings ([]byte for the
// PDF) — and does all the real work by calling straight into the existing,
// already-tested internal/ packages. Kotlin never sees a Go struct
// directly; it only ever sees JSON it already knows how to parse.
//
// This is a PROTOTYPE to validate the toolchain end-to-end (Go -> .aar ->
// Kotlin), not a final API. Once you confirm on your machine that
// `gomobile bind` actually produces a working .aar with these functions
// callable from Kotlin, we can decide whether to extend this surface or
// restructure it.
package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"ledger/internal/casimport"
	"ledger/internal/csvimport"
	"ledger/internal/finance"
	"ledger/internal/priceapi"
	"ledger/internal/store"

	"github.com/ledongthuc/pdf"
)

// ImportCASResult is the JSON shape returned by ImportCAS.
type ImportCASResult struct {
	Format       string                     `json:"format"`
	Staged       []casimport.StagedRow      `json:"staged"`
	ManualReview []casimport.ManualReviewLine `json:"manualReview"`
	Error        string                     `json:"error,omitempty"`
}

// ImportCAS parses a CAS PDF (raw bytes, e.g. read from a content:// URI on
// Android via ContentResolver.openInputStream) and returns an
// ImportCASResult as a JSON string.
//
// gomobile bind maps a Go []byte parameter to a Kotlin ByteArray directly,
// so the Android side does not need to write the PDF to a temp file first —
// it can pass the bytes it already has in memory. This uses
// pdf.NewReader(io.ReaderAt, size), NOT pdf.Open(path), specifically so no
// filesystem path is required.
func ImportCAS(pdfBytes []byte) string {
	result := runImportCAS(pdfBytes)
	out, err := json.Marshal(result)
	if err != nil {
		// Marshal of our own struct should never fail, but never return
		// an unhandled error across the JNI boundary either way.
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ImportCSV parses a mutual-fund transaction CSV export (raw bytes, e.g.
// read from a content:// URI on Android) and returns an ImportCASResult
// as a JSON string - deliberately the SAME result shape ImportCAS
// returns, so the CSV-sourced staged rows flow through the exact same
// review/commit UI on the Kotlin side (TransactionAdapter,
// CommitStagedRows) with no separate code path needed there.
//
// See internal/csvimport's package doc comment for the column-matching
// approach (name/alias-based, not position-based) that makes this work
// across different platforms' CSV layouts rather than one hardcoded
// format.
func ImportCSV(csvBytes []byte) string {
	result := csvimport.ParseCSV(csvBytes)
	out, err := json.Marshal(ImportCASResult{
		Format:       result.Format,
		Staged:       result.Staged,
		ManualReview: result.ManualReview,
	})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

func runImportCAS(pdfBytes []byte) ImportCASResult {
	reader := bytes.NewReader(pdfBytes)
	r, err := pdf.NewReader(reader, int64(len(pdfBytes)))
	if err != nil {
		return ImportCASResult{Error: "could not open PDF: " + err.Error()}
	}

	numPages := r.NumPage()
	pageTexts := make([]string, 0, numPages)
	fonts := make(map[string]*pdf.Font)

	for pageIdx := 1; pageIdx <= numPages; pageIdx++ {
		page := r.Page(pageIdx)
		if page.V.IsNull() {
			continue
		}
		for _, name := range page.Fonts() {
			if _, ok := fonts[name]; !ok {
				f := page.Font(name)
				fonts[name] = &f
			}
		}
		text, err := page.GetPlainText(fonts)
		if err != nil {
			// Same "never silently drop" principle as cmd/cascli: surface
			// the page error via ManualReview-style reporting rather than
			// aborting the whole import. For this prototype we just skip
			// the page; a real port should thread this through properly.
			continue
		}
		pageTexts = append(pageTexts, text)
	}

	fullText := ""
	for _, t := range pageTexts {
		fullText += t + "\n"
	}
	format := casimport.DetectFormat(fullText)

	if format != "MFCENTRAL" {
		return ImportCASResult{
			Format: format,
			Error:  "only MFCentral-format CAS PDFs are supported (see PROJECT_HANDOFF.md — native CAMS/KFintech was never built, no real sample file was available to test against)",
		}
	}

	result := casimport.ParseMFCentral(pageTexts)
	return ImportCASResult{
		Format:       result.Format,
		Staged:       result.Staged,
		ManualReview: result.ManualReview,
	}
}

// ComputeXIRR takes a JSON array of {"date":"YYYY-MM-DD","amount":float}
// cash flows and returns a JSON object {"rate":float,"converged":bool}.
func ComputeXIRR(cashFlowsJSON string) string {
	var flows []finance.CashFlow
	if err := json.Unmarshal([]byte(cashFlowsJSON), &flows); err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid cash flows JSON: "+err.Error())
	}
	rate, converged := finance.XIRR(flows)
	out, _ := json.Marshal(map[string]any{"rate": rate, "converged": converged})
	return string(out)
}

// ComputeHoldings takes a JSON-serialized store.Portfolio and returns a JSON
// array of finance.Holding — the same aggregation the desktop app's
// Portfolio tab uses.
func ComputeHoldings(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.ComputeHoldings(&p)
	out, err := json.Marshal(holdings)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAllocationByMarketCap takes a JSON-serialized store.Portfolio and
// returns a JSON array of finance.AllocationSlice using each asset's
// CapComposition where present, falling back to the GuessMarketCapSegment
// heuristic otherwise (same behavior as the desktop Allocation tab).
// memberID scopes to one member's holdings; empty means the whole family.
func ComputeAllocationByMarketCap(portfolioJSON string, memberID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.FilterHoldingsByMember(finance.ComputeHoldings(&p), memberID)

	compByAsset := make(map[string]store.CapComposition)
	for _, a := range p.Assets {
		if c, ok := p.GetCapComposition(a.ID); ok {
			compByAsset[a.ID] = c
		}
	}

	slices := finance.AllocationByMarketCapSegment(holdings, compByAsset)
	out, err := json.Marshal(slices)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// Ping is the minimal smoke-test function: call this first from Kotlin to
// confirm the .aar loaded and the JNI bridge works at all, before trying
// anything that touches real data.
func Ping() string {
	return "bridge ok"
}

// SetCapComposition records (or overwrites) the real Large/Mid/Small/Cash
// factsheet breakdown for one asset, mirroring the desktop app's manual
// entry workflow (deliberately manual, not auto-scraped - some AMCs'
// robots.txt blocks scraping their factsheets anyway). Returns the
// updated portfolio as JSON.
func SetCapComposition(portfolioJSON string, assetID string, large, mid, small, cash float64, asOf, source string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetCapComposition(assetID, large, mid, small, cash, asOf, source)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// RefreshAmfiPrices fetches the current AMFI NAV file over the network
// (real HTTP call - requires the Android app to hold the INTERNET
// permission) and updates the portfolio's PriceRecords for any Asset
// whose ISIN matches an AMFI record. Returns the updated portfolio JSON,
// same pattern as CommitStagedRows - the caller is responsible for
// calling SavePortfolio afterward.
func RefreshAmfiPrices(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}

	records, err := priceapi.FetchAmfiNav()
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "fetching AMFI NAV file: "+err.Error())
	}

	matched := applyAmfiRecords(&p, records)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	// Prepend the match count as a sibling top-level field isn't possible
	// once p is already marshaled, so wrap it instead.
	return fmt.Sprintf(`{"matchedCount":%d,"portfolio":%s}`, matched, string(out))
}

// applyAmfiRecords matches AMFI NavRecords against the portfolio's Assets
// by ISIN (checking both the payout and reinvest ISIN columns, since a
// fund's own ISIN could be listed under either depending on its
// distribution option) and upserts a PriceRecord for each match. Split
// out from RefreshAmfiPrices specifically so it can be unit-tested with
// synthetic records, without needing a real network call.
func applyAmfiRecords(p *store.Portfolio, records []priceapi.NavRecord) (matchedCount int) {
	byISIN := make(map[string]priceapi.NavRecord, len(records)*2)
	for _, r := range records {
		if r.ISINPayout != "" {
			byISIN[r.ISINPayout] = r
		}
		if r.ISINReinvest != "" {
			byISIN[r.ISINReinvest] = r
		}
	}

	for _, asset := range p.Assets {
		if asset.ISIN == "" {
			continue
		}
		rec, ok := byISIN[asset.ISIN]
		if !ok {
			continue
		}
		isoDate, ok := amfiDateToISO(rec.Date)
		if !ok {
			continue
		}
		upsertPriceRecord(p, asset.ID, isoDate, rec.NAV)
		matchedCount++
	}
	return matchedCount
}

// upsertPriceRecord replaces any existing PriceRecord for the same
// AssetID+Date (so refreshing twice on the same day doesn't accumulate
// duplicate rows), or appends a new one otherwise.
func upsertPriceRecord(p *store.Portfolio, assetID, isoDate string, price float64) {
	for i := range p.Prices {
		if p.Prices[i].AssetID == assetID && p.Prices[i].Date == isoDate {
			p.Prices[i].Price = price
			p.Prices[i].Source = "AMFI"
			return
		}
	}
	p.Prices = append(p.Prices, store.PriceRecord{
		AssetID: assetID, Date: isoDate, Price: price, Source: "AMFI",
	})
}

// amfiDateToISO converts AMFI's printed date format ("20-Aug-2026") to
// ISO yyyy-mm-dd. This matters beyond cosmetics: finance.ComputeHoldings
// picks the "latest" price for an asset via plain string comparison of
// the Date field, which only gives the right answer if dates are stored
// in a lexicographically sortable (ISO) format.
func amfiDateToISO(s string) (string, bool) {
	t, err := time.Parse("02-Jan-2006", s)
	if err != nil {
		return "", false
	}
	return t.Format("2006-01-02"), true
}

// ComputePortfolioXIRR computes the single pooled XIRR across the whole
// portfolio (or whatever subset of holdings was passed in), matching the
// desktop app's PortfolioXIRR. Returns {"xirr":..,"hasXIRR":bool}.
// ComputePortfolioXIRR computes XIRR across holdings for the given member
// (empty memberID means the whole family). Previously this always
// computed across every holding regardless of memberID, even though
// finance.PortfolioXIRR itself already scopes correctly to whatever
// holdings slice it's given - the missing piece was simply not filtering
// by member before calling it.
func ComputePortfolioXIRR(portfolioJSON string, memberID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.FilterHoldingsByMember(finance.ComputeHoldings(&p), memberID)
	rate, ok := finance.PortfolioXIRR(&p, holdings)
	out, err := json.Marshal(map[string]any{"xirr": rate, "hasXIRR": ok})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// LoadPortfolio reads the portfolio JSON file at the given path (an
// Android app's own filesDir path, passed in from Kotlin) and returns it
// as a JSON string. A missing file returns an empty portfolio, not an
// error - this matches store.Load's own first-run behavior.
func LoadPortfolio(path string) string {
	p, err := store.Load(path)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SavePortfolio writes the given portfolio JSON to the given path
// (backing up any existing file first, same as the desktop app).
// Returns {"ok":true} or {"error":"..."}.
func SavePortfolio(path string, portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if err := store.Save(path, &p); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return `{"ok":true}`
}

// CommitStagedRows links the raw StagedRow output of ImportCAS into a
// real Portfolio: only rows with Status "NEW" are committed (DUPLICATE
// and UNMATCHED rows are left for the user to resolve, same principle as
// the desktop Import tab). A Member with the given name is created if one
// doesn't already exist (case-sensitive exact match), along with a
// default "CAS Import" Account under that member. An empty memberName
// defaults to "Me".
//
// Assets are matched by ISIN scoped to the target account, NOT globally
// across the whole portfolio: two different members holding the same
// fund (same ISIN) must NOT be silently merged onto whichever account
// happened to create the Asset first - store.Portfolio.FindAssetByISIN
// is deliberately not used here for that reason.
//
// Before appending, each row is also checked against transactions
// ALREADY in the portfolio for the same asset (see isDuplicateTransaction)
// and skipped if a matching one already exists. This is what makes
// re-importing an overlapping or identical CAS statement safe - without
// it, every re-import silently doubled every transaction it re-parsed,
// since the staged-row "NEW" status only reflects that the PARSE step
// found nothing wrong with the row, not that it's actually new relative
// to what's already stored.
//
// Returns a JSON object {"committed": N, "skippedDuplicates": N,
// "portfolio": {...}} rather than the bare portfolio, so the caller can
// tell the person how many rows were actually added versus recognized
// as already present.
func CommitStagedRows(portfolioJSON string, stagedRowsJSON string, memberName string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	var rows []casimport.StagedRow
	if err := json.Unmarshal([]byte(stagedRowsJSON), &rows); err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid staged rows JSON: "+err.Error())
	}

	if memberName == "" {
		memberName = "Me"
	}
	const accountName = "CAS Import"

	var memberID string
	for _, m := range p.Members {
		if m.Name == memberName {
			memberID = m.ID
			break
		}
	}
	if memberID == "" {
		memberID = store.NewID("member")
		p.Members = append(p.Members, store.Member{ID: memberID, Name: memberName})
	}

	account, ok := p.FindAccountByName(memberID, accountName)
	if !ok {
		account = store.Account{ID: store.NewID("account"), MemberID: memberID, Name: accountName, Currency: "INR"}
		p.Accounts = append(p.Accounts, account)
	}

	committed := 0
	skippedDuplicates := 0
	for _, row := range rows {
		if row.Status != "NEW" {
			continue
		}
		txn := row.Txn

		asset, ok := findAssetByISINInAccount(&p, txn.ISIN, account.ID)
		if !ok {
			asset = store.Asset{
				ID:        store.NewID("asset"),
				AccountID: account.ID,
				Name:      txn.Scheme,
				ISIN:      txn.ISIN,
				Type:      "MutualFund",
			}
			p.Assets = append(p.Assets, asset)
		}

		if isDuplicateTransaction(p.Transactions, asset.ID, txn) {
			skippedDuplicates++
			continue
		}

		p.Transactions = append(p.Transactions, store.StoredTransaction{
			ID:          store.NewID("txn"),
			AccountID:   account.ID,
			AssetID:     asset.ID,
			Date:        txn.Date,
			Type:        txn.Type,
			Description: txn.Description,
			Amount:      txn.Amount,
			Units:       txn.Units,
			Price:       txn.Price,
			Source:      "CAS_IMPORT",
		})
		committed++
	}

	portfolioBytes, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	out, err := json.Marshal(struct {
		Committed         int             `json:"committed"`
		SkippedDuplicates int             `json:"skippedDuplicates"`
		Portfolio         json.RawMessage `json:"portfolio"`
	}{
		Committed:         committed,
		SkippedDuplicates: skippedDuplicates,
		Portfolio:         portfolioBytes,
	})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// isDuplicateTransaction reports whether a staged transaction being
// committed already exists among a portfolio's stored transactions for
// the same asset. Matched on Date + Type + Amount (within a small
// epsilon for floating-point rounding from repeated JSON round-trips),
// plus Units when BOTH sides have a value - CAS statements don't always
// print units for every row (e.g. tax/STT lines), so units is used as a
// tie-breaker when available rather than a hard requirement that would
// let genuinely-duplicate rows slip through whenever units happens to be
// missing on one side.
func isDuplicateTransaction(existing []store.StoredTransaction, assetID string, txn store.Transaction) bool {
	const amountEpsilon = 0.01
	const unitsEpsilon = 0.0005
	for _, e := range existing {
		if e.AssetID != assetID || e.Date != txn.Date || e.Type != txn.Type {
			continue
		}
		if math.Abs(e.Amount-txn.Amount) > amountEpsilon {
			continue
		}
		if e.Units != nil && txn.Units != nil && math.Abs(*e.Units-*txn.Units) > unitsEpsilon {
			continue
		}
		return true
	}
	return false
}

// findAssetByISINInAccount matches an Asset by ISIN scoped to a specific
// account - deliberately not store.Portfolio.FindAssetByISIN, which
// matches globally across the whole portfolio and would silently merge
// two different members' holdings of the same fund onto one account.
func findAssetByISINInAccount(p *store.Portfolio, isin string, accountID string) (store.Asset, bool) {
	if isin == "" {
		return store.Asset{}, false
	}
	for _, a := range p.Assets {
		if a.ISIN == isin && a.AccountID == accountID {
			return a, true
		}
	}
	return store.Asset{}, false
}

// ListMembers returns the portfolio's Members as JSON, for a member
// picker/filter UI.
func ListMembers(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	out, err := json.Marshal(p.Members)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeHoldingsForMember is ComputeHoldings filtered to one member, via
// the same finance.FilterHoldingsByMember the desktop app uses. An empty
// memberID returns all holdings unfiltered (the "whole family" view).
func ComputeHoldingsForMember(portfolioJSON string, memberID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.FilterHoldingsByMember(finance.ComputeHoldings(&p), memberID)
	out, err := json.Marshal(holdings)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// DeleteTransaction removes the StoredTransaction with the given ID.
// Returns the updated portfolio as JSON. If no transaction with that ID
// exists, the portfolio is returned unchanged (not an error) - deleting
// something already gone is a no-op, not a failure.
func DeleteTransaction(portfolioJSON string, txnID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	kept := p.Transactions[:0]
	for _, t := range p.Transactions {
		if t.ID != txnID {
			kept = append(kept, t)
		}
	}
	p.Transactions = kept

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// UpdateTransaction edits the date, amount, and units of an existing
// StoredTransaction in place (matched by ID). Price and Type are left
// untouched - correcting a mistyped amount or date doesn't mean the
// transaction type or recorded price changed. Returns
// {"error":"transaction not found"} if txnID doesn't match anything, so
// the caller can tell "nothing changed because it succeeded" apart from
// "nothing changed because the ID was wrong".
func UpdateTransaction(portfolioJSON string, txnID string, date string, amount float64, units float64) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	found := false
	for i := range p.Transactions {
		if p.Transactions[i].ID == txnID {
			p.Transactions[i].Date = date
			p.Transactions[i].Amount = amount
			p.Transactions[i].Units = &units
			found = true
			break
		}
	}
	if !found {
		return `{"error":"transaction not found"}`
	}

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetTargetAllocation records the person's own chosen target market-cap
// mix. Returns the updated portfolio as JSON.
func SetTargetAllocation(portfolioJSON string, large, mid, small, cash float64) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.TargetAllocation = store.TargetAllocation{Large: large, Mid: mid, Small: small, Cash: cash}

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAllocationDrift returns the actual-vs-target comparison for the
// four market-cap buckets a target can be set for. Returns
// {"hasTarget":false} (no drift array) if no target has been entered yet
// - showing a zero-drift comparison against an unset target would be
// misleading, since "no target set" and "target is exactly met" are very
// different things.
func ComputeAllocationDrift(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if !p.TargetAllocation.HasTarget() {
		return `{"hasTarget":false}`
	}

	holdings := finance.ComputeHoldings(&p)
	compByAsset := make(map[string]store.CapComposition)
	for _, a := range p.Assets {
		if c, ok := p.GetCapComposition(a.ID); ok {
			compByAsset[a.ID] = c
		}
	}
	actual := finance.AllocationByMarketCapSegment(holdings, compByAsset)
	drift := finance.AllocationDrift(actual, p.TargetAllocation)

	driftJSON, err := json.Marshal(drift)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return fmt.Sprintf(`{"hasTarget":true,"drift":%s}`, string(driftJSON))
}

// UpdateHistoricalNav fetches a mutual fund asset's full NAV history
// from TigZig (by ISIN) and merges it into the portfolio's cached
// Prices, upserting rather than duplicating any date already present.
// This is the manual "Update History" action for the Indian side of
// the progression feature - not called automatically, same pattern as
// the existing AMFI current-price refresh. Returns the updated
// portfolio JSON, or a bridge error string if the asset doesn't exist
// or the fetch/parse fails.
func UpdateHistoricalNav(portfolioJSON string, assetID string, isin string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	assetFound := false
	for _, a := range p.Assets {
		if a.ID == assetID {
			assetFound = true
			break
		}
	}
	if !assetFound {
		return `{"error":"no asset with that ID exists"}`
	}
	if isin == "" {
		return `{"error":"ISIN cannot be empty"}`
	}

	history, err := priceapi.FetchTigzigNavHistory(isin)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	records := make([]store.PriceRecord, 0, len(history.Data))
	for _, pt := range history.Data {
		records = append(records, store.PriceRecord{
			AssetID: assetID, Date: pt.Date, Price: pt.Nav, Source: "TIGZIG_HISTORY",
		})
	}
	p.UpsertPrices(records)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// UpdateHistoricalFX fetches daily INR exchange rates for a currency
// from Frankfurter (ECB-sourced, from the given date to today) and
// merges them into the portfolio's cached FXRates. Manual action, same
// "Update History" pattern as UpdateHistoricalNav. Returns the updated
// portfolio JSON, or a bridge error string if the fetch/parse fails.
func UpdateHistoricalFX(portfolioJSON string, currency string, since string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}

	rates, err := priceapi.FetchFrankfurterHistory(currency, since)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	p.UpsertFXRates(rates)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddAccount creates a new Account for a given member, e.g. a foreign
// brokerage account that has no CAS-equivalent import (a Canadian
// brokerage account for CAD-denominated ETFs, unlike Indian mutual funds
// which arrive via ImportCAS). Returns the updated portfolio as JSON.
func AddAccount(portfolioJSON string, memberID string, name string, currency string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if name == "" {
		return `{"error":"name cannot be empty"}`
	}
	if currency == "" {
		return `{"error":"currency cannot be empty"}`
	}
	memberFound := false
	for _, m := range p.Members {
		if m.ID == memberID {
			memberFound = true
			break
		}
	}
	if !memberFound {
		return `{"error":"no member with that ID exists"}`
	}
	if _, ok := p.FindAccountByName(memberID, name); ok {
		return `{"error":"an account with that name already exists for this member"}`
	}
	p.Accounts = append(p.Accounts, store.Account{
		ID: store.NewID("account"), MemberID: memberID, Name: name, Currency: currency,
	})

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddAsset creates a new Asset (e.g. a Yahoo-ticker ETF) under a given
// Account. ISIN is deliberately not required here - manually-entered
// non-Indian holdings identify by Symbol instead (e.g. "VFV.TO"), the
// same field CommitStagedRows leaves blank for these. Returns the
// updated portfolio as JSON.
func AddAsset(portfolioJSON string, accountID string, name string, symbol string, assetType string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if name == "" {
		return `{"error":"name cannot be empty"}`
	}
	accountFound := false
	for _, a := range p.Accounts {
		if a.ID == accountID {
			accountFound = true
			break
		}
	}
	if !accountFound {
		return `{"error":"no account with that ID exists"}`
	}
	p.Assets = append(p.Assets, store.Asset{
		ID: store.NewID("asset"), AccountID: accountID, Name: name, Symbol: symbol, Type: assetType,
	})

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddManualTransaction creates a new StoredTransaction directly (Source
// "MANUAL"), for holdings with no CAS-equivalent import - a Buy or Sell
// of a Canadian ETF, or a reinvested distribution (same underlying
// mechanics as an Indian fund's dividend reinvestment, just a different
// market). txnType must be "PURCHASE", "REDEMPTION", or
// "DIVIDEND_REINVEST" - any other value is rejected rather than silently
// accepted, since an unrecognised type would corrupt XIRR/holdings math
// downstream. Returns the updated portfolio as JSON.
func AddManualTransaction(portfolioJSON string, accountID string, assetID string, date string, txnType string, amount float64, units float64) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	switch store.TransactionType(txnType) {
	case store.Purchase, store.Redemption, store.DividendReinvest:
		// allowed
	default:
		return fmt.Sprintf(`{"error":%q}`, "unsupported transaction type for manual entry: "+txnType)
	}
	assetFound := false
	for _, a := range p.Assets {
		if a.ID == assetID && a.AccountID == accountID {
			assetFound = true
			break
		}
	}
	if !assetFound {
		return `{"error":"no matching asset found in that account"}`
	}
	if date == "" {
		return `{"error":"date cannot be empty"}`
	}

	p.Transactions = append(p.Transactions, store.StoredTransaction{
		ID: store.NewID("txn"), AccountID: accountID, AssetID: assetID, Date: date,
		Type: store.TransactionType(txnType), Amount: amount, Units: &units, Source: "MANUAL",
	})

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddMember creates a new Member with the given name, if one with that
// exact name doesn't already exist (matches CommitStagedRows' own
// member-matching rule, so a member added here and one created later via
// a CAS import under the same name resolve to the same person, not two).
// Returns the updated portfolio as JSON.
func AddMember(portfolioJSON string, name string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if name == "" {
		return `{"error":"name cannot be empty"}`
	}
	for _, m := range p.Members {
		if m.Name == name {
			return `{"error":"a member with that name already exists"}`
		}
	}
	p.Members = append(p.Members, store.Member{ID: store.NewID("member"), Name: name})

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAllocationByEquityOrigin returns a JSON array of
// finance.AllocationSlice splitting the portfolio's Equity-classified
// holdings into Indian vs. International, using each asset's
// EquityOriginComposition where present, defaulting to 100% Indian
// otherwise (see store.EquityOriginComposition's doc comment).
func ComputeAllocationByEquityOrigin(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.ComputeHoldings(&p)

	classByAsset := make(map[string]string, len(p.Assets))
	for _, a := range p.Assets {
		classByAsset[a.ID] = a.AssetClass
	}
	compByAsset := make(map[string]store.EquityOriginComposition)
	for _, a := range p.Assets {
		if c, ok := p.GetEquityOriginComposition(a.ID); ok {
			compByAsset[a.ID] = c
		}
	}

	slices := finance.AllocationByEquityOrigin(holdings, classByAsset, compByAsset)
	out, err := json.Marshal(slices)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetEquityOriginComposition records (or overwrites) the real Indian/
// International factsheet breakdown for one equity asset. Returns the
// updated portfolio as JSON.
func SetEquityOriginComposition(portfolioJSON string, assetID string, indian, international float64, asOf, source string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetEquityOriginComposition(assetID, indian, international, asOf, source)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAllocationByPortfolioClass returns a JSON array of
// finance.AllocationSlice grouping the WHOLE portfolio into
// Equity/Debt/Commodity/Others.
func ComputeAllocationByPortfolioClass(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.ComputeHoldings(&p)

	classByAsset := make(map[string]string, len(p.Assets))
	for _, a := range p.Assets {
		classByAsset[a.ID] = a.AssetClass
	}

	slices := finance.AllocationByPortfolioClass(holdings, classByAsset)
	out, err := json.Marshal(slices)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetPortfolioClassTarget records the person's own chosen target
// Equity/Debt/Commodity/Others mix. Returns the updated portfolio as
// JSON.
func SetPortfolioClassTarget(portfolioJSON string, equity, debt, commodity, others float64) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.PortfolioClassTarget = store.PortfolioClassTarget{Equity: equity, Debt: debt, Commodity: commodity, Others: others}

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputePortfolioClassDrift returns the actual-vs-target comparison for
// the four Equity/Debt/Commodity/Others buckets. Returns
// {"hasTarget":false} if no target has been entered yet, same convention
// as ComputeAllocationDrift.
func ComputePortfolioClassDrift(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if !p.PortfolioClassTarget.HasTarget() {
		return `{"hasTarget":false}`
	}

	holdings := finance.ComputeHoldings(&p)
	classByAsset := make(map[string]string, len(p.Assets))
	for _, a := range p.Assets {
		classByAsset[a.ID] = a.AssetClass
	}
	actual := finance.AllocationByPortfolioClass(holdings, classByAsset)
	drift := finance.PortfolioClassDrift(actual, p.PortfolioClassTarget)

	driftJSON, err := json.Marshal(drift)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return fmt.Sprintf(`{"hasTarget":true,"drift":%s}`, string(driftJSON))
}

// ComputeHoldingsInSegment returns holdings (member-filtered, same as
// ComputeHoldingsForMember) that contribute any nonzero amount to the
// given market-cap segment label - the same classification the donut
// chart and drift bars themselves use, not a separate reimplementation.
func ComputeHoldingsInSegment(portfolioJSON string, memberID string, segmentLabel string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.FilterHoldingsByMember(finance.ComputeHoldings(&p), memberID)

	compByAsset := make(map[string]store.CapComposition)
	for _, a := range p.Assets {
		if c, ok := p.GetCapComposition(a.ID); ok {
			compByAsset[a.ID] = c
		}
	}

	filtered := finance.HoldingsInSegment(holdings, compByAsset, segmentLabel)
	out, err := json.Marshal(filtered)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeProgression takes a JSON-serialized store.Portfolio and returns
// a JSON array of finance.ProgressionPoint — one point per weekly
// checkpoint (see finance.WeeklyDates) — for the requested axis, scoped
// to a member (empty memberID = whole family).
//
// axis must be one of "WholePortfolio", "IndianEquity",
// "InternationalEquity", "CombinedEquity" (see finance.ProgressionAxis);
// an unrecognized axis silently produces all-zero points rather than an
// error, since every case in the underlying switch is exhaustive over
// the four constants and there's no natural "invalid axis" value to
// distinguish from "WholePortfolio" without adding a fifth sentinel -
// callers should restrict the value they pass to the four constants
// above.
//
// today is the caller's own current local date as "YYYY-MM-DD" (not
// necessarily UTC "now") — passed in rather than computed inside Go so
// the weekly-checkpoint boundary (and the "append today" rule) matches
// what the person actually sees on their phone, not the build server's
// clock.
func ComputeProgression(portfolioJSON string, memberID string, axis string, today string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid today date: "+err.Error())
	}
	points := finance.ComputeProgression(&p, memberID, finance.ProgressionAxis(axis), t)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}
