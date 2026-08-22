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
	"time"

	"ledger/internal/casimport"
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
func ComputeAllocationByMarketCap(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.ComputeHoldings(&p)

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
func ComputePortfolioXIRR(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.ComputeHoldings(&p)
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
// the desktop Import tab). A single default Member ("Me") and Account
// ("CAS Import") are created if they don't already exist; Assets are
// matched by ISIN via the same FindAssetByISIN the desktop app uses, so
// re-importing an overlapping statement won't create duplicate assets.
// Returns the updated portfolio as JSON.
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
// Returns the updated portfolio as JSON.
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

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
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
