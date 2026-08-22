// Package priceapi fetches mutual fund NAVs (AMFI) and stock/ETF quotes
// (Yahoo Finance) for pricing portfolio holdings.
package priceapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// NavRecord is one parsed line of AMFI's NAVAll.txt.
type NavRecord struct {
	SchemeCode   string
	ISINPayout   string // "" if AMFI printed "-"
	ISINReinvest string // "" if AMFI printed "-"
	SchemeName   string
	Plan         string // "" for the pre-2026 format, which had no Plan/Option columns
	Option       string
	NAV          float64
	Date         string // as printed, e.g. "20-Aug-2026"
	Category     string // raw AMFI section header, e.g. "Open Ended Schemes(Equity Scheme - Large Cap Fund)"
	AssetClass   string // parsed from Category: "Equity", "Debt", "Hybrid", "Solution Oriented", "Other", or "" if unknown
}

// ParseAmfiText parses AMFI's NAVAll.txt content. It does not assume a
// fixed column count: AMFI inserted "Plan" and "Option" columns before
// NAV/Date in 2026 (6 fields -> 8), and may do so again. Instead it relies
// on invariants that have held across both formats:
//   - Scheme Code, ISIN Payout, ISIN Reinvest, Scheme Name are always the
//     first 4 semicolon-separated fields, in that order.
//   - NAV and Date are always the LAST two fields, whatever else sits
//     between them and Scheme Name.
//
// Section headers ("Open Ended Schemes(...)"), AMC name lines, and blank
// lines have no semicolons (or too few fields) and are skipped rather
// than treated as errors. A line that has some semicolons but not enough
// fields to be real data is reported as a skipped-line count, not
// silently dropped from view - callers can inspect that to catch a
// genuine future format break rather than getting a quiet zero-schemes
// result like the bug this was written to fix.
func ParseAmfiText(text string) (records []NavRecord, skippedDataLikeLines int, err error) {
	lines := strings.Split(text, "\n")
	currentCategory := ""
	currentAssetClass := ""
	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		fields := strings.Split(line, ";")
		if len(fields) < 6 {
			// Section header ("Open Ended Schemes(Equity Scheme - Large Cap
			// Fund)"), AMC name line, or similar - not a data row. Section
			// headers are the real, confirmed AMFI format: "Open Ended
			// Schemes(<Asset Class> Scheme - <Sub-Category>)". Capture the
			// asset class so later records can carry it.
			if cat, class, ok := parseCategoryHeader(trimmed); ok {
				currentCategory = cat
				currentAssetClass = class
			}
			continue
		}
		if strings.EqualFold(strings.TrimSpace(fields[0]), "Scheme Code") {
			// The column header row itself - expected, not an error.
			continue
		}

		navStr := strings.TrimSpace(fields[len(fields)-2])
		dateStr := strings.TrimSpace(fields[len(fields)-1])
		nav, navErr := strconv.ParseFloat(navStr, 64)
		if navErr != nil {
			// Has enough fields to look like data but the NAV column
			// isn't a number - genuinely unparseable, not a header line.
			// Count it rather than pretending it succeeded.
			skippedDataLikeLines++
			continue
		}

		rec := NavRecord{
			SchemeCode:   strings.TrimSpace(fields[0]),
			ISINPayout:   dashToEmpty(fields[1]),
			ISINReinvest: dashToEmpty(fields[2]),
			SchemeName:   strings.TrimSpace(fields[3]),
			NAV:          nav,
			Date:         dateStr,
			Category:     currentCategory,
			AssetClass:   currentAssetClass,
		}
		if len(fields) >= 8 {
			rec.Plan = strings.TrimSpace(fields[4])
			rec.Option = strings.TrimSpace(fields[5])
		}
		records = append(records, rec)
	}
	return records, skippedDataLikeLines, nil
}

// parseCategoryHeader recognises AMFI's real section-header format,
// confirmed live: "Open Ended Schemes(Equity Scheme - Large Cap Fund)",
// "Open Ended Schemes(Debt Scheme - Banking and PSU Fund)", etc. Also
// handles Close Ended and Interval schemes, which use the same shape.
// Returns ok=false for anything else (AMC name lines, blank separators).
var categoryHeaderRe = regexp.MustCompile(`^(?:Open|Close|Interval) Ended Schemes\(([^-]+) Scheme\s*-?\s*(.*)\)$`)

