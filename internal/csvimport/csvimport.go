// Package csvimport parses generic mutual-fund transaction CSV exports
// (broker tradebooks, RTA statements, or hand-built spreadsheets) into
// the same casimport.StagedRow shape ImportCAS produces, so CSV-sourced
// rows flow through the identical staging/review/commit UI already built
// for PDF-sourced CAS imports.
//
// "Form factor agnostic" here means column-NAME based, not column-
// POSITION based: the header row is matched against a table of known
// aliases per logical field, so a Zerodha Console tradebook export
// ("symbol", "trade_date", "trade_type", "quantity"), a Groww/Kuvera
// export, or a hand-built CSV with slightly different header wording can
// all be recognized without hardcoding one specific layout. Built and
// verified against a real Zerodha Console MF tradebook export.
package csvimport

import (
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ledger/internal/casimport"
	"ledger/internal/store"
)

// columnAliases maps each logical field this importer understands to the
// header names it might appear under across different platforms' CSV
// exports. Matching is case-insensitive and whitespace-trimmed; the
// first alias found in the header wins for each field.
var columnAliases = map[string][]string{
	"date":      {"trade_date", "date", "transaction date", "txn date", "order date", "value date", "posting date"},
	"scheme":    {"symbol", "scheme", "scheme name", "fund", "fund name", "security name", "particulars", "instrument"},
	"isin":      {"isin", "isin code", "isin no", "isin number"},
	"type":      {"trade_type", "type", "transaction type", "txn type", "order type", "buy/sell", "buy_sell"},
	"quantity":  {"quantity", "units", "qty", "unit", "unit balance"},
	"price":     {"price", "nav", "rate", "trade price", "unit price"},
	"amount":    {"amount", "value", "trade value", "net amount", "total amount", "gross amount"},
	"folio":     {"folio", "folio no", "folio number"},
	"reference": {"trade_id", "order_id", "reference", "reference number", "txn id", "transaction id"},
}

// Fields a usable CSV must have a recognized column for. amount, isin,
// and folio are all optional: amount can be derived from quantity*price,
// isin-less rows still get matched/created by scheme name, and folio is
// a CAS-statement concept most broker CSVs don't carry at all.
var requiredFields = []string{"date", "scheme", "type", "quantity", "price"}

// dateLayouts are tried in order against whatever's in the detected date
// column - real-world exports vary widely in date formatting.
var dateLayouts = []string{
	"2006-01-02",
	"2006-01-02T15:04:05",
	"02-01-2006",
	"02/01/2006",
	"2-Jan-2006",
	"02-Jan-2006",
	"Jan 2, 2006",
	"2006/01/02",
}

// ParseCSV parses raw CSV bytes into the same casimport.ImportResult
// shape ImportCAS produces. Never returns a Go error - anything it can't
// confidently parse is routed into ManualReview rather than silently
// dropped or guessed at, matching the CAS importer's own principle.
func ParseCSV(data []byte) casimport.ImportResult {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.FieldsPerRecord = -1 // tolerate ragged rows rather than rejecting the whole file
	records, err := reader.ReadAll()
	if err != nil {
		return casimport.ImportResult{
			Format: "CSV",
			ManualReview: []casimport.ManualReviewLine{
				{Text: err.Error(), Reason: "could not parse file as CSV"},
			},
		}
	}
	if len(records) == 0 {
		return casimport.ImportResult{Format: "CSV"}
	}

	colIndex, missing := mapColumns(records[0])
	if len(missing) > 0 {
		return casimport.ImportResult{
			Format: "CSV",
			ManualReview: []casimport.ManualReviewLine{
				{
					Text:   strings.Join(records[0], ","),
					Reason: "could not find a recognizable column for: " + strings.Join(missing, ", ") + " - check the header row matches one of the supported naming conventions",
				},
			},
		}
	}

	result := casimport.ImportResult{Format: "CSV"}
	for i, row := range records[1:] {
		lineNum := i + 2 // 1-based, +1 to account for the header row
		if isBlankRow(row) {
			continue
		}
		txn, reason := buildTransactionFromCSVRow(row, colIndex)
		if reason != "" {
			result.ManualReview = append(result.ManualReview, casimport.ManualReviewLine{
				Page: lineNum, Text: strings.Join(row, ","), Reason: reason,
			})
			continue
		}
		result.Staged = append(result.Staged, casimport.StagedRow{
			Txn: txn, Status: "NEW", SourcePage: lineNum, SourceFolio: txn.Folio,
		})
	}
	return result
}

func isBlankRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

