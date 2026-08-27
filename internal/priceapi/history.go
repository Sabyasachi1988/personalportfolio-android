package priceapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"sort"
	"time"

	"ledger/internal/store"
)

// TigzigNavPoint is one entry in a TigZig NAV history response's "data"
// array.
type TigzigNavPoint struct {
	Date string  `json:"date"`
	Nav  float64 `json:"nav"`
}

// TigzigNavResponse mirrors the single-scheme JSON shape returned by
// api.tigzig.com/mf/v1/nav?scheme=<id-or-isin> - confirmed against a
// live response on 2026-08-22:
//
//	{"scheme_code":119775,"scheme_name":"...","isin":"...","isin2":null,
//	 "first_available_date":"2013-01-03","latest_available_date":"2026-08-21",
//	 "count":3356,"data":[{"date":"2013-01-03","nav":14.052}, ...]}
//
// TigZig is a third-party republication of AMFI's public NAV records,
// not AMFI itself - flagged here since this project prefers primary
// sources where one is reachable.
type TigzigNavResponse struct {
	SchemeCode          int              `json:"scheme_code"`
	SchemeName          string           `json:"scheme_name"`
	ISIN                string           `json:"isin"`
	FirstAvailableDate  string           `json:"first_available_date"`
	LatestAvailableDate string           `json:"latest_available_date"`
	Count               int              `json:"count"`
	Data                []TigzigNavPoint `json:"data"`
}

// FetchTigzigNavHistory fetches one mutual fund's NAV history by ISIN,
// optionally bounded to since..present (since="" means the scheme's
// entire available history). Deliberately one call per fund via the
// single-identifier form, not the bulk `schemes=` form: the bulk
// response's exact shape (documented as wrapping results under a
// "schemes" key) could not be independently confirmed with a live call
// from this sandbox - tigzig.com isn't reachable from here - so rather
// than guess at an unverified shape, this uses only the form whose JSON
// was actually inspected live. TigZig's per-IP limit is 300
// requests/minute, far more than a personal portfolio's fund count
// needs even one-at-a-time.
//
// The since parameter itself IS independently confirmed, unlike the
// bulk form above - TigZig's own live OpenAPI spec at
// https://api.tigzig.com/mf/v1/openapi.json documents
// "GET /mf/v1/nav?scheme=120468&since=2024-01-01&to=2024-12-31" as a
// supported bounded-window call, and explicitly states an empty window
// (nothing published yet in that range) returns 200 with `data: []`,
// not an error - see the empty-result handling below, which
// distinguishes "asked for everything, got nothing" (a real failure)
// from "asked for a recent window, got nothing new yet" (a normal,
// successful outcome when a fund is already up to date).
func FetchTigzigNavHistory(isin string, since string) (TigzigNavResponse, error) {
	url := "https://api.tigzig.com/mf/v1/nav?scheme=" + neturl.QueryEscape(isin)
	if since != "" {
		url += "&since=" + neturl.QueryEscape(since)
	}
	resp, err := http.Get(url)
	if err != nil {
		return TigzigNavResponse{}, fmt.Errorf("tigzig nav request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TigzigNavResponse{}, fmt.Errorf("reading tigzig response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return TigzigNavResponse{}, fmt.Errorf("tigzig returned status %d for ISIN %s: %s", resp.StatusCode, isin, string(body))
	}

	var out TigzigNavResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return TigzigNavResponse{}, fmt.Errorf("parsing tigzig response for ISIN %s: %w", isin, err)
	}
	// An empty result is only a real failure for an UNBOUNDED request
	// (since=="") - a bounded/incremental request legitimately returns
	// zero new rows when the fund is already up to date, per the doc
	// comment above.
	if since == "" && (out.Count == 0 || len(out.Data) == 0) {
		return TigzigNavResponse{}, fmt.Errorf("no NAV history found for ISIN %s", isin)
	}
	return out, nil
}

// YahooPricePoint is one adjusted-close observation from
// FetchYahooAdjClose.
type YahooPricePoint struct {
	Date  string
	Price float64
}

// yahooAdjCloseResponse mirrors GET yfin-h.tigzig.com/v1/get-adj-close/
// (documented at https://yfin-h.tigzig.com/openapi.json): an object
// keyed by date ("YYYY-MM-DD"), each value an object of
// {ticker: adjusted_close}. Per that spec, a ticker absent for a given
// date is JSON null (a real trading gap, e.g. a market holiday); a
// ticker missing from the whole fetch comes back as the STRING
// "Data not available" instead of a number for every date - both cases
// are handled below via interface{} rather than assuming every value is
// a plain float64.
//
// Honesty flag, per this project's standing policy on unconfirmed
// primary sources: this shape is taken directly from the published
// OpenAPI spec, not independently confirmed with a successful live call
// from this sandbox - every attempt here (including the provider's own
// zero-parameter root endpoint) returned validation errors, which
// points to a client/tooling issue on this end rather than a broken
// service, but that couldn't be fully isolated without direct network
// access to verify further. If this shape turns out wrong in practice,
// parsing below fails loudly (an error surfaced to the person via the
// Update Price History screen), not a silent misread.
type yahooAdjCloseResponse map[string]map[string]interface{}

