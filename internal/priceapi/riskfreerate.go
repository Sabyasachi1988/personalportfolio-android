package priceapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
)

// AMCRiskFreeRateResult is one AMC factsheet's own disclosed risk-free
// rate assumption, extracted from its own footnote text - NOT a value
// this project invented or approximated. Confirmed live from multiple
// real factsheets before this was built: PPFAS ("Risk free rate
// assumed to be (FBIL Overnight MIBOR as on May 31, 2026) 5.52%"),
// Kotak ("Risk rate assumed to be 6.40% (FBIL Overnight MIBOR rate as
// on 31st May 2023)"), and Nippon India (same FBIL Overnight MIBOR
// citation in its own combined factsheet) - every major AMC's monthly
// factsheet states this exact figure verbatim, which is what makes
// scraping it directly (rather than approximating it) both possible
// and worth doing.
type AMCRiskFreeRateResult struct {
	RatePercent float64
	AsOfDate    string // exactly as extracted from the factsheet's own text (e.g. "31st May 2023") - display-only, not normalized to a fixed format since AMCs don't agree on one
	Source      string // which AMC's factsheet this came from, e.g. "PPFAS", "Nippon India"
}

// riskFreeRateBeforePercentRe and riskFreeRateAfterPercentRe both
// anchor on the confirmed "(FBIL ... Overnight MIBOR ...)" phrase
// every AMC's footnote contains, but allow the percentage figure to
// appear on EITHER side of it - AMCs' own PDFs order this differently
// (Kotak: number then parenthetical; PPFAS: parenthetical then
// number), and a PDF's table-layout text extraction can reorder
// nearby cells unpredictably regardless of the AMC's own visual
// layout, so both directions are tried rather than assuming one.
var (
	riskFreeRateBeforePercentRe = regexp.MustCompile(`(?i)risk\s*(?:free\s*)?rate[s]?\s*(?:assumed\s*to\s*be\s*)?:?\s*(\d+\.?\d*)\s*%\s*\(?\s*FBIL[^)]{0,60}Overnight MIBOR`)
	riskFreeRateAfterPercentRe  = regexp.MustCompile(`(?i)\(?\s*FBIL[^)]{0,60}Overnight MIBOR[^)]{0,60}\)?\s*\*{0,3}\s*(\d+\.?\d*)\s*%`)
	riskFreeRateAsOfRe          = regexp.MustCompile(`(?i)as\s*on\s*([^)]{4,40})\)`)
)

// ParseRiskFreeRateFootnote is FetchAMCRiskFreeRate's pure extraction
// logic, split out so it has a real test against captured real
// footnote text, independent of the live network fetch this sandbox
// can't make to an AMC's own site. Sanity-bounds the extracted number
// to [1, 15]% before accepting it - a real risk-free rate in India has
// sat in this range for decades, so anything outside it is far more
// likely a mis-extracted stray number from the surrounding table
// (portfolio turnover, a Beta, a Sharpe ratio) than a genuine rate,
// and should be treated as "not found" rather than trusted.
func ParseRiskFreeRateFootnote(text string) (ratePercent float64, asOfDate string, found bool) {
	m := riskFreeRateBeforePercentRe.FindStringSubmatch(text)
	if m == nil {
		m = riskFreeRateAfterPercentRe.FindStringSubmatch(text)
	}
	if m == nil {
		return 0, "", false
	}
	rate, err := strconv.ParseFloat(m[1], 64)
	if err != nil || rate < 1 || rate > 15 {
		return 0, "", false
	}
	asOf := ""
	if dm := riskFreeRateAsOfRe.FindStringSubmatch(text); dm != nil {
		asOf = strings.TrimSpace(dm[1])
	}
	return rate, asOf, true
}

