package csvimport

import (
	"testing"
)

// This is the header + first two rows of a real Zerodha Console MF
// tradebook CSV export, used as the primary test case since that's the
// concrete format this was built against.
const zerodhaSample = `symbol,isin,trade_date,exchange,segment,series,trade_type,auction,quantity,price,trade_id,order_id,order_execution_time
NAVI ELSS TAX SAVER NIFTY 50 INDEX FUND - DIRECT PLAN,INF959L01GR6,2025-01-03,BSE,MF,,buy,false,1741.770000,14.352500,1582636286,1582636286,2025-01-03T00:00:00
NIPPON INDIA INDEX FUND - NIFTY 50 PLAN - DIRECT PLAN,INF204K01H36,2025-01-07,BSE,MF,,buy,false,582.072000,42.947900,1594748061,1594748061,2025-01-07T00:00:00
`

func TestParseCSV_RealZerodhaTradebookFormat(t *testing.T) {
	result := ParseCSV([]byte(zerodhaSample))

	if len(result.ManualReview) != 0 {
		t.Fatalf("expected no manual review lines, got %+v", result.ManualReview)
	}
	if len(result.Staged) != 2 {
		t.Fatalf("expected 2 staged rows, got %d", len(result.Staged))
	}

	first := result.Staged[0].Txn
	if first.Date != "2025-01-03" {
		t.Errorf("Date = %q, want 2025-01-03", first.Date)
	}
	if first.ISIN != "INF959L01GR6" {
		t.Errorf("ISIN = %q, want INF959L01GR6", first.ISIN)
	}
	if first.Scheme != "NAVI ELSS TAX SAVER NIFTY 50 INDEX FUND - DIRECT PLAN" {
		t.Errorf("Scheme = %q, unexpected", first.Scheme)
	}
	if first.Type != "PURCHASE" {
		t.Errorf("Type = %q, want PURCHASE", first.Type)
	}
	if first.Units == nil || *first.Units != 1741.77 {
		t.Errorf("Units = %v, want 1741.77", first.Units)
	}
	// quantity * price = 1741.77 * 14.3525 = 24998.7568425 - amount is
	// derived since there's no explicit amount column in this format.
	wantAmount := 1741.77 * 14.3525
	if diff := first.Amount - wantAmount; diff > 0.01 || diff < -0.01 {
		t.Errorf("Amount = %v, want ~%v (quantity*price)", first.Amount, wantAmount)
	}
	if first.Price == nil || *first.Price != 14.3525 {
		t.Errorf("Price = %v, want 14.3525", first.Price)
	}
}

func TestParseCSV_RedemptionGetsNegativeSign(t *testing.T) {
	csvData := `trade_date,symbol,isin,trade_type,quantity,price
2025-06-01,SOME FUND,INF000000001,sell,100.5,25.0
`
	result := ParseCSV([]byte(csvData))
	if len(result.Staged) != 1 {
		t.Fatalf("expected 1 staged row, got %d (manual review: %+v)", len(result.Staged), result.ManualReview)
	}
	txn := result.Staged[0].Txn
	if txn.Type != "REDEMPTION" {
		t.Errorf("Type = %q, want REDEMPTION", txn.Type)
	}
	if txn.Amount >= 0 {
		t.Errorf("Amount = %v, want negative (redemption)", txn.Amount)
	}
	if txn.Units == nil || *txn.Units >= 0 {
		t.Errorf("Units = %v, want negative (redemption)", txn.Units)
	}
}

func TestParseCSV_ColumnOrderAndNamingIsFlexible(t *testing.T) {
	// Deliberately different column ORDER and different header NAMES
	// than the Zerodha sample, to verify the importer is genuinely
	// column-name-based rather than accidentally position-dependent.
	csvData := `Fund Name,Units,Transaction Type,NAV,Date,ISIN Code
Some Other Fund - Direct Growth,50.25,Purchase,100.50,15-Mar-2025,INF888888888
`
	result := ParseCSV([]byte(csvData))
	if len(result.ManualReview) != 0 {
		t.Fatalf("expected no manual review lines, got %+v", result.ManualReview)
	}
	if len(result.Staged) != 1 {
		t.Fatalf("expected 1 staged row, got %d", len(result.Staged))
	}
	txn := result.Staged[0].Txn
	if txn.Date != "2025-03-15" {
		t.Errorf("Date = %q, want 2025-03-15 (from '15-Mar-2025')", txn.Date)
	}
	if txn.Scheme != "Some Other Fund - Direct Growth" {
		t.Errorf("Scheme = %q, unexpected", txn.Scheme)
	}
	if txn.ISIN != "INF888888888" {
		t.Errorf("ISIN = %q, unexpected", txn.ISIN)
	}
	if txn.Type != "PURCHASE" {
		t.Errorf("Type = %q, want PURCHASE", txn.Type)
	}
}

func TestParseCSV_ExplicitAmountColumnPreferredOverComputed(t *testing.T) {
	// If the CSV has its own Amount column, trust it rather than
	// recomputing from quantity*price - a broker's own stated amount may
	// include rounding/fees that quantity*price alone wouldn't capture.
	csvData := `trade_date,symbol,isin,trade_type,quantity,price,amount
2025-01-01,SOME FUND,INF000000001,buy,10,100,1005.50
`
	result := ParseCSV([]byte(csvData))
	if len(result.Staged) != 1 {
		t.Fatalf("expected 1 staged row, got %d (manual review: %+v)", len(result.Staged), result.ManualReview)
	}
	if result.Staged[0].Txn.Amount != 1005.50 {
		t.Errorf("Amount = %v, want 1005.50 (explicit column, not 1000 from quantity*price)", result.Staged[0].Txn.Amount)
	}
}

func TestParseCSV_MissingRequiredColumnGoesToManualReview(t *testing.T) {
	csvData := `some_column,another_column
foo,bar
`
	result := ParseCSV([]byte(csvData))
	if len(result.Staged) != 0 {
		t.Fatalf("expected 0 staged rows, got %d", len(result.Staged))
	}
	if len(result.ManualReview) != 1 {
		t.Fatalf("expected 1 manual review line describing the missing columns, got %d", len(result.ManualReview))
	}
}

func TestParseCSV_UnrecognisedRowGoesToManualReviewNotSkippedSilently(t *testing.T) {
	csvData := `trade_date,symbol,isin,trade_type,quantity,price
2025-01-01,GOOD FUND,INF000000001,buy,10,100
not-a-date,BAD ROW,INF000000002,buy,5,50
`
	result := ParseCSV([]byte(csvData))
	if len(result.Staged) != 1 {
		t.Errorf("expected 1 staged row (the good one), got %d", len(result.Staged))
	}
	if len(result.ManualReview) != 1 {
		t.Errorf("expected 1 manual review line (the bad date), got %d", len(result.ManualReview))
	}
}

func TestParseCSV_ThousandsSeparatorCommasHandled(t *testing.T) {
	csvData := `trade_date,symbol,isin,trade_type,quantity,price
2025-01-01,SOME FUND,INF000000001,buy,"1,741.77","14.35"
`
	result := ParseCSV([]byte(csvData))
	if len(result.Staged) != 1 {
		t.Fatalf("expected 1 staged row, got %d (manual review: %+v)", len(result.Staged), result.ManualReview)
	}
	if result.Staged[0].Txn.Units == nil || *result.Staged[0].Txn.Units != 1741.77 {
		t.Errorf("Units = %v, want 1741.77 (comma should be stripped)", result.Staged[0].Txn.Units)
	}
}
