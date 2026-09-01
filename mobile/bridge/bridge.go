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
	"sort"
	"strings"
	"sync"
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
// NameListEntry is one row of the Manage Names screen - one asset OR
// benchmark's identity, its real/default Name, and its current Nickname
// (empty if none set). Deliberately includes EVERY asset/benchmark, not
// just ones with price history (unlike ComputeReturnsTable) - renaming
// should be possible before a fund has ever been priced.
type NameListEntry struct {
	SeriesID    string
	Name        string // the real/default name - NEVER the nickname, so the edit screen always shows what it's overriding
	Nickname    string
	IsBenchmark bool
	// UsableAsBenchmark mirrors Asset.UsableAsBenchmark - always false
	// for a benchmark entry itself (see that field's own doc comment;
	// the toggle only makes sense on a fund).
	UsableAsBenchmark bool
	// ISIN - needed so the Manage Names screen can grey out/explain the
	// toggle for a fund with no ISIN (AddBenchmarkFromAsset requires
	// one). Empty for a benchmark entry.
	ISIN string
}

// ComputeNameList powers the Manage Names screen - see NameListEntry's
// doc comment.
func ComputeNameList(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	entries := make([]NameListEntry, 0, len(p.Assets)+len(p.Benchmarks))
	for _, a := range p.Assets {
		entries = append(entries, NameListEntry{SeriesID: a.ID, Name: a.Name, Nickname: a.Nickname, IsBenchmark: false, UsableAsBenchmark: a.UsableAsBenchmark, ISIN: a.ISIN})
	}
	for _, b := range p.Benchmarks {
		entries = append(entries, NameListEntry{SeriesID: b.ID, Name: b.Name, Nickname: b.Nickname, IsBenchmark: true})
	}
	out, err := json.Marshal(entries)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetNickname sets or clears (given "") the personal display name for
// an asset or benchmark - see store.Portfolio.SetNickname's doc
// comment. Returns the updated portfolio as JSON, same convention as
// every other Set/Add/Remove bridge function.
func SetNickname(portfolioJSON string, seriesID string, nickname string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetNickname(seriesID, strings.TrimSpace(nickname))
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetUsableAsBenchmark toggles Asset.UsableAsBenchmark - see that
// field's own doc comment.
func SetUsableAsBenchmark(portfolioJSON string, assetID string, usable bool) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	found := false
	for i, a := range p.Assets {
		if a.ID == assetID {
			p.Assets[i].UsableAsBenchmark = usable
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf(`{"error":%q}`, "no asset found with ID "+assetID)
	}
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddBenchmarkFromAsset - see store.Portfolio.AddBenchmarkFromAsset's
// own doc comment for why this exists (a benchmark with immediate,
// zero-network-call history, reusing an already-tracked fund's own
// data).
func AddBenchmarkFromAsset(portfolioJSON string, assetID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if _, err := p.AddBenchmarkFromAsset(assetID); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetPreferredBenchmark sets or clears Asset.PreferredBenchmarkID -
// see that field's own doc comment. Pass an empty benchmarkID to
// clear the override and go back to auto-select.
func SetPreferredBenchmark(portfolioJSON string, assetID string, benchmarkID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	found := false
	for i, a := range p.Assets {
		if a.ID == assetID {
			p.Assets[i].PreferredBenchmarkID = benchmarkID
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf(`{"error":%q}`, "no asset found with ID "+assetID)
	}
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}


// finance.Holding, carrying only the 3 fields MainActivity's Dashboard
// actually uses (HasPrice/NetInvested/CurrentValue - it also needs a
// plain count and an empty check, both of which are just len() on the
// slice, no extra field needed for those). Deliberately NOT reusing
// finance.Holding directly - finance.Holding carries a nested `Tags
// []string` field, and gomobile bind cannot generate a working binding
// for an EXPORTED struct that itself contains a slice-of-struct field
// (DashboardResult.Holdings) whose element type has its OWN nested
// slice field (Holding.Tags) - this was a real, confirmed CI failure
// ("Build bridge.aar from the Go code" failed) the one time this tried
// to expose finance.Holding directly, unlike store.PriceRecord (used
// successfully elsewhere as a slice field, e.g. MultiSeriesHistoryItem.
// Points), which has zero nested slice fields of its own. Keeping this
// struct minimal sidesteps the problem rather than fighting gomobile's
// binder, and also shrinks the JSON payload for a screen that never
// needed the other 15 fields (Tags, GroupLabel, ISIN, XIRR, etc.) anyway.
type DashboardHolding struct {
	HasPrice     bool
	NetInvested  float64
	CurrentValue float64
}

// DashboardResult bundles every figure MainActivity's Dashboard screen
// needs into ONE JSON payload from ONE portfolio-JSON unmarshal, instead
// of the 7 separate bridge calls it used to make (computeHoldingsForMember,
// computePortfolioXIRR, computePeriodGains, computeCalendarYearGain,
// computeAllocationByMarketCap, computeAllocationByEquityOrigin,
// computeAllocationByPortfolioClass) - each of which independently
// re-parsed the SAME portfolio JSON on the Go side. PortfolioLoadCache
// (Kotlin side) already avoids re-reading/re-parsing the JSON STRING on
// every screen visit, but it can't avoid this - each bridge call is a
// fresh JNI crossing into Go, and json.Unmarshal has to run again inside
// each one regardless of what Kotlin already cached. This was the
// confirmed remaining cause of "especially going to the dashboard" lag.
// Field names deliberately match each existing bridge function's own
// JSON output shape (e.g. "xirr"/"hasXIRR" matching ComputePortfolioXIRR's
// map output) so the Kotlin-side data classes barely change.
type DashboardResult struct {
	Holdings         []DashboardHolding
	XIRR             float64 `json:"xirr"`
	HasXIRR          bool    `json:"hasXIRR"`
	RollingGains     []finance.PeriodGain
	CalendarYearGain finance.PeriodGain
	MarketCapSlices  []finance.AllocationSlice
	OriginSlices     []finance.AllocationSlice
	ClassSlices      []finance.AllocationSlice
}

// ComputeDashboard is the combined replacement for the 7 calls described
// in DashboardResult's doc comment - see that comment for why this
// exists. today is "yyyy-MM-dd", same convention as ComputePeriodGains/
// ComputeCalendarYearGain.
func ComputeDashboard(portfolioJSON string, memberID string, today string) string {
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

	allHoldings := finance.ComputeHoldings(&p)
	memberHoldings := finance.FilterHoldingsByMember(allHoldings, memberID)
	xirr, hasXIRR := finance.PortfolioXIRR(&p, memberHoldings)

	classByAsset := make(map[string]string, len(p.Assets))
	for _, a := range p.Assets {
		classByAsset[a.ID] = a.AssetClass
	}
	capCompByAsset := make(map[string]store.CapComposition)
	originCompByAsset := make(map[string]store.EquityOriginComposition)
	for _, a := range p.Assets {
		if c, ok := p.GetCapComposition(a.ID); ok {
			capCompByAsset[a.ID] = c
		}
		if c, ok := p.GetEquityOriginComposition(a.ID); ok {
			originCompByAsset[a.ID] = c
		}
	}

	result := DashboardResult{
		Holdings:         toDashboardHoldings(memberHoldings),
		XIRR:             xirr,
		HasXIRR:          hasXIRR,
		RollingGains:     finance.ComputePeriodGains(&p, memberID, t),
		CalendarYearGain: finance.ComputeCalendarYearGain(&p, memberID, t),
		// Market cap allocation is member-scoped (matches
		// ComputeAllocationByMarketCap's own memberHoldings use);
		// origin/class allocation are whole-family always (matches
		// ComputeAllocationByEquityOrigin/ByPortfolioClass, which never
		// took a memberID param in the first place).
		MarketCapSlices: finance.AllocationByMarketCapSegment(memberHoldings, capCompByAsset),
		OriginSlices:    finance.AllocationByEquityOrigin(allHoldings, classByAsset, originCompByAsset),
		ClassSlices:     finance.AllocationByPortfolioClass(allHoldings, classByAsset),
	}

	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

func toDashboardHoldings(holdings []finance.Holding) []DashboardHolding {
	out := make([]DashboardHolding, len(holdings))
	for i, h := range holdings {
		out[i] = DashboardHolding{HasPrice: h.HasPrice, NetInvested: h.NetInvested, CurrentValue: h.CurrentValue}
	}
	return out
}

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

// ComputeFamilyPooledHoldings is the "All (family)" view's own holdings
// list - see finance.PoolHoldingsByISIN's own doc comment for what
// pooling means here (the SAME fund held by different members shows as
// ONE row with combined totals, confirmed as the correct family-wide
// behavior). Always operates on the WHOLE portfolio (every member,
// unfiltered) - there's no memberID parameter, since pooling across
// members is specifically what this is for; use ComputeGroupedHoldings
// or the plain per-holding path instead for a single member's own view.
func ComputeFamilyPooledHoldings(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	holdings := finance.ComputeHoldings(&p)
	pooled := finance.PoolHoldingsByISIN(&p, holdings)
	out, err := json.Marshal(pooled)
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

// SetAssetClassOverride records (or clears, given "") the manual
// Equity/Debt/Commodity/Others correction for one asset - see
// store.Asset.AssetClassOverride's doc comment. Returns the updated
// portfolio as JSON.
func SetAssetClassOverride(portfolioJSON string, assetID string, class string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetAssetClassOverride(assetID, class)
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
		// Deliberately does NOT fall back to time.Now() when AsOf is
		// unavailable (an earlier version of this did) - a confirmed
		// real bug: time.Now() reads the DEVICE'S OWN CLOCK, which
		// isn't necessarily correct or in a timezone that lines up with
		// when the market actually traded. If a device's date is even
		// one day off (misconfigured, or a timezone edge case), that
		// wrong date gets stored as a real price record and becomes the
		// "most recent" date across the WHOLE portfolio - dragging
		// ComputePeriodGains' Day anchor (see its own doc comment) into
		// a day no real fund has actually been priced for yet, which
		// silently reports Day as a flat ₹0 everywhere (this was
		// reported live: "Comparing 2026-08-26 → 2026-08-27" when the
		// real date was still 2026-08-26 - the fallback had stamped a
		// price with tomorrow's device-local date). Skipping this
		// asset's refresh (surfaced as a clear failure the person can
		// see) is far better than silently storing a guessed, possibly
		// wrong, date that then corrupts a portfolio-wide calculation.
		if quote.AsOf.IsZero() {
			failures = append(failures, fmt.Sprintf("%s (%s): quote had no trade timestamp - skipped rather than guessing today's date", asset.Name, asset.Symbol))
			continue
		}
		isoDate := quote.AsOf.Format("2006-01-02")
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
// CommitStagedRows adds every "NEW"-status staged row to the portfolio,
// attaching them to a "CAS Import" account under the given EXISTING
// member. memberID must match a real Member already in the portfolio -
// this deliberately does NOT create a new member on a mismatch (unlike
// an earlier version of this function, which matched by free-text NAME
// and silently created a brand-new member whenever that name didn't
// exactly match an existing one). That was a real risk: a typo in a
// hand-typed member name (e.g. "Mom" vs "Mother") would silently spawn
// a phantom duplicate member that doesn't actually exist, rather than
// surfacing an error - see ImportActivity's member picker (a dropdown
// of real members, no free-text entry) for the other half of this fix.
// Adding an actual new family member is Manage Members' job, not an
// incidental side effect of importing a statement.
func CommitStagedRows(portfolioJSON string, stagedRowsJSON string, memberID string) string {
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

	if memberID == "" {
		return `{"error":"no member selected - pick who this statement belongs to"}`
	}
	memberFound := false
	for _, m := range p.Members {
		if m.ID == memberID {
			memberFound = true
			break
		}
	}
	if !memberFound {
		return `{"error":"no member with that ID exists - add them via Manage Members first"}`
	}
	const accountName = "CAS Import"

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
			// A fund that was being TRACKED (an "Additional Fund", not
			// yet owned - see store.Asset.AccountID's own doc comment)
			// and is now genuinely being bought for the first time gets
			// PROMOTED (its existing Asset ID gains a real Account)
			// rather than getting a second, brand-new Asset row - see
			// store.Portfolio.PromoteTrackedFund's own doc comment for
			// why that matters (its already-fetched NAV history carries
			// straight over, nothing re-fetched or merged).
			if tracked, found := p.FindTrackedFundByISIN(txn.ISIN); found {
				if p.PromoteTrackedFund(tracked.ID, account.ID) {
					asset = tracked
					asset.AccountID = account.ID
					ok = true
				}
			}
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
		assetNameByID[a.ID] = a.DisplayName()
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

// UpdateHistoricalNav fetches a mutual fund asset's NAV history from
// TigZig (by ISIN) and merges it into the portfolio's cached Prices,
// upserting rather than duplicating any date already present. This is
// the manual "Update History" action for the Indian side of the
// progression feature - not called automatically, same pattern as the
// existing AMFI current-price refresh.
//
// Incremental by default: if this asset already has cached price
// history, only the gap from the day after its latest cached date
// onward is actually fetched over the network (see
// priceapi.FetchTigzigNavHistory's doc comment - TigZig's own live
// OpenAPI spec confirms a `since` bound is genuinely supported, not
// previously used here). A brand-new asset with no cached history yet
// still fetches everything available, same as before. This was the
// confirmed cause of "Update History taking 30-60 seconds" - every
// fund's ENTIRE history was being re-downloaded on every tap, even for
// funds updated minutes earlier.
//
// Returns the updated portfolio JSON, or a bridge error string if the
// asset doesn't exist or the fetch/parse fails.
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

	since := dayAfterLatest(p.PriceSeries(assetID))
	history, err := priceapi.FetchTigzigNavHistory(isin, since)
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

	// Self-heal a stale "Name == ISIN" entry - a confirmed real
	// leftover from the sync.Once scheme-list cache permanently
	// poisoning on a single transient network failure (fixed in
	// fetchMfapiSchemeList): a fund added by ISIN back when that bug
	// was live had its name-resolve silently fail and fall back to
	// storing the bare ISIN as Name. Retried here, best-effort, on
	// every successful Refresh - now that resolution actually retries
	// instead of staying permanently broken, this quietly repairs the
	// name without requiring a separate "fix name" action anywhere.
	// Never fails the NAV update above if this doesn't succeed.
	for i, a := range p.Assets {
		if a.ID == assetID && strings.EqualFold(a.Name, a.ISIN) {
			if resolvedName, err := priceapi.ResolveMfapiSchemeName(a.ISIN); err == nil && resolvedName != "" {
				p.Assets[i].Name = resolvedName
			}
			break
		}
	}

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// dayAfterLatest returns the day after the latest date in series,
// formatted "yyyy-MM-dd", or "" if series is empty (meaning "fetch
// everything available" to every caller below - the shared incremental-
// fetch convention this file uses for NAV/ETF-stock/FX/benchmark
// history alike). A date that fails to parse is treated the same as
// "not found" - fails safe to a full re-fetch rather than silently
// skipping a legitimately stale gap.
func dayAfterLatest(series []store.PriceRecord) string {
	if len(series) == 0 {
		return ""
	}
	latest := series[0].Date
	for _, r := range series[1:] {
		if r.Date > latest {
			latest = r.Date
		}
	}
	t, err := time.Parse("2006-01-02", latest)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// dayAfterLatestFX is dayAfterLatest's counterpart for FX rates, scoped
// to one currency (FXRates holds every currency's history together, so
// this can't just reuse dayAfterLatest over the whole slice).
func dayAfterLatestFX(rates []store.FXRate, currency string) string {
	found := false
	var latest string
	for _, r := range rates {
		if r.Currency != currency {
			continue
		}
		if !found || r.Date > latest {
			latest = r.Date
			found = true
		}
	}
	if !found {
		return ""
	}
	t, err := time.Parse("2006-01-02", latest)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, 1).Format("2006-01-02")
}

// UpdateHistoricalPrice fetches price history for a symbol-based asset
// (an ETF or stock - anything identified by a Yahoo Finance ticker
// rather than an AMFI ISIN, e.g. a manually-entered NIFTYBEES.NS or a
// foreign brokerage holding) and merges it into the portfolio's cached
// Prices, same "Update History" manual-action pattern as
// UpdateHistoricalNav - including the same incremental behavior: since
// is only used as the LOWER BOUND for an asset with no cached history
// yet; once any history exists, the day after its latest cached date
// wins instead, so a re-run only fetches the actual gap. See
// priceapi.FetchYahooAdjClose's doc comment for the one honesty caveat
// on this data source. Returns the updated portfolio JSON, or a bridge
// error string if the fetch/parse fails.
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

	hasExistingHistory := len(p.PriceSeries(assetID)) > 0
	if incremental := dayAfterLatest(p.PriceSeries(assetID)); incremental != "" {
		since = incremental
	}
	points, err := priceapi.FetchYahooAdjClose(symbol, since)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	// Zero results is only worth surfacing as an error for a first-ever
	// fetch (likely a bad/delisted ticker) - see FetchYahooAdjClose's
	// doc comment on why the function itself no longer makes this call.
	// For an asset with existing history, zero new points just means
	// "already up to date", the normal and common case once incremental
	// fetching is working.
	if len(points) == 0 && !hasExistingHistory {
		return fmt.Sprintf(`{"error":%q}`, "no price history found for "+symbol)
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

// AddTRIBenchmark is AddBenchmark's Total-Return counterpart - see
// store.Benchmark.NiftyTRIIndexName's doc comment for what makes this
// a genuinely different data source, not just a labeling difference.
// niftyTRIIndexName must be one of NSE Indices' own canonical
// spellings (see priceapi.FetchNiftyIndicesTRI's doc comment) - an
// unrecognized spelling won't error here (this call doesn't fetch
// anything), only later when history is actually requested.
func AddTRIBenchmark(portfolioJSON string, name string, niftyTRIIndexName string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if name == "" {
		return `{"error":"name cannot be empty"}`
	}
	if niftyTRIIndexName == "" {
		return `{"error":"niftyTRIIndexName cannot be empty"}`
	}
	p.AddTRIBenchmark(name, niftyTRIIndexName)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddProxyFundBenchmark is AddTRIBenchmark's index-fund-proxy
// counterpart - see store.Benchmark.ProxyFundISIN's own doc comment.
// Resolves the fund's real name from mfapi.in (same as
// ResolveFundNameByISIN) so it's never stored as a bare ISIN.
func AddProxyFundBenchmark(portfolioJSON string, name string, isin string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if name == "" {
		return `{"error":"name cannot be empty"}`
	}
	isin = strings.TrimSpace(strings.ToUpper(isin))
	if isin == "" {
		return `{"error":"isin cannot be empty"}`
	}
	proxyFundName, err := priceapi.ResolveMfapiSchemeName(isin)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	p.AddProxyFundBenchmark(name, isin, proxyFundName)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// AddTrackedFund adds an "Additional Fund" - a fund tracked purely for
// comparison, not actually owned - see store.Asset.AccountID's own doc
// comment. Rejects a duplicate ISIN already being tracked (an ISIN
// already OWNED is fine - see store.Portfolio.AddTrackedFund's own doc
// comment for why that case isn't blocked). Returns the updated
// portfolio JSON, same convention as every other Add* bridge function.
func AddTrackedFund(portfolioJSON string, name string, isin string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	name = strings.TrimSpace(name)
	isin = strings.TrimSpace(strings.ToUpper(isin))
	if name == "" {
		return `{"error":"name cannot be empty"}`
	}
	if isin == "" {
		return `{"error":"isin cannot be empty"}`
	}
	if _, found := p.FindTrackedFundByISIN(isin); found {
		return `{"error":"this fund is already being tracked"}`
	}
	p.AddTrackedFund(name, isin)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// RemoveTrackedFund removes an Additional Fund by ID - see
// store.Portfolio.RemoveTrackedFund's own doc comment for why this
// refuses (returns an error, changes nothing) if the ID actually
// belongs to a real, owned holding rather than a tracked-only entry.
func RemoveTrackedFund(portfolioJSON string, assetID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	if !p.RemoveTrackedFund(assetID) {
		return `{"error":"no tracked (not-owned) fund with that ID exists - an owned holding can't be removed this way"}`
	}
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SearchMfapiSchemes looks up funds by name against mfapi.in's full
// scheme list (the same cached list priceapi.ResolveMfapiSchemeCode
// already uses for the NAV fallback - see its own doc comment for
// provenance) - the search step behind "add an Additional Fund by
// name" rather than requiring a person to already know its ISIN.
func SearchMfapiSchemes(query string) string {
	query = strings.TrimSpace(query)
	if len(query) < 3 {
		return `{"error":"query must be at least 3 characters"}`
	}
	matches, err := priceapi.SearchMfapiSchemes(query, 25)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	out, err := json.Marshal(matches)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ResolveFundNameByISIN looks up a fund's real name from mfapi.in by
// ISIN (see priceapi.ResolveMfapiSchemeName's own doc comment) - used
// when a person adds an Additional Fund by pasting a bare ISIN rather
// than picking one from a name search, so the fund can be shown by its
// actual name instead of the ISIN string itself - a confirmed real
// complaint with the ISIN-only add path. Returns {"name":"..."} on
// success.
func ResolveFundNameByISIN(isin string) string {
	name, err := priceapi.ResolveMfapiSchemeName(strings.TrimSpace(strings.ToUpper(isin)))
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return fmt.Sprintf(`{"name":%q}`, name)
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

// UpdateBenchmarkHistory fetches historical daily levels for one
// tracked index via Yahoo Finance (see priceapi.FetchYahooAdjClose's doc
// comment for the one honesty caveat on this data source, which applies
// here too) and merges them into the portfolio's cached Prices, keyed by
// the benchmark's own ID - same "Update History" manual-action pattern
// as UpdateHistoricalPrice, and see store.Benchmark's doc comment for
// why this reuses the exact same Prices storage as fund NAV history.
// Same incremental behavior too: since is only the lower bound for a
// benchmark with no cached history yet - every caller today
// (BenchmarksActivity's per-index Refresh, and the centralized Update
// History screen) always passes a fixed early date, which used to mean
// a full history re-fetch on literally every tap regardless of how
// current the cached data already was. Returns the updated portfolio
// JSON, or a bridge error string if the fetch/parse fails.
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

	hasExistingHistory := len(p.PriceSeries(benchmarkID)) > 0
	if incremental := dayAfterLatest(p.PriceSeries(benchmarkID)); incremental != "" {
		since = incremental
	}

	// A TRI benchmark fetches via NSE Indices' own TRI endpoint instead
	// of Yahoo - see Benchmark.NiftyTRIIndexName's doc comment for why
	// these are two entirely separate fetch paths rather than one
	// trying to serve both series.
	// Checked in this order deliberately: a proxy fund, once chosen, is
	// the person's own reliable pick (see Benchmark.ProxyFundISIN's doc
	// comment) so it takes priority; NiftyTRIIndexName's scrape is next;
	// YahooTicker (plain price index, no TRI) is the last resort.
	var points []priceapi.YahooPricePoint
	var source string
	if benchmark.ProxyFundISIN != "" {
		navPoints, err := priceapi.FetchMfapiNavHistory(benchmark.ProxyFundISIN, since)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error())
		}
		for _, pt := range navPoints {
			points = append(points, priceapi.YahooPricePoint{Date: pt.Date, Price: pt.Nav})
		}
		source = "INDEX_FUND_PROXY_NAV"
	} else if benchmark.NiftyTRIIndexName != "" {
		triPoints, err := priceapi.FetchNiftyIndicesTRI(benchmark.NiftyTRIIndexName, since)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error())
		}
		for _, pt := range triPoints {
			points = append(points, priceapi.YahooPricePoint{Date: pt.Date, Price: pt.Nav})
		}
		source = "NIFTY_TRI_HISTORY"
	} else {
		var err error
		points, err = priceapi.FetchYahooAdjClose(benchmark.YahooTicker, since)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, err.Error())
		}
		source = "YAHOO_HISTORY"
	}
	if len(points) == 0 && !hasExistingHistory {
		label := benchmark.YahooTicker
		if benchmark.NiftyTRIIndexName != "" {
			label = benchmark.NiftyTRIIndexName
		}
		if benchmark.ProxyFundISIN != "" {
			label = benchmark.ProxyFundISIN
		}
		return fmt.Sprintf(`{"error":%q}`, "no price history found for "+label)
	}
	records := make([]store.PriceRecord, 0, len(points))
	for _, pt := range points {
		records = append(records, store.PriceRecord{
			AssetID: benchmarkID, Date: pt.Date, Price: pt.Price, Source: source,
		})
	}
	p.UpsertPrices(records)

	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// UpdateAllHistoryResult bundles the outcome of UpdateAllHistory across
// all 4 fetch types (NAV, ETF/stock price, FX, benchmark/index), plus
// the updated portfolio JSON - one combined result instead of the
// caller making 4 separate round trips through the individual
// Update*History functions above.
type UpdateAllHistoryResult struct {
	NavSucceeded          int
	NavTotal              int
	NavFailures           []string
	NavUsedFallback       []string // labels of funds whose NAV history came from the fallback source (mfapi.in), not TigZig
	PriceSucceeded        int
	PriceTotal            int
	PriceFailures         []string
	PriceUsedFallback     []string // labels of assets whose price history came from the fallback source, not the primary
	FxSucceeded           int
	FxTotal               int
	FxFailures            []string
	FxUsedFallback        []string // currencies whose rate history came from the fallback source
	BenchmarkSucceeded    int
	BenchmarkTotal        int
	BenchmarkFailures     []string
	BenchmarkUsedFallback []string // labels of benchmarks whose history came from the fallback source
	// RiskFreeRateUpdated/RiskFreeRateError report the outcome of a
	// best-effort risk-free-rate refresh (see RefreshRiskFreeRate's own
	// doc comment) bundled into this same NAV-refresh action, per an
	// explicit request that it happen "on a regular basis... when I
	// refresh to get NAV updates" rather than needing its own separate
	// trigger. Deliberately does NOT fail the whole UpdateAllHistory
	// call if this one piece fails - NAV/price/FX/benchmark history is
	// the primary purpose of this function, and a factsheet PDF being
	// briefly unreachable shouldn't block everything else that
	// succeeded.
	RiskFreeRateUpdated bool
	RiskFreeRateError   string
	PortfolioJSON       string
}

// historyOutcome is one concurrent fetch's result, collected before any
// of them touch the portfolio - see UpdateAllHistory's doc comment for
// why. Never part of the bridge's exported API surface (no function
// takes or returns it), so it isn't a gomobile-binding concern the way
// UpdateAllHistoryResult above is.
type historyOutcome struct {
	kind         string // "nav", "price", "fx", "benchmark"
	label        string
	priceRecs    []store.PriceRecord
	fxRecs       []store.FXRate
	err          error
	usedFallback bool // true if the primary source failed and a second, independent source supplied this data instead - see UpdateAllHistory's doc comment on redundancy
}

// UpdateAllHistory is the single-call replacement for looping over
// UpdateHistoricalNav/UpdateHistoricalPrice/UpdateHistoricalFX/
// UpdateBenchmarkHistory one at a time (the previous approach, still
// used individually by BenchmarksActivity's own per-index Refresh).
// Each fetch is now genuinely incremental (see those functions' own doc
// comments), so an already-current portfolio's fetches are individually
// fast - but a real portfolio has many funds/indices, and doing 20+ of
// them SEQUENTIALLY still costs 20+ round-trip latencies even when each
// one returns almost nothing, which is exactly what was reported as
// "still 12-13 seconds even when nothing changed". This runs every
// fetch CONCURRENTLY instead (well within TigZig's documented 300
// requests/minute limit for any realistic personal-portfolio fund
// count), which turns that cost from "sum of every round trip" into
// "the single slowest round trip".
//
// Concurrency safety: every goroutine below only READS the portfolio
// (via PriceSeries/FXRates, to compute its own incremental "since") and
// does its own independent network fetch - nothing touches p.Prices/
// p.FXRates (the only fields any Upsert* call mutates) until AFTER
// wg.Wait(), applied single-threaded in the loop below. This matters
// because Portfolio.PriceSeries lazily builds and caches an internal
// index on its FIRST call with no locking (see its own doc comment) -
// calling it concurrently from many goroutines before that index exists
// would be a genuine data race (concurrent writes to the same map).
// The p.PriceSeries("") call up front forces that one-time build
// safely, single-threaded, before any goroutine starts; every
// concurrent PriceSeries call after that is a plain, safe map read.
func UpdateAllHistory(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.PriceSeries("") // force the one-time index build before any concurrent reads - see doc comment above

	const fxFallbackSince = "2015-01-01"
	const benchmarkFallbackSince = "2000-01-01"

	var wg sync.WaitGroup
	var mu sync.Mutex
	var outcomes []historyOutcome
	record := func(o historyOutcome) {
		mu.Lock()
		outcomes = append(outcomes, o)
		mu.Unlock()
	}

	// Grouped by ISIN, not iterated per-Asset directly - two Assets
	// sharing the same ISIN (the same real fund held under two
	// different Accounts/members, or an owned Asset alongside its own
	// not-yet-owned "Additional Fund" tracking entry before promotion)
	// would otherwise trigger two independent, fully redundant network
	// fetches for the exact same NAV history. Fetch once per unique
	// ISIN, then apply the same result to every Asset sharing it.
	assetsByISIN := make(map[string][]store.Asset)
	for _, a := range p.Assets {
		if a.ISIN == "" {
			continue
		}
		assetsByISIN[a.ISIN] = append(assetsByISIN[a.ISIN], a)
	}
	for isin, assetsForISIN := range assetsByISIN {
		isin := isin
		assetsForISIN := assetsForISIN
		wg.Add(1)
		go func() {
			defer wg.Done()
			// "since" must cover the EARLIEST gap across every Asset
			// sharing this ISIN - if one of them already has fuller
			// history than another (e.g. one was added earlier), an
			// incremental "since" based on only one of them could skip
			// dates the other one still needs.
			since := ""
			for i, a := range assetsForISIN {
				s := dayAfterLatest(p.PriceSeries(a.ID))
				if i == 0 {
					since = s
				} else if s == "" || (since != "" && s < since) {
					since = s
				}
			}
			history, err := priceapi.FetchTigzigNavHistory(isin, since)
			label := assetsForISIN[0].DisplayName()
			o := historyOutcome{kind: "nav", label: label}
			var navPoints []priceapi.TigzigNavPoint
			if err != nil {
				// Primary (TigZig) failed - retry via mfapi.in, a
				// genuinely independent second source with no TigZig
				// involvement. See priceapi.FetchMfapiNavHistory's doc
				// comment for how this was confirmed.
				navPoints, err = priceapi.FetchMfapiNavHistory(isin, since)
				if err == nil {
					o.usedFallback = true
				}
			} else {
				navPoints = history.Data
			}
			if err != nil {
				o.err = err
			} else {
				source := "TIGZIG_HISTORY"
				if o.usedFallback {
					source = "MFAPI_FALLBACK"
				}
				for _, a := range assetsForISIN {
					for _, pt := range navPoints {
						o.priceRecs = append(o.priceRecs, store.PriceRecord{AssetID: a.ID, Date: pt.Date, Price: pt.Nav, Source: source})
					}
				}
			}
			record(o)
		}()
	}

	for _, a := range p.Assets {
		if a.ISIN != "" || a.Symbol == "" {
			continue
		}
		a := a
		wg.Add(1)
		go func() {
			defer wg.Done()
			since := fxFallbackSince
			if incremental := dayAfterLatest(p.PriceSeries(a.ID)); incremental != "" {
				since = incremental
			}
			o := historyOutcome{kind: "price", label: fmt.Sprintf("%s (%s)", a.DisplayName(), a.Symbol)}
			points, err := priceapi.FetchYahooAdjClose(a.Symbol, since)
			if err != nil {
				// Primary (TigZig's Yahoo proxy) failed - retry via a
				// genuinely independent second source (Yahoo's own
				// public chart endpoint directly, no TigZig involved)
				// before giving up on this asset entirely. See
				// priceapi.FetchYahooAdjCloseDirect's doc comment.
				points, err = priceapi.FetchYahooAdjCloseDirect(a.Symbol, since)
				if err == nil {
					o.usedFallback = true
				}
			}
			if err != nil {
				o.err = err
			} else {
				for _, pt := range points {
					source := "YAHOO_HISTORY"
					if o.usedFallback {
						source = "YAHOO_DIRECT_FALLBACK"
					}
					o.priceRecs = append(o.priceRecs, store.PriceRecord{AssetID: a.ID, Date: pt.Date, Price: pt.Price, Source: source})
				}
			}
			record(o)
		}()
	}

	currencies := map[string]bool{}
	for _, acc := range p.Accounts {
		if acc.Currency != "" && acc.Currency != "INR" {
			currencies[acc.Currency] = true
		}
	}
	for currency := range currencies {
		currency := currency
		wg.Add(1)
		go func() {
			defer wg.Done()
			since := fxFallbackSince
			if incremental := dayAfterLatestFX(p.FXRates, currency); incremental != "" {
				since = incremental
			}
			o := historyOutcome{kind: "fx", label: currency}
			rates, err := priceapi.FetchFrankfurterHistory(currency, since)
			if err != nil {
				// Primary (Frankfurter/ECB) failed - retry via a
				// genuinely independent second source (the
				// fawazahmed0/currency-api project, served statically
				// via jsDelivr, no ECB/Frankfurter involvement at all).
				// See priceapi.FetchCurrencyApiFallbackHistory's doc
				// comment, including its capped backfill window.
				rates, err = priceapi.FetchCurrencyApiFallbackHistory(currency, since)
				if err == nil {
					o.usedFallback = true
				}
			}
			if err != nil {
				o.err = err
			} else {
				o.fxRecs = rates
			}
			record(o)
		}()
	}

	for _, b := range p.Benchmarks {
		b := b
		wg.Add(1)
		go func() {
			defer wg.Done()
			since := benchmarkFallbackSince
			if incremental := dayAfterLatest(p.PriceSeries(b.ID)); incremental != "" {
				since = incremental
			}
			o := historyOutcome{kind: "benchmark", label: b.DisplayName()}

			// A proxy-fund benchmark fetches via the fund's own NAV
			// history (mfapi.in, same proven path Additional Funds
			// uses) - see Benchmark.ProxyFundISIN's own doc comment for
			// why this is checked first, ahead of the TRI scrape.
			if b.ProxyFundISIN != "" {
				navPoints, err := priceapi.FetchMfapiNavHistory(b.ProxyFundISIN, since)
				if err != nil {
					o.err = err
				} else {
					for _, pt := range navPoints {
						o.priceRecs = append(o.priceRecs, store.PriceRecord{AssetID: b.ID, Date: pt.Date, Price: pt.Nav, Source: "INDEX_FUND_PROXY_NAV"})
					}
				}
				record(o)
				return
			}

			// A TRI benchmark fetches via NSE Indices' own TRI endpoint
			// instead of Yahoo, with NO Yahoo-direct fallback attempted
			// on error - see Benchmark.NiftyTRIIndexName's doc comment:
			// there is no equivalent "independent second source" for
			// genuine TRI data the way there is for a plain price
			// index, so a TRI fetch failure is just reported as a
			// failure, not silently retried against a different series
			// that wouldn't actually be TRI data.
			if b.NiftyTRIIndexName != "" {
				triPoints, err := priceapi.FetchNiftyIndicesTRI(b.NiftyTRIIndexName, since)
				if err != nil {
					o.err = err
				} else {
					for _, pt := range triPoints {
						o.priceRecs = append(o.priceRecs, store.PriceRecord{AssetID: b.ID, Date: pt.Date, Price: pt.Nav, Source: "NIFTY_TRI_HISTORY"})
					}
				}
				record(o)
				return
			}

			points, err := priceapi.FetchYahooAdjClose(b.YahooTicker, since)
			if err != nil {
				// Same independent fallback as the ETF/stock price loop
				// above - see priceapi.FetchYahooAdjCloseDirect's doc
				// comment.
				points, err = priceapi.FetchYahooAdjCloseDirect(b.YahooTicker, since)
				if err == nil {
					o.usedFallback = true
				}
			}
			if err != nil {
				o.err = err
			} else {
				for _, pt := range points {
					source := "YAHOO_HISTORY"
					if o.usedFallback {
						source = "YAHOO_DIRECT_FALLBACK"
					}
					o.priceRecs = append(o.priceRecs, store.PriceRecord{AssetID: b.ID, Date: pt.Date, Price: pt.Price, Source: source})
				}
			}
			record(o)
		}()
	}

	wg.Wait()

	// Single-threaded from here on - every p.Upsert* call below is safe
	// precisely because nothing else touches p concurrently anymore.
	result := UpdateAllHistoryResult{}
	for _, o := range outcomes {
		switch o.kind {
		case "nav":
			result.NavTotal++
			if o.err != nil {
				result.NavFailures = append(result.NavFailures, o.label+": "+o.err.Error())
			} else {
				p.UpsertPrices(o.priceRecs)
				result.NavSucceeded++
				if o.usedFallback {
					result.NavUsedFallback = append(result.NavUsedFallback, o.label)
				}
			}
		case "price":
			result.PriceTotal++
			if o.err != nil {
				result.PriceFailures = append(result.PriceFailures, o.label+": "+o.err.Error())
			} else {
				p.UpsertPrices(o.priceRecs)
				result.PriceSucceeded++
				if o.usedFallback {
					result.PriceUsedFallback = append(result.PriceUsedFallback, o.label)
				}
			}
		case "fx":
			result.FxTotal++
			if o.err != nil {
				result.FxFailures = append(result.FxFailures, o.label+": "+o.err.Error())
			} else {
				p.UpsertFXRates(o.fxRecs)
				result.FxSucceeded++
				if o.usedFallback {
					result.FxUsedFallback = append(result.FxUsedFallback, o.label)
				}
			}
		case "benchmark":
			result.BenchmarkTotal++
			if o.err != nil {
				result.BenchmarkFailures = append(result.BenchmarkFailures, o.label+": "+o.err.Error())
			} else {
				p.UpsertPrices(o.priceRecs)
				result.BenchmarkSucceeded++
				if o.usedFallback {
					result.BenchmarkUsedFallback = append(result.BenchmarkUsedFallback, o.label)
				}
			}
		}
	}

	// Best-effort risk-free-rate refresh, bundled into this same NAV-
	// refresh action - see UpdateAllHistoryResult.RiskFreeRateUpdated's
	// own doc comment for why this doesn't fail the whole call on
	// error. Skipped entirely (not even attempted) while a manual
	// override is active - SetFetchedRiskFreeRate already no-ops in
	// that case, but checking here too avoids the pointless network
	// fetch when its result would just be discarded anyway.
	if !p.RiskFreeRateManual {
		if rfResult, errs := priceapi.FetchConsensusRiskFreeRate(); len(errs) == 0 || rfResult.RatePercent != 0 {
			if p.SetFetchedRiskFreeRate(rfResult.RatePercent, rfResult.AsOfDate, rfResult.Source) {
				result.RiskFreeRateUpdated = true
			}
		} else {
			msgs := make([]string, 0, len(errs))
			for _, e := range errs {
				msgs = append(msgs, e.Error())
			}
			result.RiskFreeRateError = strings.Join(msgs, "; ")
		}
	}

	portfolioOut, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	result.PortfolioJSON = string(portfolioOut)

	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// (an actual portfolio holding) or one benchmark index. Day and Month
// are trailing-only (see finance.TrailingReturn's doc comment on why a
// rolling distribution of sub-year windows wouldn't mean much); each of
// 1/3/5/10 Year carries BOTH a trailing figure (finance.
// ComputeTrailingReturnForYears - "what actually happened over the most
// recent N years") and a rolling distribution (finance.
// ComputeRollingReturnStats - "what a random N-year window has
// historically looked like") side by side, so the two different
// questions those numbers answer are both visible at once.
type ReturnsTableRow struct {
	SeriesID    string // an Asset.ID or a Benchmark.ID - pass back to ComputePriceHistory for the tap-to-graph drill-down
	Name        string
	IsBenchmark bool
	// IsAdditional marks an "Additional Fund" - a fund tracked for
	// comparison but not actually owned (see store.Asset.AccountID's
	// own doc comment). Always false for a benchmark row. The Returns
	// screen uses this to split fund rows into two sections (owned vs
	// tracked-only) rather than one flat list.
	IsAdditional      bool
	Day               finance.TrailingReturn
	Month             finance.TrailingReturn
	OneYearTrailing   finance.TrailingReturn
	OneYearRolling    finance.RollingReturnStats
	ThreeYearTrailing finance.TrailingReturn
	ThreeYearRolling  finance.RollingReturnStats
	FiveYearTrailing  finance.TrailingReturn
	FiveYearRolling   finance.RollingReturnStats
	TenYearTrailing   finance.TrailingReturn
	TenYearRolling    finance.RollingReturnStats
}

// ComputeReturnsTable builds one ReturnsTableRow per currently-held fund
// (Type == "MutualFund", the only type with real long-run NAV history
// via AMFI/TigZig today - see UpdateHistoricalNav) plus one row per
// tracked Benchmark, for the Returns screen's table. Every figure is
// anchored to each series' own latest actual data point, never literal
// calendar "today" - see finance.ComputeTrailingReturn's doc comment -
// so this no longer needs a `today` parameter at all. Returns a JSON
// array of ReturnsTableRow, or a bridge error string.
func ComputeReturnsTable(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
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
		row := buildReturnsRow(a.ID, a.DisplayName(), false, series)
		row.IsAdditional = !a.IsOwned()
		rows = append(rows, row)
	}
	for _, b := range p.Benchmarks {
		series := p.PriceSeries(b.ID)
		if len(series) == 0 {
			continue
		}
		rows = append(rows, buildReturnsRow(b.ID, b.DisplayName(), true, series))
	}

	out, err := json.Marshal(rows)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

func buildReturnsRow(seriesID, name string, isBenchmark bool, series []store.PriceRecord) ReturnsTableRow {
	return ReturnsTableRow{
		SeriesID:          seriesID,
		Name:              name,
		IsBenchmark:       isBenchmark,
		Day:               finance.ComputeTrailingReturn(series, 1, "Day"),
		Month:             finance.ComputeTrailingReturn(series, 30, "1 Month"),
		OneYearTrailing:   finance.ComputeTrailingReturnForYears(series, 1, "1 Year"),
		OneYearRolling:    finance.ComputeRollingReturnStats(series, 1, "1 Year"),
		ThreeYearTrailing: finance.ComputeTrailingReturnForYears(series, 3, "3 Year"),
		ThreeYearRolling:  finance.ComputeRollingReturnStats(series, 3, "3 Year"),
		FiveYearTrailing:  finance.ComputeTrailingReturnForYears(series, 5, "5 Year"),
		FiveYearRolling:   finance.ComputeRollingReturnStats(series, 5, "5 Year"),
		TenYearTrailing:   finance.ComputeTrailingReturnForYears(series, 10, "10 Year"),
		TenYearRolling:    finance.ComputeRollingReturnStats(series, 10, "10 Year"),
	}
}

// CustomPeriodReturnResult bundles the trailing figure and rolling
// distribution for a person-typed custom tenure (see
// ComputeCustomPeriodReturn) - the same trailing+rolling pairing every
// fixed 1/3/5/10-Year tenure already gets in ReturnsTableRow, just for
// an arbitrary years value instead of one of those four.
type CustomPeriodReturnResult struct {
	Trailing finance.TrailingReturn
	Rolling  finance.RollingReturnStats
}

// ComputeCustomPeriodReturn computes the trailing return and rolling-
// return distribution for one fund/benchmark over a person-typed
// tenure in years (e.g. 2.5), rather than one of the four fixed
// tenures ReturnsTableRow always shows - the Returns detail screen's
// "type a period" option. years must be positive. Returns a bridge
// error string only for malformed portfolio JSON or an unrecognized
// seriesID; a years value too large for the series' own history comes
// back as HasData=false on both fields, same as every other tenure.
func ComputeCustomPeriodReturn(portfolioJSON string, seriesID string, years float64) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	series := p.PriceSeries(seriesID)
	if len(series) == 0 {
		return fmt.Sprintf(`{"error":%q}`, "unknown or empty seriesID: "+seriesID)
	}
	label := fmt.Sprintf("%g Year", years)
	result := CustomPeriodReturnResult{
		Trailing: finance.ComputeTrailingReturnForCustomYears(series, years, label),
		Rolling:  finance.ComputeRollingReturnStatsForCustomYears(series, years, label),
	}
	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputePriceHistory returns the raw historical price series for one
// fund or benchmark (identified by ReturnsTableRow.SeriesID) as a JSON
// array of {Date, Price} objects - for the Returns screen's tap-to-graph
// drill-down. Returns an empty array (not an error) if the series has no
// data yet.
// TransactionMarker is one buy/sell point overlaid on a fund's price
// history chart - the bridge-side source for the transaction-markers
// feature (dots on the NAV graph showing when/how much was invested,
// tap to see date/NAV/units/amount, similar to a broker app's holding
// chart). IsBuy distinguishes a positive-unit event (PURCHASE,
// PURCHASE_SIP, SWITCH_IN, SWITCH_IN_MERGER, DIVIDEND_REINVEST) from a
// negative-unit one (REDEMPTION, SWITCH_OUT, SWITCH_OUT_MERGER) so the
// Kotlin side can color them differently without re-deriving the
// classification from the raw TransactionType string itself. A
// transaction with no unit change at all (e.g. a cash DIVIDEND_PAYOUT,
// or a tax/fee line) has nothing meaningful to plot on a NAV-vs-time
// chart and is excluded entirely - see buildTransactionMarkers below.
type TransactionMarker struct {
	Date        string
	IsBuy       bool
	Units       float64
	Price       float64 // NAV/price at the time of this transaction, as recorded on the statement - NOT re-derived from the price history series, since the two can legitimately differ slightly (statement price vs a later-fetched close)
	Amount      float64 // absolute value - sign is already conveyed by IsBuy, no need to double-encode it
	Description string
	// Member is who made this specific transaction - always "" for a
	// single-fund chart (ComputeAssetTransactionMarkers), only ever
	// populated by ComputeFamilyTransactionMarkers below, where a
	// single chart can now show markers from more than one family
	// member's own account. A confirmed real request: the SAME real
	// fund held by different members should show as ONE chart with
	// everyone's transactions on it, each labeled whose it is - not a
	// tap-through summary dialog.
	Member string
}

// ComputeAssetTransactionMarkers returns every plottable transaction
// for one asset, sorted ascending by date - see TransactionMarker's
// doc comment for what "plottable" excludes. seriesID matches an
// Asset.ID (this is fund/ETF-only; Benchmarks have no transactions to
// plot, so an unrecognised or benchmark ID simply yields an empty
// array, not an error - consistent with ComputePriceHistory's own
// unknown-ID handling).
func ComputeAssetTransactionMarkers(portfolioJSON string, seriesID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	markers := buildTransactionMarkers(p.Transactions, seriesID)
	out, err := json.Marshal(markers)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeFamilyTransactionMarkers is ComputeAssetTransactionMarkers'
// multi-asset counterpart - for a fund pooled across family members
// (see finance.PoolHoldingsByISIN's own doc comment for the "same
// ISIN, different accounts" pooling this serves), returns EVERY
// plottable transaction across ALL the given asset IDs on one combined,
// date-sorted list, each marker tagged with which member made it (via
// that asset's own Account -> Member chain) - so a single chart can
// show the whole family's buys/sells on the one fund they all hold,
// not just one person's. assetIDsJSON is a JSON array of Asset.ID
// strings (the pooled group's AssetIDs, exactly as
// finance.GroupedHolding.AssetIDs already provides them).
func ComputeFamilyTransactionMarkers(portfolioJSON string, assetIDsJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	var assetIDs []string
	if err := json.Unmarshal([]byte(assetIDsJSON), &assetIDs); err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid assetIDs JSON: "+err.Error())
	}

	accountMember := make(map[string]string, len(p.Accounts))
	for _, a := range p.Accounts {
		accountMember[a.ID] = a.MemberID
	}
	memberName := make(map[string]string, len(p.Members))
	for _, m := range p.Members {
		memberName[m.ID] = m.Name
	}
	assetAccount := make(map[string]string, len(p.Assets))
	for _, a := range p.Assets {
		assetAccount[a.ID] = a.AccountID
	}

	var all []TransactionMarker
	for _, assetID := range assetIDs {
		markers := buildTransactionMarkers(p.Transactions, assetID)
		mName := memberName[accountMember[assetAccount[assetID]]]
		for i := range markers {
			markers[i].Member = mName
		}
		all = append(all, markers...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Date < all[j].Date })

	out, err := json.Marshal(all)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// buildTransactionMarkers is split out from ComputeAssetTransactionMarkers
// so the filtering/classification logic is directly unit-testable
// without going through JSON marshaling.
func buildTransactionMarkers(transactions []store.StoredTransaction, seriesID string) []TransactionMarker {
	markers := make([]TransactionMarker, 0)
	for _, t := range transactions {
		if t.AssetID != seriesID {
			continue
		}
		if t.Units == nil || *t.Units == 0 {
			continue // nothing to plot - see TransactionMarker's doc comment
		}
		price := 0.0
		if t.Price != nil {
			price = *t.Price
		}
		markers = append(markers, TransactionMarker{
			Date:        t.Date,
			IsBuy:       *t.Units > 0,
			Units:       absFloat64(*t.Units),
			Price:       price,
			Amount:      absFloat64(t.Amount),
			Description: t.Description,
		})
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].Date < markers[j].Date })
	return markers
}

func absFloat64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

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

// MultiSeriesHistoryItem is one entry of ComputeMultiSeriesHistory's
// result - one requested series' identity plus its raw price points, for
// the multi-series overlay comparison chart. Points are the fund/
// benchmark's raw price levels, NOT pre-normalized to a common base -
// normalization happens client-side (see OverlayChartView on the
// Kotlin side), since which base date to normalize against changes
// dynamically as the person zooms/pans the chart. Sending raw prices
// once and letting the client re-normalize on every zoom step avoids a
// bridge round-trip per zoom gesture.
type MultiSeriesHistoryItem struct {
	SeriesID    string
	Name        string
	IsBenchmark bool
	Points      []store.PriceRecord
}

// ComputeMultiSeriesHistory is ComputePriceHistory generalized to several
// series in one call, for the multi-series overlay comparison chart -
// avoids N separate bridge round-trips (JSON parse + JNI crossing each)
// for an N-fund/index comparison. seriesIDsJSON is a JSON array of
// strings (Asset.IDs and/or Benchmark.IDs). Unknown IDs are silently
// skipped (produce a zero-point entry) rather than erroring the whole
// call - one bad/stale ID shouldn't block seeing the others. Returns a
// JSON array of MultiSeriesHistoryItem, or a bridge error string only
// for a malformed portfolio or seriesIDsJSON itself.
func ComputeMultiSeriesHistory(portfolioJSON string, seriesIDsJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	var seriesIDs []string
	if err := json.Unmarshal([]byte(seriesIDsJSON), &seriesIDs); err != nil {
		return fmt.Sprintf(`{"error":%q}`, "invalid series id list: "+err.Error())
	}

	nameAndKind := map[string]struct {
		name        string
		isBenchmark bool
	}{}
	for _, a := range p.Assets {
		nameAndKind[a.ID] = struct {
			name        string
			isBenchmark bool
		}{a.DisplayName(), false}
	}
	for _, b := range p.Benchmarks {
		nameAndKind[b.ID] = struct {
			name        string
			isBenchmark bool
		}{b.DisplayName(), true}
	}

	items := make([]MultiSeriesHistoryItem, 0, len(seriesIDs))
	for _, id := range seriesIDs {
		info := nameAndKind[id]
		items = append(items, MultiSeriesHistoryItem{
			SeriesID:    id,
			Name:        info.name,
			IsBenchmark: info.isBenchmark,
			Points:      p.PriceSeries(id),
		})
	}

	out, err := json.Marshal(items)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// FundMetricsResult is ComputeFundMetrics' JSON shape - the computed
// finance.FundMetrics plus which benchmark was actually used, so the
// UI can show "Compared against: Nifty 50 (auto)" and let the person
// override it. Empty BenchmarkID/BenchmarkName means no benchmark
// comparison was possible (no default match, e.g. a debt fund, AND
// none was explicitly requested) - Max Drawdown alone may still have
// data in that case (see finance.FundMetrics' own doc comment).
type FundMetricsResult struct {
	finance.FundMetrics
	BenchmarkID   string
	BenchmarkName string
	AutoSelected  bool // true if BenchmarkID was picked by DefaultBenchmarkTicker, not chosen by the person
	// RiskFreeRatePercent/AsOf/Source/Manual mirror the SAME fields on
	// store.Portfolio (see RiskFreeRatePercent's own doc comment) -
	// included here so the Kotlin side can show WHICH rate was actually
	// used for this specific Sharpe/Sortino/Alpha, and where it came
	// from, rather than presenting the figures with no visible
	// assumption behind them.
	RiskFreeRatePercent float64
	RiskFreeRateAsOf    string
	RiskFreeRateSource  string
	RiskFreeRateManual  bool
}

// RefreshRiskFreeRate fetches the current risk-free rate directly from
// a real AMC's own monthly factsheet - see priceapi.FetchConsensusRiskFreeRate's
// own doc comment for the source/redundancy/fallback details. No-op
// (does NOT overwrite) if the person has a manual override active -
// see store.Portfolio.RiskFreeRateManual's own doc comment for why.
// Meant to be called from the same "refresh" action that already
// fetches NAV history, so the rate stays current without a separate
// dedicated action to remember.
func RefreshRiskFreeRate(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	result, errs := priceapi.FetchConsensusRiskFreeRate()
	if len(errs) > 0 && result.RatePercent == 0 {
		msgs := make([]string, 0, len(errs))
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return fmt.Sprintf(`{"error":%q}`, "could not fetch a risk-free rate from any source: "+strings.Join(msgs, "; "))
	}
	p.SetFetchedRiskFreeRate(result.RatePercent, result.AsOfDate, result.Source)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SetManualRiskFreeRate lets the person override the risk-free rate
// directly - see store.Portfolio.SetManualRiskFreeRate's own doc
// comment. ratePercent is a PERCENT (5.5 for 5.5%), matching the
// field's own convention throughout this app.
func SetManualRiskFreeRate(portfolioJSON string, ratePercent float64) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.SetManualRiskFreeRate(ratePercent)
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ClearManualRiskFreeRate reverts to letting RefreshRiskFreeRate update
// the rate again - see store.Portfolio.ClearManualRiskFreeRate's own
// doc comment.
func ClearManualRiskFreeRate(portfolioJSON string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	p.ClearManualRiskFreeRate()
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// ComputeFundMetrics computes Beta/Information Ratio/Up-Down Capture/Max
// Drawdown for one fund (seriesID must be an Asset.ID - benchmarks don't
// get compared against themselves). If benchmarkID is empty, a default
// benchmark is auto-picked via finance.DefaultBenchmarkTicker matched
// against the portfolio's own already-added Benchmarks by YahooTicker -
// if the person hasn't added that ticker yet (see BenchmarksActivity's
// quick-add chips), no default is available and only Max Drawdown will
// have data; pass an explicit benchmarkID (from the person manually
// picking one) to override. Returns a bridge error string only for
// malformed portfolio JSON or an unrecognized seriesID.
func ComputeFundMetrics(portfolioJSON string, seriesID string, benchmarkID string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}

	var fundName string
	var preferredBenchmarkID string
	found := false
	for _, a := range p.Assets {
		if a.ID == seriesID {
			fundName = a.Name
			preferredBenchmarkID = a.PreferredBenchmarkID
			found = true
			break
		}
	}
	if !found {
		return fmt.Sprintf(`{"error":%q}`, "unknown seriesID: "+seriesID)
	}
	fundSeries := p.PriceSeries(seriesID)

	result := FundMetricsResult{}
	if dd, ok := finance.ComputeMaxDrawdown(fundSeries); ok {
		result.MaxDrawdown = dd
		result.MaxDrawdownHasData = true
	}

	// The person's own stored/fetched risk-free rate, resolved once and
	// reused across every metric below that needs it (Alpha/Sharpe/
	// Sortino) - see finance.ResolveRiskFreeRate's own doc comment.
	riskFreeRate := finance.ResolveRiskFreeRate(&p)

	// Everything below (Beta/Information Ratio/Capture/Alpha/Sharpe/
	// Sortino/Standard Deviation) is windowed to the trailing 3 years -
	// see finance.WindowToTrailingYears' own doc comment for the
	// confirmed real-world convention this matches. Max Drawdown above
	// deliberately stays on the FULL fundSeries (not windowed) - no
	// equivalent 3-year convention was confirmed for it.
	windowedFundSeries := finance.WindowToTrailingYears(fundSeries, 3)

	autoSelected := false
	if benchmarkID == "" && preferredBenchmarkID != "" {
		// The person's own persisted manual choice (Manage Names or a
		// fund detail screen's benchmark picker - see
		// Asset.PreferredBenchmarkID's own doc comment) - takes
		// priority over name-based auto-select, but an explicit
		// benchmarkID param (a one-off override for THIS call only)
		// still wins over even this. Only used if that benchmark
		// still actually exists - a deleted benchmark's stale ID
		// falls through to ordinary auto-select below rather than
		// erroring out.
		for _, b := range p.Benchmarks {
			if b.ID == preferredBenchmarkID {
				benchmarkID = b.ID
				break
			}
		}
	}
	if benchmarkID == "" {
		// TRI is tried FIRST - a fund's real factsheet always
		// benchmarks against the TRI variant (see
		// store.Benchmark.NiftyTRIIndexName's doc comment), so if the
		// person has added that TRI benchmark, prefer it over the
		// plain price-index version. Falls back to the price-index
		// match only when no TRI benchmark for this segment has been
		// added yet.
		wantTRIName := finance.DefaultBenchmarkTRIName(fundName)
		if wantTRIName != "" {
			// Checked in two passes over the same benchmark list: a
			// proxy-fund benchmark (Name holds the canonical NSE index
			// name, e.g. "NIFTY 500" - see BenchmarksActivity.kt's
			// proxyFundTargets doc comment for why it's stored that
			// way) is preferred over a TRI-scrape benchmark for the
			// SAME index, matching UpdateBenchmarkHistory's own
			// priority order (a proxy fund is the person's deliberate,
			// reliable choice). Only falls to the scrape-based match if
			// no proxy for this segment has been added.
			for _, b := range p.Benchmarks {
				if b.ProxyFundISIN != "" && b.Name == wantTRIName {
					benchmarkID = b.ID
					autoSelected = true
					break
				}
			}
			if benchmarkID == "" {
				for _, b := range p.Benchmarks {
					if b.NiftyTRIIndexName == wantTRIName {
						benchmarkID = b.ID
						autoSelected = true
						break
					}
				}
			}
		}
		if benchmarkID == "" {
			wantTicker := finance.DefaultBenchmarkTicker(fundName)
			if wantTicker != "" {
				for _, b := range p.Benchmarks {
					if b.YahooTicker == wantTicker {
						benchmarkID = b.ID
						autoSelected = true
						break
					}
				}
			}
		}
	}

	if benchmarkID != "" {
		for _, b := range p.Benchmarks {
			if b.ID == benchmarkID {
				result.BenchmarkID = b.ID
				result.BenchmarkName = b.DisplayName()
				result.AutoSelected = autoSelected
				benchSeries := finance.WindowToTrailingYears(p.PriceSeries(b.ID), 3)
				if beta, ok := finance.ComputeBeta(windowedFundSeries, benchSeries); ok {
					result.Beta = beta
					result.BetaHasData = true
				}
				if ir, ok := finance.ComputeInformationRatio(windowedFundSeries, benchSeries); ok {
					result.InformationRatio = ir
					result.InfoRatioHasData = true
				}
				up, down, upOK, downOK := finance.ComputeCaptureRatios(windowedFundSeries, benchSeries)
				if upOK {
					result.UpCapture = up
					result.UpCaptureHasData = true
				}
				if downOK {
					result.DownCapture = down
					result.DownCaptureHasData = true
				}
				if alpha, ok := finance.ComputeAlpha(windowedFundSeries, benchSeries, riskFreeRate); ok {
					result.Alpha = alpha
					result.AlphaHasData = true
				}
				break
			}
		}
	}

	// Sharpe/Sortino/Standard Deviation need only the fund's own
	// series, no benchmark - see finance.ComputeSharpeRatio's doc
	// comment - so these are computed unconditionally, unlike the
	// benchmark-relative block above (which also now includes Alpha,
	// since Jensen's Alpha genuinely needs a Beta/benchmark, unlike
	// these three).
	if sharpe, ok := finance.ComputeSharpeRatio(windowedFundSeries, riskFreeRate); ok {
		result.SharpeRatio = sharpe
		result.SharpeHasData = true
	}
	if sortino, ok := finance.ComputeSortinoRatio(windowedFundSeries, riskFreeRate); ok {
		result.SortinoRatio = sortino
		result.SortinoHasData = true
	}
	if sd, ok := finance.ComputeStandardDeviation(windowedFundSeries); ok {
		result.StandardDeviation = sd
		result.StdDevHasData = true
	}
	result.RiskFreeRatePercent = p.RiskFreeRatePercent
	result.RiskFreeRateAsOf = p.RiskFreeRateAsOf
	result.RiskFreeRateSource = p.RiskFreeRateSource
	result.RiskFreeRateManual = p.RiskFreeRateManual
	if result.RiskFreeRatePercent == 0 && !p.RiskFreeRateManual {
		// Nothing fetched or set yet - report the actual fallback value
		// being used (as a percent, matching the field's own
		// convention) rather than leaving this looking like a genuine
		// 0% rate was used.
		result.RiskFreeRatePercent = finance.DefaultAnnualRiskFreeRate * 100
		result.RiskFreeRateSource = "Default (not yet fetched)"
	}

	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// from Frankfurter (ECB-sourced, from the given date to today) and
// merges them into the portfolio's cached FXRates. Manual action, same
// "Update History" pattern as UpdateHistoricalNav. Returns the updated
// portfolio JSON, or a bridge error string if the fetch/parse fails.
// UpdateHistoricalFX fetches FX rate history for one currency and
// merges it into the portfolio's cached FXRates - same "Update
// History" manual-action pattern and same incremental behavior as
// UpdateHistoricalNav/UpdateHistoricalPrice: since is only the lower
// bound for a currency with no cached rates yet; once any rates exist
// for this currency, the day after its latest cached date wins
// instead. Returns the updated portfolio JSON, or a bridge error
// string if the fetch fails.
func UpdateHistoricalFX(portfolioJSON string, currency string, since string) string {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}

	hasExistingHistory := false
	for _, r := range p.FXRates {
		if r.Currency == currency {
			hasExistingHistory = true
			break
		}
	}
	if incremental := dayAfterLatestFX(p.FXRates, currency); incremental != "" {
		since = incremental
	}
	rates, err := priceapi.FetchFrankfurterHistory(currency, since)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	if len(rates) == 0 && !hasExistingHistory {
		return fmt.Sprintf(`{"error":%q}`, "no FX rates returned for "+currency)
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