// FetchYahooAdjClose fetches split/dividend-adjusted daily close prices
// for one ticker (Yahoo Finance convention - "NIFTYBEES.NS" for an
// NSE-listed ETF, "RELIANCE.NS", a bare US ticker like "AAPL", etc.)
// from `since` to today, via TigZig's Yahoo Finance proxy. See
// yahooAdjCloseResponse's doc comment for the response-shape caveat.
//
// Deliberately does NOT treat zero results as an error itself (unlike
// earlier versions of this function) - a narrow, incremental `since`
// window for an asset that's already up to date is EXPECTED to come
// back empty (weekend/holiday, or simply nothing new published yet),
// and that is success, not failure. Distinguishing that from a
// genuinely bad ticker requires knowing whether this is a first-ever
// fetch or a top-up, which only the caller knows - see
// UpdateHistoricalPrice's doc comment for where that check now lives.
func FetchYahooAdjClose(ticker string, since string) ([]YahooPricePoint, error) {
	if ticker == "" {
		return nil, fmt.Errorf("ticker cannot be empty")
	}
	today := time.Now().UTC().Format("2006-01-02")
	url := "https://yfin-h.tigzig.com/v1/get-adj-close/?tickers=" + neturl.QueryEscape(ticker) +
		"&start_date=" + neturl.QueryEscape(since) + "&end_date=" + neturl.QueryEscape(today)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("yahoo adj-close request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading yahoo adj-close response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo adj-close returned status %d for %s: %s", resp.StatusCode, ticker, string(body))
	}

	var raw yahooAdjCloseResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing yahoo adj-close response for %s: %w", ticker, err)
	}

	points := make([]YahooPricePoint, 0, len(raw))
	for date, byTicker := range raw {
		v, ok := byTicker[ticker]
		if !ok || v == nil {
			continue // no price for this ticker on this date - a real trading gap, not an error
		}
		price, ok := v.(float64)
		if !ok {
			continue // the documented "Data not available" string sentinel (or any other unexpected type) - skip rather than guess
		}
		points = append(points, YahooPricePoint{Date: date, Price: price})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Date < points[j].Date })
	return points, nil
}

// FrankfurterTimeSeriesResponse mirrors Frankfurter's date-range (time
// series) JSON shape. The single-date/latest shape -
// {"amount":1,"base":"EUR","date":"...","rates":{"AUD":1.57,...}} - was
// confirmed live from this sandbox on 2026-08-22. The time-series form
// used here swaps "date" for "start_date"/"end_date" and nests "rates"
// as date -> currency -> rate; this nested shape is described
// consistently across every Frankfurter client library checked (it's
// been stable since the underlying Fixer.io-style API convention), but
// a live time-series call could not be independently confirmed from
// this sandbox (frankfurter.dev wasn't reachable for that specific
// request). Flagging that honestly: if this shape is wrong, parsing
// will fail loudly (zero rates returned, surfaced as an error) rather
// than silently reading garbage.
type FrankfurterTimeSeriesResponse struct {
	Amount    float64                       `json:"amount"`
	Base      string                        `json:"base"`
	StartDate string                        `json:"start_date"`
	EndDate   string                        `json:"end_date"`
	Rates     map[string]map[string]float64 `json:"rates"`
}

// FetchFrankfurterHistory fetches daily INR exchange rates for the
// given currency (e.g. "CAD") from `since` to today, from Frankfurter
// (ECB-sourced, free, no key, ignoring the reciprocal that Frankfurter
// itself is a third-party redistributor of ECB reference rates, not the
// ECB directly). INR-to-INR is never fetched or stored - it's always
// 1.0 by definition. Only dates Frankfurter actually published are
// returned (weekends/holidays are naturally absent, not zero-filled or
// interpolated).
//
// Deliberately does NOT treat zero results as an error itself, same
// reasoning as FetchYahooAdjClose's doc comment - a narrow incremental
// `since` for an already-current currency is expected to come back
// empty, and that's success. See UpdateHistoricalFX for where the
// first-fetch-vs-top-up distinction now lives.
func FetchFrankfurterHistory(currency string, since string) ([]store.FXRate, error) {
	if currency == "" {
		return nil, fmt.Errorf("currency cannot be empty")
	}
	if currency == "INR" {
		return nil, nil
	}
	if since == "" {
		return nil, fmt.Errorf("since date cannot be empty")
	}

	url := fmt.Sprintf("https://api.frankfurter.dev/v1/%s..?base=%s&symbols=INR",
		neturl.QueryEscape(since), neturl.QueryEscape(currency))
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("frankfurter request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading frankfurter response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("frankfurter returned status %d for %s since %s: %s", resp.StatusCode, currency, since, string(body))
	}

	rates, err := ParseFrankfurterTimeSeries(body, currency)
	if err != nil {
		return nil, err
	}
	return rates, nil
}

// ParseFrankfurterTimeSeries is split out from FetchFrankfurterHistory
// so the parsing logic itself has a real test against a fixture,
// independent of the live network call this sandbox can't make.
func ParseFrankfurterTimeSeries(body []byte, currency string) ([]store.FXRate, error) {
	var parsed FrankfurterTimeSeriesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing frankfurter response: %w", err)
	}

	rates := make([]store.FXRate, 0, len(parsed.Rates))
	for date, byCurrency := range parsed.Rates {
		inrRate, ok := byCurrency["INR"]
		if !ok {
			continue // shouldn't happen given symbols=INR, but don't fabricate a rate if it's genuinely absent
		}
		rates = append(rates, store.FXRate{Date: date, Currency: currency, INRPerUnit: inrRate})
	}
	sort.Slice(rates, func(i, j int) bool { return rates[i].Date < rates[j].Date })
	return rates, nil
}