func parseCategoryHeader(line string) (category, assetClass string, ok bool) {
	m := categoryHeaderRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return line, strings.TrimSpace(m[1]), true
}

func dashToEmpty(s string) string {
	s = strings.TrimSpace(s)
	if s == "-" {
		return ""
	}
	return s
}

const amfiURL = "https://www.amfiindia.com/spages/NAVAll.txt"

// FetchAmfiNav downloads and parses the current AMFI NAV file.
func FetchAmfiNav() ([]NavRecord, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(amfiURL)
	if err != nil {
		return nil, fmt.Errorf("fetching AMFI NAV file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AMFI NAV file: unexpected status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading AMFI NAV file: %w", err)
	}
	records, _, err := ParseAmfiText(string(body))
	if err != nil {
		return nil, err
	}
	return records, nil
}

// YahooQuote is a minimal stock/ETF quote.
type YahooQuote struct {
	Symbol   string
	Price    float64
	Currency string
	AsOf     time.Time
}

// yahooChartResponse mirrors just the fields we need from
// https://query1.finance.yahoo.com/v8/finance/chart/<symbol>
type yahooChartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				Currency           string  `json:"currency"`
				RegularMarketTime  int64   `json:"regularMarketTime"`
			} `json:"meta"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

// FetchYahooQuote fetches a current quote for a symbol (e.g.
// "RELIANCE.NS" for NSE-listed stocks/ETFs).
func FetchYahooQuote(symbol string) (YahooQuote, error) {
	url := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s", symbol)
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return YahooQuote{}, err
	}
	// Yahoo's endpoint 403s without a browser-like User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	resp, err := client.Do(req)
	if err != nil {
		return YahooQuote{}, fmt.Errorf("fetching Yahoo quote for %s: %w", symbol, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return YahooQuote{}, fmt.Errorf("Yahoo quote for %s: unexpected status %d", symbol, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return YahooQuote{}, err
	}
	var parsed yahooChartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return YahooQuote{}, fmt.Errorf("parsing Yahoo response for %s: %w", symbol, err)
	}
	if len(parsed.Chart.Result) == 0 {
		return YahooQuote{}, fmt.Errorf("Yahoo quote for %s: no result (symbol may be wrong)", symbol)
	}
	meta := parsed.Chart.Result[0].Meta
	return YahooQuote{
		Symbol:   symbol,
		Price:    meta.RegularMarketPrice,
		Currency: meta.Currency,
		AsOf:     time.Unix(meta.RegularMarketTime, 0),
	}, nil
}

// ConnectivityResult is one line of a connectivity test.
type ConnectivityResult struct {
	Label   string
	OK      bool
	Message string
}

// RunConnectivityTest checks both data sources and returns a status line
// for each, for display in a "Connectivity test" dialog.
func RunConnectivityTest(yahooTestSymbol string) []ConnectivityResult {
	var results []ConnectivityResult

	records, err := FetchAmfiNav()
	switch {
	case err != nil:
		results = append(results, ConnectivityResult{
			Label: "AMFI (mutual funds)", OK: false,
			Message: fmt.Sprintf("could not fetch AMFI file — %v", err),
		})
	case len(records) == 0:
		results = append(results, ConnectivityResult{
			Label: "AMFI (mutual funds)", OK: false,
			Message: "AMFI file fetched but zero schemes parsed — the file format may have changed",
		})
	default:
		results = append(results, ConnectivityResult{
			Label: "AMFI (mutual funds)", OK: true,
			Message: fmt.Sprintf("OK — %d schemes parsed", len(records)),
		})
	}

	quote, err := FetchYahooQuote(yahooTestSymbol)
	if err != nil {
		results = append(results, ConnectivityResult{
			Label: "Yahoo Finance (stocks/ETFs)", OK: false,
			Message: fmt.Sprintf("could not fetch %s — %v", yahooTestSymbol, err),
		})
	} else {
		results = append(results, ConnectivityResult{
			Label: "Yahoo Finance (stocks/ETFs)", OK: true,
			Message: fmt.Sprintf("OK — %s = %.2f %s (%s)", quote.Symbol, quote.Price, quote.Currency, quote.AsOf.Format("2006-01-02")),
		})
	}

	return results
}
