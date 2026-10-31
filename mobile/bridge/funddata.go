package bridge

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"ledger/internal/finance"
	"ledger/internal/priceapi"
	"ledger/internal/store"
)

// fetchNavRecords is the network half of a NAV pull. Incremental by default: only days after the latest stored
// date are requested, so a repeat pull after 15 days fetches ~15 days, not the whole history. full=true
// re-requests everything, to fill gaps before the stored range (existing days are then compared, never blindly
// overwritten - see applyNav).
func fetchNavRecords(p *store.Portfolio, assetID, isin string, full bool) ([]store.PriceRecord, error) {
	since := ""
	if !full {
		since = dayAfterLatest(p.PriceSeries(assetID))
	}
	hadHistory := len(p.PriceSeries(assetID)) > 0
	var records []store.PriceRecord
	history, err := priceapi.FetchTigzigNavHistory(isin, since)
	if err == nil {
		for _, pt := range history.Data {
			records = append(records, store.PriceRecord{AssetID: assetID, Date: pt.Date, Price: pt.Nav, Source: "TIGZIG_HISTORY"})
		}
	} else {
		pts, err2 := priceapi.FetchMfapiNavHistory(isin, since)
		if err2 != nil {
			return nil, fmt.Errorf("NAV fetch failed (%v; fallback: %v)", err, err2)
		}
		for _, pt := range pts {
			records = append(records, store.PriceRecord{AssetID: assetID, Date: pt.Date, Price: pt.Nav, Source: "MFAPI_HISTORY"})
		}
	}
	if len(records) == 0 && !hadHistory {
		return nil, fmt.Errorf("no NAV history found for %s", isin)
	}
	return records, nil
}

// applyNav merges fetched prices with guard rails (see store.UpsertPricesGuarded) and records a plain-language note.
func applyNav(p *store.Portfolio, assetID string, records []store.PriceRecord) {
	st := p.UpsertPricesGuarded(records)
	note := fmt.Sprintf("%d new days", st.Added)
	if st.Updated > 0 {
		note += fmt.Sprintf(", %d small revisions applied", st.Updated)
	}
	if st.Conflicts > 0 {
		note += fmt.Sprintf(", %d days kept as stored because the new value disagreed by more than 2%%", st.Conflicts)
	}
	if st.Added == 0 && st.Updated == 0 && st.Conflicts == 0 {
		note = "already up to date"
	}
	p.EnsureFundHistory(assetID).NavNote = note + " (" + time.Now().UTC().Format("2006-01-02") + ")"
}

func pullNavGuarded(p *store.Portfolio, assetID, isin string, full bool) error {
	records, err := fetchNavRecords(p, assetID, isin, full)
	if err != nil {
		return err
	}
	applyNav(p, assetID, records)
	return nil
}

// PullFundAtPath is the one-tap "get everything for this fund" on the portfolio FILE: NAV history (incremental,
// or full when fullNav is true) merged with guard rails, plus the fund's published monthly cap/sector history.
// Network work happens first with no portfolio state held; the file is then reloaded fresh before merging and
// saving, so anything saved by another screen in the meantime is kept. Works for owned and tracked funds alike.
// Returns {"ok":true} or an error JSON.
func PullFundAtPath(path string, assetID string, fullNav bool) string {
	p0, err := store.Load(path)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	var asset store.Asset
	found := false
	for _, a := range p0.Assets {
		if a.ID == assetID {
			asset, found = a, true
			break
		}
	}
	if !found {
		return `{"error":"no asset with that ID exists"}`
	}
	if strings.TrimSpace(asset.ISIN) == "" {
		return `{"error":"this asset has no ISIN"}`
	}
	records, navErr := fetchNavRecords(p0, assetID, asset.ISIN, fullNav)
	m, mErr := priceapi.FetchFundDataManifest()
	var ff fundFetch
	if mErr == nil {
		ff = fetchFund(asset, m)
	}
	if navErr != nil && len(p0.PriceSeries(assetID)) == 0 {
		return fmt.Sprintf(`{"error":%q}`, navErr.Error())
	}
	p, err := store.Load(path)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	if navErr == nil {
		applyNav(p, assetID, records)
	} else {
		p.EnsureFundHistory(assetID).NavNote = "NAV not updated: " + navErr.Error()
	}
	if mErr == nil {
		applyFund(p, ff, m)
	} else {
		p.EnsureFundHistory(assetID).Note = "Could not reach fund-data: " + mErr.Error()
	}
	if err := store.Save(path, p); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return `{"ok":true}`
}

// fundFetch is the network half of a sync, kept separate so it can run without holding any portfolio state
// (the file-based sync reloads the portfolio fresh afterwards, so a save made meanwhile is never clobbered).
type fundFetch struct {
	assetID string
	pid     string
	months  []store.FundMonth
	note    string // non-empty = nothing to merge, just record this
	covered bool   // false = no published data for this fund (still records the manifest version)
	failed  bool   // transient failure: do not advance the stored version
}

