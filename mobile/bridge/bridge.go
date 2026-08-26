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
	"regexp"
	"strings"
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
	Format       string                       `json:"format"`
	Staged       []casimport.StagedRow        `json:"staged"`
	ManualReview []casimport.ManualReviewLine `json:"manualReview"`
	Error        string                       `json:"error,omitempty"`
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

// ComputeGroupedHoldings is ComputeHoldings' consolidated counterpart -
// see finance.GroupHoldingsByLabel's doc comment. memberID scopes to one
// member's holdings first (empty = whole family), same convention as
// ComputeAllocationByMarketCap.
func ComputeGroupedHoldings(portfolioJSON string, memberID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.FilterHoldingsByMember(finance.ComputeHoldings(&p), memberID)
	grouped := finance.GroupHoldingsByLabel(&p, holdings)
	out, err := json.Marshal(grouped)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetAssetSymbolAndType updates an asset's Symbol and Type - see
// store.Portfolio.SetAssetSymbolAndType's doc comment. Returns the
// updated portfolio as JSON.
func SetAssetSymbolAndType(portfolioJSON string, assetID string, symbol string, assetType string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetAssetSymbolAndType(assetID, symbol, assetType)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetAssetGroupLabel records (or clears, if label is "") the fund-group
// label for an asset - see store.Asset.GroupLabel's doc comment. Returns
// the updated portfolio as JSON.
func SetAssetGroupLabel(portfolioJSON string, assetID string, label string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetAssetGroupLabel(assetID, label)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetAssetTags replaces the full tag list for an asset - see
// store.Asset.Tags' doc comment and store.Portfolio.SetAssetTags.
// tagsJSON is a JSON-encoded array of strings, e.g. ["Mid Cap","Growth"]
// - gomobile bind can't pass a Go/Kotlin string list directly across the
// bridge (only basic scalar types and strings), so this follows the same
// JSON-string convention every other non-trivial bridge parameter/return
// already uses here. An empty tagsJSON clears all tags. Returns the
// updated portfolio as JSON.
func SetAssetTags(portfolioJSON string, assetID string, tagsJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	var tags []string
	if tagsJSON != "" {
		if err := json.Unmarshal([]byte(tagsJSON), &tags); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid tags JSON: "+err.Error())
		}
	}
	p.SetAssetTags(assetID, tags)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetAssetPrimaryTag records (or clears, if tag is "") the pie-chart
// exclusivity override for an asset - see store.Asset.PrimaryTag's doc
// comment. Returns the updated portfolio as JSON.
func SetAssetPrimaryTag(portfolioJSON string, assetID string, tag string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetAssetPrimaryTag(assetID, tag)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAllTags returns a JSON array of every distinct tag currently
// used by at least one asset, sorted alphabetically - see
// store.Portfolio.AllTags. Used to populate a "pick an existing tag"
// list in the tag-editing UI, so the person isn't forced to retype (and
// risks mis-spelling into a silently different) a tag they've already
// used elsewhere.
func ComputeAllTags(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	out, err := json.Marshal(p.AllTags())
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAllocationByTag takes a JSON-serialized store.Portfolio and
// returns a JSON array of finance.AllocationSlice, one slice per
// distinct tag currently in use (plus "Untagged") - see
// finance.AllocationByTag's doc comment. memberID scopes to one member's
// holdings; empty means the whole family.
func ComputeAllocationByTag(portfolioJSON string, memberID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.FilterHoldingsByMember(finance.ComputeHoldings(&p), memberID)
	slices := finance.AllocationByTag(holdings)
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

// SetAssetETMoneyURL records (or clears, if url is "") the ETMoney fund
// page URL for an asset, so FetchCapCompositionFromETMoney can later be
// pointed at it without re-typing the URL every time. Returns the
// updated portfolio as JSON.
func SetAssetETMoneyURL(portfolioJSON string, assetID string, url string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetAssetETMoneyURL(assetID, url)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// FetchCapCompositionFromETMoney fetches and parses Large/Mid/Small/Cash
// percentages from an ETMoney fund page URL (real HTTP call - requires
// the Android app to hold the INTERNET permission). Does NOT save
// anything - the caller reviews the result and calls SetCapComposition
// separately, same two-step pattern as RefreshAmfiPrices/SavePortfolio.
// See priceapi.FetchETMoneyCapComposition's doc comment: this path is
// unverified against the live site and may need adjustment after a real
// on-device test.
func FetchCapCompositionFromETMoney(url string) string {
	result, err := priceapi.FetchETMoneyCapComposition(url)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	out, err := json.Marshal(result)
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
		upsertPriceRecord(p, asset.ID, isoDate, rec.NAV, "AMFI")
		matchedCount++
	}
	return matchedCount
}

// upsertPriceRecord replaces any existing PriceRecord for the same
// AssetID+Date (so refreshing twice on the same day doesn't accumulate
// duplicate rows), or appends a new one otherwise.
// upsertPriceRecord is a single-record convenience wrapper around
// store.Portfolio.UpsertPrices - delegating rather than duplicating the
// upsert logic here means this also gets that method's cache
// invalidation (see Portfolio.invalidatePriceIndex's doc comment) for
// free, rather than needing its own copy of that bookkeeping.
func upsertPriceRecord(p *store.Portfolio, assetID, isoDate string, price float64, source string) {
	p.UpsertPrices([]store.PriceRecord{
		{AssetID: assetID, Date: isoDate, Price: price, Source: source},
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

// RefreshSymbolPrices fetches a live quote (real HTTP call - requires
// the Android app to hold the INTERNET permission) for every Asset that
// has a Symbol but no ISIN - an ETF or stock, not a folio-based mutual
// fund - and updates today's PriceRecord for each.
//
// Complements RefreshAmfiPrices, which only ever covered ISIN-based
// mutual fund assets: together they're "refresh today's price for
// everything", the lightweight quick-refresh counterpart to
// UpdateHistoricalPrice's much heavier multi-year history fetch. Any
// individual symbol's fetch failing (e.g. a bad/incomplete symbol - see
// FixAssetSymbolActivity) does not stop the others from being tried;
// failures are collected and returned so the caller can show exactly
// which ones need attention, rather than one bad symbol silently
// blocking every fund's refresh.
func RefreshSymbolPrices(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}

	matched := 0
	var failures []string
	for _, asset := range p.Assets {
		if asset.ISIN != "" || asset.Symbol == "" {
			continue
		}
		quote, err := priceapi.FetchYahooQuote(asset.Symbol)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s (%s): %s", asset.Name, asset.Symbol, err.Error()))
			continue
		}
		isoDate := time.Now().Format("2006-01-02")
		if !quote.AsOf.IsZero() {
			isoDate = quote.AsOf.Format("2006-01-02")
		}
		upsertPriceRecord(&p, asset.ID, isoDate, quote.Price, "YAHOO")
		matched++
	}

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	failuresJSON, err := json.Marshal(failures)
	if err != nil {
		failuresJSON = []byte("[]")
	}
	return fmt.Sprintf(`{"matchedCount":%d,"failures":%s,"portfolio":%s}`, matched, failuresJSON, string(out))
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
// is deliberately not used here for that reason. Rows with no usable
// ISIN (common for broker trade-CSV exports, unlike CAS statements,
// which always carry one) fall back to matching by exact scheme/fund
// name within the account instead - see findAssetByNameInAccount's doc
// comment for what this does and doesn't guarantee.
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
			asset, ok = findAssetByNameInAccount(&p, txn.Scheme, account.ID)
		}
		if !ok {
			asset = store.Asset{
				ID:        store.NewID("asset"),
				AccountID: account.ID,
				Name:      txn.Scheme,
				ISIN:      txn.ISIN,
				Type:      inferAssetType(txn),
				Symbol:    inferInitialSymbol(txn),
			}
			p.Assets = append(p.Assets, asset)
		} else if asset.ISIN == "" && txn.ISIN != "" {
			// Backfill: this asset was first created from an ISIN-less
			// CSV row (matched by name just now), and this row happens
			// to carry a real ISIN - store it so FUTURE imports for this
			// same fund can match by the more reliable ISIN path instead
			// of continuing to depend on the name staying identical.
			for i := range p.Assets {
				if p.Assets[i].ID == asset.ID {
					p.Assets[i].ISIN = txn.ISIN
					asset.ISIN = txn.ISIN
					break
				}
			}
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
			Balance:     txn.Balance,
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
// RemoveDuplicateTransactions scans the portfolio's own stored
// transactions for exact duplicates (same asset, date, type, amount,
// and units where both have it - see transactionsMatch) and removes all
// but the first of each duplicate group. This is a cleanup tool for
// duplicates that already made it into a portfolio - from before the
// commit-time dedup existed, from an older app build that predated it,
// or from any other path that bypassed CommitStagedRows - not a
// replacement for that dedup, which prevents new duplicates at import
// time. Which exact copy is kept within a duplicate group is arbitrary
// (whichever appears first in Transactions) since the copies are
// financially identical either way. Returns
// {"removed": N, "groups": [...], "portfolio": {...}} - see
// DuplicateGroup's doc comment for why the groups are surfaced rather
// than just a bare count.
// DuplicateGroup describes one set of matching transactions found by
// RemoveDuplicateTransactions - shown to the person before they confirm
// removal. Confidence reflects which tier of transactionsMatch decided
// the group: "reference" when both sides carried a matching genuine
// per-transaction reference (as certain as the data allows), "balance"
// when both sides had a running unit balance that also matched (CAS
// statements print one on nearly every row - strong evidence, though
// not absolutely as certain as a reference), or "heuristic" when
// neither was available on both sides and the match rests only on
// date/amount/units coinciding - see transactionsMatch's doc comment
// for the full reasoning. Showing this lets a person trust "reference"
// and "balance" groups readily and give "heuristic" groups a closer
// look before confirming.
type DuplicateGroup struct {
	AssetID     string  `json:"assetId"`
	AssetName   string  `json:"assetName"`
	Date        string  `json:"date"`
	Amount      float64 `json:"amount"`
	ExtraCopies int     `json:"extraCopies"` // copies beyond the first that would be/were removed
	Confidence  string  `json:"confidence"`  // "reference" or "heuristic"
}

func RemoveDuplicateTransactions(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	assetNameByID := make(map[string]string, len(p.Assets))
	for _, a := range p.Assets {
		assetNameByID[a.ID] = a.Name
	}

	keep := make([]store.StoredTransaction, 0, len(p.Transactions))
	var groups []DuplicateGroup
	groupIndex := make(map[string]int) // AssetID+Date -> index into groups, only for txns that had at least one duplicate found
	removed := 0
	for _, txn := range p.Transactions {
		matchedKeptIndex := -1
		for i, kept := range keep {
			if transactionsMatch(kept, txn) {
				matchedKeptIndex = i
				break
			}
		}
		if matchedKeptIndex == -1 {
			keep = append(keep, txn)
			continue
		}
		removed++
		key := txn.AssetID + "|" + txn.Date
		if gi, ok := groupIndex[key]; ok {
			groups[gi].ExtraCopies++
		} else {
			confidence := "heuristic"
			refKept := extractTransactionReference(keep[matchedKeptIndex].Description)
			refTxn := extractTransactionReference(txn.Description)
			switch {
			case refKept != "" && refTxn != "" && refKept == refTxn:
				confidence = "reference"
			case keep[matchedKeptIndex].Balance != nil && txn.Balance != nil:
				confidence = "balance"
			}
			groupIndex[key] = len(groups)
			groups = append(groups, DuplicateGroup{
				AssetID: txn.AssetID, AssetName: assetNameByID[txn.AssetID],
				Date: txn.Date, Amount: keep[matchedKeptIndex].Amount, ExtraCopies: 1,
				Confidence: confidence,
			})
		}
	}
	p.Transactions = keep

	out, err := json.Marshal(struct {
		Removed   int              `json:"removed"`
		Groups    []DuplicateGroup `json:"groups"`
		Portfolio json.RawMessage  `json:"portfolio"`
	}{Removed: removed, Groups: groups, Portfolio: mustMarshal(p)})
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// transactionReferencePattern and bareReferencePattern extract a
// genuine per-transaction reference embedded in a Description, when the
// source data actually carries one:
//   - MFCentral CAS statements print "...Trxn.Ref.No.pay_XXXXXXXXX//..."
//     for netbanking-initiated Purchase rows (a Razorpay-style payment
//     ID) - transactionReferencePattern - though NOT for auto-debited
//     SIP installments ("Sys. Investment ISIP (n/28)"), which have no
//     such reference at all.
//   - A small number of CAS rows carry a bare "pay_XXXXXXXXX" token
//     without the "Trxn.Ref.No." prefix (a formatting quirk seen in
//     real statements) - bareReferencePattern catches those too.
//   - CSV imports (see csvimport.go) embed the broker's own trade ID
//     (e.g. Zerodha's trade_id) as "[ref:XXXXXXXXX]" in Description.
//
// Verified against a real MFCentral CAS PDF's actual description text,
// not just the parser's structural regexes.
var (
	transactionReferencePattern = regexp.MustCompile(`Trxn\.Ref\.No\.(\S+?)//`)
	bareReferencePattern        = regexp.MustCompile(`\bpay_[A-Za-z0-9]+\b`)
	csvReferencePattern         = regexp.MustCompile(`\[ref:([^\]]+)\]`)
)

func extractTransactionReference(description string) string {
	if m := transactionReferencePattern.FindStringSubmatch(description); m != nil {
		return m[1]
	}
	if m := bareReferencePattern.FindString(description); m != "" {
		return m
	}
	if m := csvReferencePattern.FindStringSubmatch(description); m != nil {
		return m[1]
	}
	return ""
}

// transactionsMatch is the shared "are these the same transaction"
// rule, used both to keep new imports from duplicating existing rows
// (isDuplicateTransaction) and to find duplicates that already exist
// (RemoveDuplicateTransactions), so the two never disagree about what
// counts as a duplicate.
//
// Three tiers, most authoritative first:
//  1. Reference (see extractTransactionReference): when BOTH sides
//     carry a genuine per-transaction reference, it's decisive either
//     way - same reference means the same real-world transaction, no
//     matter what amount/units say; different references means
//     genuinely different transactions, even if amount/units coincide.
//  2. Running unit balance (CAS statements print one on every row,
//     unlike the reference, which only appears on netbanking Purchase
//     rows - SIP installments never have one): when both sides have a
//     Balance and it differs, they're never the same transaction,
//     regardless of amount/units - a real distinct purchase always
//     leaves the running total at a different point. This is what
//     catches the case a reference alone can't: a manual Purchase
//     landing on the same day, for the same amount, as a scheduled SIP
//     installment (a real case, confirmed in this portfolio's own CAS
//     statement, not hypothetical) - the SIP side has no reference, but
//     both sides do have a balance, and it's different.
//  3. Date + amount + units (the original heuristic): the fallback when
//     neither of the above is available on both sides. This is the one
//     genuine remaining ambiguity - mutual fund NAV is fixed per day,
//     so two separate real purchases of the same amount on the same day
//     produce identical units too, and if the source data carries
//     neither a reference nor a balance for either row, there's no more
//     signal left to tell them apart.
func transactionsMatch(a, b store.StoredTransaction) bool {
	const amountEpsilon = 0.01
	const unitsEpsilon = 0.0005
	const balanceEpsilon = 0.001
	if a.AssetID != b.AssetID || a.Date != b.Date || a.Type != b.Type {
		return false
	}

	refA := extractTransactionReference(a.Description)
	refB := extractTransactionReference(b.Description)
	if refA != "" && refB != "" {
		return refA == refB
	}

	if math.Abs(a.Amount-b.Amount) > amountEpsilon {
		return false
	}
	if a.Units != nil && b.Units != nil && math.Abs(*a.Units-*b.Units) > unitsEpsilon {
		return false
	}
	if a.Balance != nil && b.Balance != nil && math.Abs(*a.Balance-*b.Balance) > balanceEpsilon {
		return false
	}
	return true
}

func isDuplicateTransaction(existing []store.StoredTransaction, assetID string, txn store.Transaction) bool {
	candidate := store.StoredTransaction{
		AssetID: assetID, Date: txn.Date, Type: txn.Type, Amount: txn.Amount,
		Units: txn.Units, Balance: txn.Balance, Description: txn.Description,
	}
	for _, e := range existing {
		if transactionsMatch(e, candidate) {
			return true
		}
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

// inferAssetType picks a starting Type for a newly-committed asset,
// used regardless of whether the row came from a CAS PDF or a CSV.
//
// Folio is the signal: every real CAS-statement transaction carries a
// mutual fund folio number (it's intrinsic to how folio-based MF units
// work), while broker trade-CSV exports (Zerodha Console included) -
// covering both ETFs and individual stocks, both traded via a plain
// exchange Symbol rather than a folio - essentially never do. Blindly
// defaulting every new asset to "MutualFund" (the old behavior) was
// wrong for exactly this case: an ISIN-carrying, folio-less CSV row for
// an NSE/BSE-listed ETF (e.g. NIFTYBEES) got mislabeled as a mutual
// fund, which matters beyond cosmetics - see UpdateHistoryActivity's
// asset-classification comment: an asset with an ISIN is routed to the
// AMFI/TigZig NAV-history path, NOT the Yahoo symbol-based path an
// ETF/stock actually needs, regardless of what its Type says. Fixing
// Type here doesn't fix that routing by itself (routing keys off
// ISIN-presence, not Type) - inferInitialSymbol below is the other half
// of the real fix, since populating Symbol is what a person can use
// to correct the routing after import (see FixAssetSymbolActivity).
//
// This is a heuristic, not a certified classification - a mutual fund
// bought through a broker's CSV export with no folio column at all
// would also default to "Stock" here, incorrectly. There's no
// stronger signal available in a generic broker CSV to distinguish
// that case.
func inferAssetType(txn store.Transaction) string {
	if txn.Folio != "" {
		return "MutualFund"
	}
	return "Stock"
}

// inferInitialSymbol pre-fills Asset.Symbol from the CSV/CAS row's own
// scheme/security text for a folio-less (likely ETF/stock) row, as a
// STARTING point only - a raw broker CSV "symbol" column (e.g.
// "NIFTYBEES") is not by itself a valid Yahoo Finance ticker, which
// needs an exchange suffix (".NS" for NSE, ".BO" for BSE) that this
// function does not add, since the same underlying instrument can
// genuinely trade - and appear in the same CSV - under both exchanges,
// and guessing which one the person wants would be presenting a guess
// as fact. The person still needs to review/complete this via
// FixAssetSymbolActivity before "Update Price History" can use it.
func inferInitialSymbol(txn store.Transaction) string {
	if txn.Folio != "" {
		return "" // folio-based (mutual fund) row - no symbol concept applies
	}
	return strings.TrimSpace(txn.Scheme)
}

// findAssetByNameInAccount is CommitStagedRows' fallback for rows with
// no usable ISIN - most broker trade-CSV exports (Zerodha Console
// included) simply don't carry an ISIN column at all, unlike a CAS
// statement, which always does. Without this fallback, every CSV-only
// import (or overlapping re-import of the same CSV) fell straight to
// "create a new asset" every single time, since
// findAssetByISINInAccount("") always returns not-found by design -
// this was silently duplicating assets (and therefore every transaction
// attached to them) on every re-import, even though the PDF path's
// reliance on ISIN worked correctly the whole time.
//
// Matching is exact (case-insensitive, whitespace-trimmed) on the
// scheme/fund name text as it appears in that row - reliable when
// re-importing overlapping exports from the SAME broker/source (the
// name text is identical every time), which is the common case this was
// reported against. It is NOT guaranteed to bridge across genuinely
// different naming conventions (e.g. a CSV's short ticker-style name vs
// a CAS statement's full official AMFI scheme name for the same real
// fund) - if those differ, this still creates a separate asset, same as
// before. That's a real, known limitation, not silently papered over.
func findAssetByNameInAccount(p *store.Portfolio, scheme string, accountID string) (store.Asset, bool) {
	normalized := strings.ToLower(strings.TrimSpace(scheme))
	if normalized == "" {
		return store.Asset{}, false
	}
	for _, a := range p.Assets {
		if a.AccountID == accountID && strings.ToLower(strings.TrimSpace(a.Name)) == normalized {
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

// UpdateHistoricalPrice fetches full price history for a symbol-based
// asset (an ETF or stock - anything identified by a Yahoo Finance
// ticker rather than an AMFI ISIN, e.g. a manually-entered NIFTYBEES.NS
// or a foreign brokerage holding) and merges it into the portfolio's
// cached Prices, same "Update History" manual-action pattern as
// UpdateHistoricalNav. See priceapi.FetchYahooAdjClose's doc comment
// for the one honesty caveat on this data source. Returns the updated
// portfolio JSON, or a bridge error string if the fetch/parse fails.
func UpdateHistoricalPrice(portfolioJSON string, assetID string, symbol string, since string) string {
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
	if symbol == "" {
		return `{"error":"symbol cannot be empty"}`
	}

	points, err := priceapi.FetchYahooAdjClose(symbol, since)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	records := make([]store.PriceRecord, 0, len(points))
	for _, pt := range points {
		records = append(records, store.PriceRecord{
			AssetID: assetID, Date: pt.Date, Price: pt.Price, Source: "YAHOO_HISTORY",
		})
	}
	p.UpsertPrices(records)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddBenchmark adds a new tracked market index - see store.Benchmark's
// doc comment. Returns the updated portfolio JSON.
func AddBenchmark(portfolioJSON string, name string, yahooTicker string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if name == "" {
		return `{"error":"name cannot be empty"}`
	}
	if yahooTicker == "" {
		return `{"error":"yahooTicker cannot be empty"}`
	}
	p.AddBenchmark(name, yahooTicker)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// RemoveBenchmark removes a tracked index by ID - see
// store.Portfolio.RemoveBenchmark. Returns the updated portfolio JSON.
func RemoveBenchmark(portfolioJSON string, benchmarkID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.RemoveBenchmark(benchmarkID)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// UpdateBenchmarkHistory fetches full historical daily levels for one
// tracked index via Yahoo Finance (see priceapi.FetchYahooAdjClose's doc
// comment for the one honesty caveat on this data source, which applies
// here too) and merges them into the portfolio's cached Prices, keyed by
// the benchmark's own ID - same "Update History" manual-action pattern
// as UpdateHistoricalPrice, and see store.Benchmark's doc comment for
// why this reuses the exact same Prices storage as fund NAV history.
// Returns the updated portfolio JSON, or a bridge error string if the
// fetch/parse fails.
func UpdateBenchmarkHistory(portfolioJSON string, benchmarkID string, since string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	var benchmark *store.Benchmark
	for i := range p.Benchmarks {
		if p.Benchmarks[i].ID == benchmarkID {
			benchmark = &p.Benchmarks[i]
			break
		}
	}
	if benchmark == nil {
		return `{"error":"no benchmark with that ID exists"}`
	}

	points, err := priceapi.FetchYahooAdjClose(benchmark.YahooTicker, since)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	records := make([]store.PriceRecord, 0, len(points))
	for _, pt := range points {
		records = append(records, store.PriceRecord{
			AssetID: benchmarkID, Date: pt.Date, Price: pt.Price, Source: "YAHOO_HISTORY",
		})
	}
	p.UpsertPrices(records)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ReturnsTableRow is one row of the Returns screen's table - one fund
// (an actual portfolio holding) or one benchmark index, with its
// trailing Day/Month figures and full rolling-return distributions for
// 1/3/5/10 years - see finance.ComputeTrailingReturn and
// finance.ComputeRollingReturnStats' doc comments for exactly what each
// figure means.
type ReturnsTableRow struct {
	SeriesID    string // an Asset.ID or a Benchmark.ID - pass back to ComputePriceHistory for the tap-to-graph drill-down
	Name        string
	IsBenchmark bool
	Day         finance.TrailingReturn
	Month       finance.TrailingReturn
	OneYear     finance.RollingReturnStats
	ThreeYear   finance.RollingReturnStats
	FiveYear    finance.RollingReturnStats
	TenYear     finance.RollingReturnStats
}

// ComputeReturnsTable builds one ReturnsTableRow per currently-held fund
// (Type == "MutualFund", the only type with real long-run NAV history
// via AMFI/TigZig today - see UpdateHistoricalNav) plus one row per
// tracked Benchmark, for the Returns screen's table. today is
// "yyyy-MM-dd". Returns a JSON array of ReturnsTableRow, or a bridge
// error string.
func ComputeReturnsTable(portfolioJSON string, today string) string {
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

	var rows []ReturnsTableRow
	for _, a := range p.Assets {
		if a.Type != "MutualFund" {
			continue
		}
		series := p.PriceSeries(a.ID)
		if len(series) == 0 {
			continue // never had a price fetched - nothing to show
		}
		rows = append(rows, buildReturnsRow(a.ID, a.Name, false, series, t))
	}
	for _, b := range p.Benchmarks {
		series := p.PriceSeries(b.ID)
		if len(series) == 0 {
			continue
		}
		rows = append(rows, buildReturnsRow(b.ID, b.Name, true, series, t))
	}

	out, err := json.Marshal(rows)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

func buildReturnsRow(seriesID, name string, isBenchmark bool, series []store.PriceRecord, today time.Time) ReturnsTableRow {
	return ReturnsTableRow{
		SeriesID:    seriesID,
		Name:        name,
		IsBenchmark: isBenchmark,
		Day:         finance.ComputeTrailingReturn(series, 1, "Day", today),
		Month:       finance.ComputeTrailingReturn(series, 30, "1 Month", today),
		OneYear:     finance.ComputeRollingReturnStats(series, 1, "1 Year"),
		ThreeYear:   finance.ComputeRollingReturnStats(series, 3, "3 Year"),
		FiveYear:    finance.ComputeRollingReturnStats(series, 5, "5 Year"),
		TenYear:     finance.ComputeRollingReturnStats(series, 10, "10 Year"),
	}
}

// ComputePriceHistory returns the raw historical price series for one
// fund or benchmark (identified by ReturnsTableRow.SeriesID) as a JSON
// array of {Date, Price} objects - for the Returns screen's tap-to-graph
// drill-down. Returns an empty array (not an error) if the series has no
// data yet.
func ComputePriceHistory(portfolioJSON string, seriesID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	series := p.PriceSeries(seriesID)
	out, err := json.Marshal(series)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

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
func ComputeProgression(portfolioJSON string, memberID string, axis string, today string, cachePath string) string {
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
	cache := loadProgressionCacheIfRequested(cachePath)
	points := finance.ComputeProgression(&p, memberID, finance.ProgressionAxis(axis), t, cache)
	saveProgressionCacheIfRequested(cache, cachePath)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// loadProgressionCacheIfRequested/saveProgressionCacheIfRequested wrap
// finance.LoadProgressionCache/(*ProgressionCache).Save for the 3
// weekly progression bridge functions - cachePath == "" means "caching
// disabled", passed straight through as a nil *finance.ProgressionCache
// (see finance.ComputeProgression's doc comment: nil just means always
// compute fresh, no error). A save failure is deliberately swallowed -
// this is a pure performance cache, not user data, and a failed write
// here should never surface as a user-facing error or block anything
// else the caller is doing.
func loadProgressionCacheIfRequested(cachePath string) *finance.ProgressionCache {
	if cachePath == "" {
		return nil
	}
	return finance.LoadProgressionCache(cachePath)
}

func saveProgressionCacheIfRequested(cache *finance.ProgressionCache, cachePath string) {
	if cache == nil || cachePath == "" {
		return
	}
	_ = cache.Save(cachePath)
}

// ComputeProgressionDailyRange is ComputeProgression's daily-granularity
// counterpart, bounded to [startDate, endDate] - see
// finance.ComputeProgressionDailyRange's doc comment. Intended for a
// zoomed-in chart window (see ProgressionChartView's onWindowChanged),
// not full-history daily browsing.
func ComputeProgressionDailyRange(portfolioJSON string, memberID string, axis string, startDate string, endDate string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid start date: "+err.Error())
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid end date: "+err.Error())
	}
	points := finance.ComputeProgressionDailyRange(&p, memberID, finance.ProgressionAxis(axis), start, end)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAssetProgression is ComputeProgression's single-fund
// counterpart - see finance.ComputeAssetProgression's doc comment.
func ComputeAssetProgression(portfolioJSON string, assetID string, today string, cachePath string) string {
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
	cache := loadProgressionCacheIfRequested(cachePath)
	points := finance.ComputeAssetProgression(&p, assetID, t, cache)
	saveProgressionCacheIfRequested(cache, cachePath)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeAssetProgressionDailyRange is ComputeAssetProgression's
// daily-granularity counterpart, bounded to [startDate, endDate] - see
// finance.ComputeAssetProgressionDailyRange's doc comment.
func ComputeAssetProgressionDailyRange(portfolioJSON string, assetID string, startDate string, endDate string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid start date: "+err.Error())
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid end date: "+err.Error())
	}
	points := finance.ComputeAssetProgressionDailyRange(&p, assetID, start, end)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeGroupProgression is ComputeProgression's fund-group counterpart
// - see finance.ComputeGroupProgression's doc comment. groupLabel is the
// free-text label assigned via SetAssetGroupLabel (e.g. "Nifty 50").
func ComputeGroupProgression(portfolioJSON string, memberID string, groupLabel string, today string, cachePath string) string {
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
	cache := loadProgressionCacheIfRequested(cachePath)
	points := finance.ComputeGroupProgression(&p, memberID, groupLabel, t, cache)
	saveProgressionCacheIfRequested(cache, cachePath)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeGroupProgressionDailyRange is ComputeGroupProgression's
// daily-granularity counterpart, bounded to [startDate, endDate] - see
// finance.ComputeGroupProgressionDailyRange's doc comment.
func ComputeGroupProgressionDailyRange(portfolioJSON string, memberID string, groupLabel string, startDate string, endDate string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid start date: "+err.Error())
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid end date: "+err.Error())
	}
	points := finance.ComputeGroupProgressionDailyRange(&p, memberID, groupLabel, start, end)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeTagProgression is ComputeProgression's tag counterpart - see
// finance.ComputeTagProgression's doc comment. tag is one value assigned
// via SetAssetTags (e.g. "Mid Cap").
func ComputeTagProgression(portfolioJSON string, memberID string, tag string, today string, cachePath string) string {
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
	cache := loadProgressionCacheIfRequested(cachePath)
	points := finance.ComputeTagProgression(&p, memberID, tag, t, cache)
	saveProgressionCacheIfRequested(cache, cachePath)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeTagProgressionDailyRange is ComputeTagProgression's
// daily-granularity counterpart, bounded to [startDate, endDate] - see
// finance.ComputeTagProgressionDailyRange's doc comment.
func ComputeTagProgressionDailyRange(portfolioJSON string, memberID string, tag string, startDate string, endDate string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid start date: "+err.Error())
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid end date: "+err.Error())
	}
	points := finance.ComputeTagProgressionDailyRange(&p, memberID, tag, start, end)
	out, err := json.Marshal(points)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputePeriodGains returns a JSON array of finance.PeriodGain - rolling
// Day/Week/Month/Year gains for the whole portfolio, net of
// contributions during each window - see finance.ComputePeriodGains' doc
// comment for exactly what that means. memberID scopes to one member;
// empty means the whole family. today is "yyyy-MM-dd".
func ComputePeriodGains(portfolioJSON string, memberID string, today string) string {
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
	gains := finance.ComputePeriodGains(&p, memberID, t)
	out, err := json.Marshal(gains)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeCalendarYearGain returns a JSON-encoded finance.PeriodGain
// (single object, not an array) for the year-to-date window - January
// 1st of today's year through today - net of contributions, same
// methodology as ComputePeriodGains - see
// finance.ComputeCalendarYearGain's doc comment. memberID scopes to one
// member; empty means the whole family. today is "yyyy-MM-dd".
func ComputeCalendarYearGain(portfolioJSON string, memberID string, today string) string {
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
	gain := finance.ComputeCalendarYearGain(&p, memberID, t)
	out, err := json.Marshal(gain)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}