// extractPDFText mirrors runImportCAS's own page-text-extraction loop
// in mobile/bridge/bridge.go (same pdf.NewReader(io.ReaderAt, size)
// approach, same per-page font-caching) - duplicated here rather than
// shared, since that function lives in the bridge package (which
// imports priceapi, so priceapi importing back from bridge would be a
// cycle) and this is a small enough routine that duplicating it is
// simpler than a new shared package just for this.
func extractPDFText(pdfBytes []byte) (string, error) {
	reader := bytes.NewReader(pdfBytes)
	r, err := pdf.NewReader(reader, int64(len(pdfBytes)))
	if err != nil {
		return "", fmt.Errorf("could not open PDF: %w", err)
	}
	fonts := make(map[string]*pdf.Font)
	fullText := ""
	for pageIdx := 1; pageIdx <= r.NumPage(); pageIdx++ {
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
			continue // same "skip a bad page rather than abort the whole import" principle as runImportCAS
		}
		fullText += text + "\n"
		// A risk-free-rate footnote always appears within a factsheet's
		// first handful of pages (it's per-fund, in each fund's own
		// summary block, not buried in back-matter) - stopping early
		// once there's plainly enough text to have found it keeps this
		// fast on factsheets that run to 100+ pages.
		if pageIdx >= 10 {
			break
		}
	}
	return fullText, nil
}

func fetchAMCRiskFreeRate(url, source string) (AMCRiskFreeRateResult, error) {
	resp, err := http.Get(url)
	if err != nil {
		return AMCRiskFreeRateResult{}, fmt.Errorf("%s factsheet request failed: %w", source, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AMCRiskFreeRateResult{}, fmt.Errorf("reading %s factsheet: %w", source, err)
	}
	if resp.StatusCode != http.StatusOK {
		return AMCRiskFreeRateResult{}, fmt.Errorf("%s factsheet returned status %d (URL: %s)", source, resp.StatusCode, url)
	}
	text, err := extractPDFText(body)
	if err != nil {
		return AMCRiskFreeRateResult{}, fmt.Errorf("%s factsheet PDF unreadable: %w", source, err)
	}
	rate, asOf, found := ParseRiskFreeRateFootnote(text)
	if !found {
		return AMCRiskFreeRateResult{}, fmt.Errorf("%s factsheet fetched but no risk-free-rate footnote found in it", source)
	}
	return AMCRiskFreeRateResult{RatePercent: rate, AsOfDate: asOf, Source: source}, nil
}

// FetchConsensusRiskFreeRate tries PPFAS's own monthly factsheet
// first (the exact source this whole feature was confirmed against),
// then falls back to Nippon India's if PPFAS's isn't reachable/
// parseable - both AMCs' factsheet URLs are predictable by month, and
// each is tried for the CURRENT calendar month first, then the
// PREVIOUS month, since a factsheet for the just-started month may
// not be published yet (AMCs publish a few days into the following
// month, not on day one). Returns the first successful source, not an
// average or a "most recent" comparison across sources - PPFAS's and
// Nippon's factsheets can have slightly different "as on" dates even
// within the same calendar month (MIBOR moves daily), so there's no
// single "more correct" answer to reconcile between two genuinely
// different, both-accurate snapshots; PPFAS is simply preferred as
// primary since it's the source this feature was originally verified
// against. If BOTH fail, all attempted URLs/errors are returned so the
// caller can report a a real "why" rather than a bare failure.
func FetchConsensusRiskFreeRate() (AMCRiskFreeRateResult, []error) {
	var errs []error
	now := time.Now().UTC()
	for _, monthsAgo := range []int{0, 1} {
		t := now.AddDate(0, -monthsAgo, 0)
		url := fmt.Sprintf("https://amc.ppfas.com/downloads/factsheet/%d/ppfas-mf-factsheet-for-%s-%d.pdf", t.Year(), t.Month().String(), t.Year())
		result, err := fetchAMCRiskFreeRate(url, "PPFAS")
		if err == nil {
			return result, nil
		}
		errs = append(errs, err)
	}
	for _, monthsAgo := range []int{0, 1} {
		t := now.AddDate(0, -monthsAgo, 0)
		url := fmt.Sprintf("https://mf.nipponindiaim.com/InvestorServices/FactSheets/NipponIndia-Factsheet-%s-%d.pdf", t.Month().String(), t.Year())
		result, err := fetchAMCRiskFreeRate(url, "Nippon India")
		if err == nil {
			return result, nil
		}
		errs = append(errs, err)
	}
	return AMCRiskFreeRateResult{}, errs
}