func fetchFund(a store.Asset, m priceapi.FundDataManifest) fundFetch {
	r := fundFetch{assetID: a.ID}
	pid, err := priceapi.ResolveFundPid(a.ISIN, m.Version)
	if err != nil {
		if priceapi.IsFundNotFound(err) {
			r.note = "No published cap/sector data for this fund"
		} else {
			r.note, r.failed = "Could not resolve fund: "+err.Error(), true
		}
		return r
	}
	r.pid = pid
	f, err := priceapi.FetchFundDataFile(pid)
	if err != nil {
		if priceapi.IsFundNotFound(err) {
			r.note = "No published cap/sector data for this fund"
		} else {
			r.note, r.failed = "Could not fetch fund data: "+err.Error(), true
		}
		return r
	}
	for k, v := range f.Months {
		if len(v.S) < 8 {
			continue
		}
		r.months = append(r.months, store.FundMonth{Month: k, Split: v.S, Sectors: v.X, Estimated: v.E == 1, Source: "FUND_DATA"})
	}
	sort.Slice(r.months, func(i, j int) bool { return r.months[i].Month < r.months[j].Month })
	r.covered = true
	return r
}

func applyFund(p *store.Portfolio, r fundFetch, m priceapi.FundDataManifest) {
	h := p.EnsureFundHistory(r.assetID)
	now := time.Now().UTC().Format("2006-01-02")
	if r.failed {
		h.Note = r.note
		return
	}
	if !r.covered {
		h.Pid, h.Version, h.SyncedAt, h.Note = r.pid, m.Version, now, r.note
		return
	}
	st := p.MergeFundMonths(r.assetID, r.pid, m.Version, now, r.months)
	p.EnsureFundHistory(r.assetID).Note = fmt.Sprintf("%d months added, %d revised, %d unchanged/kept (data through %s)", st.Added, st.Updated, st.Kept, m.Latest)
}

func syncOne(p *store.Portfolio, a store.Asset, m priceapi.FundDataManifest) {
	applyFund(p, fetchFund(a, m), m)
}

// SyncFundHistoriesAtPath is the background auto-sync: it works on the portfolio FILE. It checks the published
// manifest, downloads data only for funds whose stored version is behind (network, no portfolio state held),
// then reloads the file fresh, merges, and saves - so nothing the person saved in the meantime is overwritten.
// Returns "" if nothing was needed, {"synced":N} when it saved, or an error JSON.
func SyncFundHistoriesAtPath(path string) string {
	p0, err := store.Load(path)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	m, err := priceapi.FetchFundDataManifest()
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	var todo []store.Asset
	for _, a := range p0.Assets {
		if strings.TrimSpace(a.ISIN) == "" || a.Type != "MutualFund" {
			continue
		}
		if h, ok := p0.FundHistoryFor(a.ID); ok && h.Version == m.Version {
			continue
		}
		todo = append(todo, a)
	}
	if len(todo) == 0 {
		return ""
	}
	results := make([]fundFetch, 0, len(todo))
	for _, a := range todo {
		results = append(results, fetchFund(a, m))
	}
	p, err := store.Load(path) // fresh
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	n := 0
	for _, r := range results {
		still := false
		for _, a := range p.Assets {
			if a.ID == r.assetID {
				still = true
				break
			}
		}
		if still {
			applyFund(p, r, m)
			n++
		}
	}
	if err := store.Save(path, p); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return fmt.Sprintf(`{"synced":%d}`, n)
}

func decodePortfolio(portfolioJSON string) (store.Portfolio, string) {
	var p store.Portfolio
	if portfolioJSON != "" {
		if err := json.Unmarshal([]byte(portfolioJSON), &p); err != nil {
			return p, fmt.Sprintf(`{"error":%q}`, "invalid portfolio JSON: "+err.Error())
		}
	}
	return p, ""
}

func encodePortfolio(p *store.Portfolio) string {
	out, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}

// SyncAllFundHistories refreshes cap/sector history for every mutual fund in the portfolio whose stored
// data is behind the published manifest version. Returns "" when nothing needed doing (so the caller can skip
// a save), otherwise the updated portfolio JSON. Safe to call on every app start: when the manifest version
// matches, it costs a single tiny request.
func SyncAllFundHistories(portfolioJSON string) string {
	p, e := decodePortfolio(portfolioJSON)
	if e != "" {
		return e
	}
	m, err := priceapi.FetchFundDataManifest()
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	changed := false
	for _, a := range append([]store.Asset(nil), p.Assets...) {
		if strings.TrimSpace(a.ISIN) == "" || a.Type != "MutualFund" {
			continue
		}
		if h, ok := p.FundHistoryFor(a.ID); ok && h.Version == m.Version {
			continue
		}
		syncOne(&p, a, m)
		changed = true
	}
	if !changed {
		return ""
	}
	return encodePortfolio(&p)
}

// ComputeCompositionHistory returns the month-by-month market-cap / sector mix of the member's portfolio
// (memberID "" = everyone) as JSON.
func ComputeCompositionHistory(portfolioJSON string, memberID string) string {
	p, e := decodePortfolio(portfolioJSON)
	if e != "" {
		return e
	}
	pts := finance.ComputeCompositionHistory(&p, memberID, time.Now().UTC())
	out, err := json.Marshal(pts)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(out)
}