func mapColumns(header []string) (map[string]int, []string) {
	normalized := make([]string, len(header))
	for i, h := range header {
		normalized[i] = strings.ToLower(strings.TrimSpace(h))
	}

	colIndex := make(map[string]int)
	for field, aliases := range columnAliases {
		for _, alias := range aliases {
			found := false
			for i, h := range normalized {
				if h == alias {
					colIndex[field] = i
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	var missing []string
	for _, required := range requiredFields {
		if _, ok := colIndex[required]; !ok {
			missing = append(missing, required)
		}
	}
	return colIndex, missing
}

func buildTransactionFromCSVRow(row []string, col map[string]int) (store.Transaction, string) {
	get := func(field string) string {
		idx, ok := col[field]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	dateStr := get("date")
	date, ok := parseCSVDate(dateStr)
	if !ok {
		return store.Transaction{}, fmt.Sprintf("could not parse date %q", dateStr)
	}

	scheme := get("scheme")
	if scheme == "" {
		return store.Transaction{}, "missing scheme/fund name"
	}

	typeStr := get("type")
	typ, confident := classifyCSVType(typeStr)
	if !confident {
		return store.Transaction{}, fmt.Sprintf("unrecognised transaction type %q", typeStr)
	}

	quantityStr := get("quantity")
	quantity, okQ := parsePlainFloat(quantityStr)
	if !okQ {
		return store.Transaction{}, fmt.Sprintf("could not parse quantity %q", quantityStr)
	}

	priceStr := get("price")
	price, okP := parsePlainFloat(priceStr)

	var amount float64
	if amtStr := get("amount"); amtStr != "" {
		a, okA := parsePlainFloat(amtStr)
		switch {
		case okA:
			amount = absFloat(a)
		case okP:
			amount = absFloat(quantity * price)
		default:
			return store.Transaction{}, fmt.Sprintf("could not parse amount %q", amtStr)
		}
	} else if okP {
		amount = absFloat(quantity * price)
	} else {
		return store.Transaction{}, "no amount column present, and price could not be parsed to compute one from quantity"
	}

	// Sign convention matches casimport.buildMFCTransaction exactly:
	// redemptions/switch-outs get both Amount and Units negated.
	isOutflow := typ == "REDEMPTION" || typ == "SWITCH_OUT" || typ == "SWITCH_OUT_MERGER"
	if isOutflow {
		amount = -amount
		quantity = -absFloat(quantity)
	} else {
		quantity = absFloat(quantity)
	}

	var priceForTxn *float64
	if okP {
		p := price
		priceForTxn = &p
	}

	units := quantity
	description := strings.TrimSpace(typeStr + " " + scheme)
	// Tag the description with the broker's own trade reference when
	// the CSV carries one (e.g. Zerodha's trade_id) - this is what lets
	// a re-imported statement be recognized with certainty rather than
	// only by amount/units happening to coincide (see transactionsMatch
	// in mobile/bridge/bridge.go, which looks for this exact "[ref:...]"
	// marker).
	if ref := get("reference"); ref != "" {
		description = description + " [ref:" + ref + "]"
	}

	txn := store.Transaction{
		Date:        date,
		Description: description,
		Amount:      amount,
		Units:       &units,
		Price:       priceForTxn,
		Type:        store.TransactionType(typ),
		Folio:       get("folio"),
		Scheme:      scheme,
		ISIN:        get("isin"),
	}
	return txn, ""
}

func parseCSVDate(s string) (string, bool) {
	if s == "" {
		return "", false
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
}

// classifyCSVType handles the terse buy/sell vocabulary common in broker
// trade CSVs (which casimport.Classify, built for long-form CAS
// descriptions like "Purchase - Systematic Investment", doesn't
// recognize as a bare "buy"/"sell"), then falls back to Classify itself
// so a CSV using richer wording (e.g. "Redemption", "SIP Purchase") is
// still recognized without duplicating that logic here.
func classifyCSVType(s string) (typ string, confident bool) {
	d := strings.ToLower(strings.TrimSpace(s))
	switch d {
	case "buy", "purchase", "b":
		return "PURCHASE", true
	case "sell", "redeem", "redemption", "s":
		return "REDEMPTION", true
	}
	return casimport.Classify(s)
}

// parsePlainFloat strips thousands-separator commas before parsing,
// since several real-world exports format quantity/price/amount with
// them (e.g. "1,741.77").
func parsePlainFloat(s string) (float64, bool) {
	cleaned := strings.ReplaceAll(strings.TrimSpace(s), ",", "")
	if cleaned == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
