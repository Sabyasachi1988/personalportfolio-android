package casimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ledger/internal/store"
)

var (
	folioNoLineRe = regexp.MustCompile(`^FOLIO NO:\s*(\d+)\s*$`)
	isinLineRe    = regexp.MustCompile(`ISIN:\s*([A-Z0-9]{12})\s*$`)
	dateOnlyRe    = regexp.MustCompile(`^\d{2}-[A-Z]{3}-\d{4}$`)
	numericLineRe = regexp.MustCompile(`^\(?[\d,]+\.\d+\)?$`)

	monthAbbrev2 = map[string]string{
		"JAN": "01", "FEB": "02", "MAR": "03", "APR": "04", "MAY": "05", "JUN": "06",
		"JUL": "07", "AUG": "08", "SEP": "09", "OCT": "10", "NOV": "11", "DEC": "12",
	}
)

type rawMFCTxn struct {
	date, desc, amt, units, price, bal string
	amc, folio, scheme, isin           string
	page                                int
}

// ParseMFCentral parses MFCentral-format CAS text extracted via
// page.GetPlainText() (real per-line structure - date, description, and
// each of the four running numbers each on their own line). Nothing is
// ever silently dropped: anything that can't be turned into a transaction
// lands in ManualReview instead.
func ParseMFCentral(pageTexts []string) ImportResult {
	result := ImportResult{Format: "MFCENTRAL"}

	lines, pageOf := buildLines(pageTexts)
	if len(lines) == 0 {
		return result
	}

	var currentAMC, currentFolio, currentScheme, currentISIN string
	var pendingAMCCandidate string
	var raw []rawMFCTxn

	i := 0
	for i < len(lines) {
		line := lines[i]
		page := pageOf[i]

		switch {
		case folioNoLineRe.MatchString(line):
			m := folioNoLineRe.FindStringSubmatch(line)
			currentFolio = m[1]
			if pendingAMCCandidate != "" {
				currentAMC = pendingAMCCandidate
			}
			pendingAMCCandidate = ""
			i++

		case strings.Contains(line, "ISIN:"):
			if im := isinLineRe.FindStringSubmatch(line); im != nil {
				currentISIN = im[1]
			}
			currentScheme = cutSchemeName(line)
			pendingAMCCandidate = ""
			i++

		case strings.HasPrefix(line, "KYC"),
			strings.HasPrefix(line, "Opening Unit Balance"),
			strings.HasPrefix(line, "Closing Unit Balance"),
			strings.HasPrefix(line, "Nav as on"),
			strings.HasPrefix(line, "Valuation on"),
			strings.Contains(line, "No Folios Found"):
			pendingAMCCandidate = ""
			i++

		case dateOnlyRe.MatchString(line):
			if i+5 >= len(lines) {
				result.ManualReview = append(result.ManualReview, ManualReviewLine{
					Page:   page,
					Folio:  currentFolio,
					Reason: fmt.Sprintf("row dated %s near end of document: not enough following lines for a full row", line),
					Text:   line,
				})
				i++
				continue
			}
			desc, amt, units, price, bal := lines[i+1], lines[i+2], lines[i+3], lines[i+4], lines[i+5]
			if !numericLineRe.MatchString(amt) || !numericLineRe.MatchString(units) ||
				!numericLineRe.MatchString(price) || !numericLineRe.MatchString(bal) {
				result.ManualReview = append(result.ManualReview, ManualReviewLine{
					Page:   page,
					Folio:  currentFolio,
					Reason: fmt.Sprintf("row dated %s: the next 4 lines don't all look numeric", line),
					Text:   fmt.Sprintf("%s | %s | %s | %s | %s", desc, amt, units, price, bal),
				})
				i++ // resync by just skipping the date line, not the whole block
				continue
			}
			raw = append(raw, rawMFCTxn{
				date: line, desc: desc, amt: amt, units: units, price: price, bal: bal,
				amc: currentAMC, folio: currentFolio, scheme: currentScheme, isin: currentISIN,
				page: page,
			})
			i += 6

		default:
			// Not a recognised marker: most likely the AMC-name line that
			// precedes a "FOLIO NO:" line when the AMC has just changed.
			// Only trusted if the very next relevant line turns out to
			// actually be a FOLIO NO line (handled above).
			pendingAMCCandidate = line
			i++
		}
	}

	// Merge page-break continuation rows: identical date/amount/units/
	// price/balance to the row directly above means this is the second
	// half of a wrapped description (MFCentral's own renderer duplicates
	// the numeric columns when a description wraps across a page break),
	// not a new transaction. The merged row keeps the FIRST half's page
	// number, since that's where the transaction actually starts.
	merged := make([]rawMFCTxn, 0, len(raw))
	for _, r := range raw {
		if n := len(merged); n > 0 && merged[n-1].date == r.date && merged[n-1].amt == r.amt &&
			merged[n-1].units == r.units && merged[n-1].price == r.price && merged[n-1].bal == r.bal {
			merged[n-1].desc = strings.TrimSpace(merged[n-1].desc + " " + r.desc)
			continue
		}
		merged = append(merged, r)
	}

	for _, r := range merged {
		txn, reason := buildMFCTransaction(r)
		if reason != "" {
			result.ManualReview = append(result.ManualReview, ManualReviewLine{
				Page:   r.page,
				Folio:  r.folio,
				Reason: reason,
				Text:   fmt.Sprintf("%s | %s | %s %s %s %s", r.date, r.desc, r.amt, r.units, r.price, r.bal),
			})
			continue
		}
		result.Staged = append(result.Staged, StagedRow{Txn: txn, Status: "NEW", SourceFolio: r.folio, SourcePage: r.page})
	}

	return result
}

// buildLines strips each page's repeated banner/table header (marked by
// the two-line sequence "Unit Balance" / "Date" that ends every page's
// header block) and returns the remaining real content as trimmed,
// non-empty lines, concatenated across pages in order, along with a
// parallel slice giving the 1-based source page number for each line.
// Pages with no such header (title page, "No Folios Found" pages)
// contribute nothing.
func buildLines(pageTexts []string) ([]string, []int) {
	var all []string
	var pageOf []int
	for pageIdx, pt := range pageTexts {
		pageLines := strings.Split(pt, "\n")
		start := -1
		for j := 0; j+1 < len(pageLines); j++ {
			if strings.TrimSpace(pageLines[j]) == "Unit Balance" && strings.TrimSpace(pageLines[j+1]) == "Date" {
				start = j + 2
				break
			}
		}
		if start == -1 {
			continue
		}
		for _, l := range pageLines[start:] {
			l = strings.TrimSpace(l)
			if l != "" {
				all = append(all, l)
				pageOf = append(pageOf, pageIdx+1) // 1-based
			}
		}
	}
	return all, pageOf
}

// cutSchemeName trims the scheme-name line down to just the fund name,
// dropping the trailing "(Advisor: ...) ISIN: ..." segment.
func cutSchemeName(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "(Advisor"); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}

func buildMFCTransaction(r rawMFCTxn) (store.Transaction, string) {
	date, ok := parseMFCDate(r.date)
	if !ok {
		return store.Transaction{}, fmt.Sprintf("could not parse date %q", r.date)
	}
	amount, ok := parseSignedNumber(r.amt)
	if !ok {
		return store.Transaction{}, fmt.Sprintf("could not parse amount %q", r.amt)
	}
	units, ok := parseSignedNumber(r.units)
	if !ok {
		return store.Transaction{}, fmt.Sprintf("could not parse units %q", r.units)
	}
	price, okP := parseSignedNumber(r.price)
	bal, okB := parseSignedNumber(r.bal)

	typ, confident := Classify(r.desc)
	if !confident {
		return store.Transaction{}, fmt.Sprintf("unrecognised transaction description %q", r.desc)
	}

	if typ == "REDEMPTION" || typ == "SWITCH_OUT" || typ == "SWITCH_OUT_MERGER" {
		amount = -absFloat(amount)
		units = -absFloat(units)
	}

	txn := store.Transaction{
		Date:        date,
		Description: strings.TrimSpace(r.desc),
		Amount:      amount,
		Units:       &units,
		Type:        store.TransactionType(typ),
		AMC:         r.amc,
		Folio:       r.folio,
		Scheme:      r.scheme,
		ISIN:        r.isin,
	}
	if okP {
		txn.Price = &price
	}
	if okB {
		txn.Balance = &bal
	}
	return txn, ""
}

func parseMFCDate(s string) (string, bool) {
	if !dateOnlyRe.MatchString(s) {
		return "", false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return "", false
	}
	day, mon, year := parts[0], strings.ToUpper(parts[1]), parts[2]
	mm, ok := monthAbbrev2[mon]
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s-%s-%s", year, mm, day), true
}

// parseSignedNumber handles MFCentral's accounting-style negatives, e.g.
// "(4,918.00)" -> -4918.00, as well as plain "24,998.75" -> 24998.75.
func parseSignedNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	negative := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		negative = true
		s = s[1 : len(s)-1]
	}
	s = strings.ReplaceAll(s, ",", "")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	if negative {
		v = -v
	}
	return v, true
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
