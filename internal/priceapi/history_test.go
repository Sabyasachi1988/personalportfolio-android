package priceapi

import (
	"encoding/json"
	"testing"
)

// realTigzigFixture is a trimmed excerpt of a live response actually
// fetched from https://api.tigzig.com/mf/v1/nav?scheme=119775 on
// 2026-08-22 (first few and last few points kept, middle elided) - not
// a guessed shape.
const realTigzigFixture = `{"scheme_code":119775,"scheme_name":"Kotak Midcap Fund - Direct Plan - Growth","isin":"INF174K01LT0","isin2":null,"first_available_date":"2013-01-03","latest_available_date":"2026-08-21","count":3356,"data":[{"date":"2013-01-03","nav":14.052},{"date":"2013-01-04","nav":14.177},{"date":"2026-08-20","nav":172.846},{"date":"2026-08-21","nav":172.898}]}`

func TestTigzigNavResponse_ParsesRealFixture(t *testing.T) {
	var out TigzigNavResponse
	if err := json.Unmarshal([]byte(realTigzigFixture), &out); err != nil {
		t.Fatalf("failed to parse real TigZig fixture: %v", err)
	}
	if out.ISIN != "INF174K01LT0" {
		t.Errorf("ISIN = %q, want INF174K01LT0", out.ISIN)
	}
	if out.Count != 3356 {
		t.Errorf("Count = %d, want 3356", out.Count)
	}
	if len(out.Data) != 4 {
		t.Fatalf("expected 4 data points in trimmed fixture, got %d", len(out.Data))
	}
	if out.Data[0].Date != "2013-01-03" || out.Data[0].Nav != 14.052 {
		t.Errorf("first point = %+v, want {2013-01-03 14.052}", out.Data[0])
	}
	if out.Data[3].Date != "2026-08-21" || out.Data[3].Nav != 172.898 {
		t.Errorf("last point = %+v, want {2026-08-21 172.898}", out.Data[3])
	}
}

// realMfapiFixture mirrors mfapi.in's documented /mf/{code} response
// shape (confirmed live against mfapi.in's own docs page, cross-checked
// against an independent third-party captured example - see
// history.go's MfapiScheme doc comment) - DD-MM-YYYY dates, NAV as a
// JSON string, newest-first ordering as mfapi.in actually returns it.
const realMfapiFixture = `{"meta":{"fund_house":"HDFC Mutual Fund","scheme_type":"Open Ended Schemes","scheme_category":"Equity Scheme - Flexi Cap Fund","scheme_code":118955,"scheme_name":"HDFC Flexi Cap Fund - Direct Plan - Growth","isin_growth":"INF179K01BB2","isin_div_reinvestment":null},"data":[{"date":"27-08-2026","nav":"2297.09000"},{"date":"26-08-2026","nav":"2289.44000"},{"date":"01-01-2013","nav":"94.71200"}],"status":"SUCCESS"}`

func TestParseMfapiNavHistory_ParsesRealFixtureAndSortsAscending(t *testing.T) {
	points, err := ParseMfapiNavHistory([]byte(realMfapiFixture), "")
	if err != nil {
		t.Fatalf("failed to parse real mfapi.in fixture: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(points))
	}
	// Source is newest-first; parser must sort ascending like every
	// other price series in this codebase.
	if points[0].Date != "2013-01-01" || points[0].Nav != 94.712 {
		t.Errorf("first point = %+v, want {2013-01-01 94.712}", points[0])
	}
	if points[2].Date != "2026-08-27" || points[2].Nav != 2297.09 {
		t.Errorf("last point = %+v, want {2026-08-27 2297.09}", points[2])
	}
}

func TestParseMfapiNavHistory_SinceFiltersOlderRows(t *testing.T) {
	points, err := ParseMfapiNavHistory([]byte(realMfapiFixture), "2026-08-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points after since filter, got %d: %+v", len(points), points)
	}
}

func TestParseMfapiNavHistory_MalformedRowSkippedNotFatal(t *testing.T) {
	fixture := `{"data":[{"date":"27-08-2026","nav":"2297.09000"},{"date":"not-a-date","nav":"1.0"},{"date":"26-08-2026","nav":"N.A."}]}`
	points, err := ParseMfapiNavHistory([]byte(fixture), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("expected only the 1 well-formed row to survive, got %d: %+v", len(points), points)
	}
}

// realNiftyTRIFixture mirrors niftyindices.com's own confirmed doubly-
// encoded response shape - see history.go's FetchNiftyIndicesTRI doc
// comment for the source and how this was confirmed (a live capture
// documented by a third party, matching the "d" field being a
// JSON-ENCODED STRING, "DD MMM YYYY" dates with spaces, most-recent-
// first ordering).
const realNiftyTRIFixture = `{"d": "[{\"RequestNumber\":\"TRI63915064004640291500\",\"Index Name\":\"Nifty 500\",\"Date\":\"22 May 2026\",\"TotalReturnsIndex\":\"38121.36\",\"NTR_Value\":\"33450.12\"},{\"RequestNumber\":\"TRI63915064004640291501\",\"Index Name\":\"Nifty 500\",\"Date\":\"21 May 2026\",\"TotalReturnsIndex\":\"37980.50\",\"NTR_Value\":\"33320.44\"},{\"RequestNumber\":\"TRI63915064004640291502\",\"Index Name\":\"Nifty 500\",\"Date\":\"01 Jan 1999\",\"TotalReturnsIndex\":\"1000.00\",\"NTR_Value\":\"1000.00\"}]"}`

func TestParseNiftyIndicesTRI_ParsesRealFixtureAndSortsAscending(t *testing.T) {
	points, err := ParseNiftyIndicesTRI([]byte(realNiftyTRIFixture), "")
	if err != nil {
		t.Fatalf("failed to parse real niftyindices TRI fixture: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(points))
	}
	// Source is most-recent-first; parser must sort ascending like
	// every other price series in this codebase.
	if points[0].Date != "1999-01-01" || points[0].Nav != 1000.00 {
		t.Errorf("first point = %+v, want {1999-01-01 1000.00}", points[0])
	}
	if points[2].Date != "2026-05-22" || points[2].Nav != 38121.36 {
		t.Errorf("last point = %+v, want {2026-05-22 38121.36}", points[2])
	}
}

func TestParseNiftyIndicesTRI_SinceFiltersOlderRows(t *testing.T) {
	points, err := ParseNiftyIndicesTRI([]byte(realNiftyTRIFixture), "2026-01-01")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points after since filter, got %d: %+v", len(points), points)
	}
}

func TestParseNiftyIndicesTRI_EmptyArrayIsNotAnError(t *testing.T) {
	// A typo'd/unrecognized index name returns 200 OK with an empty
	// array, not an error - see FetchNiftyIndicesTRI's doc comment.
	// ParseNiftyIndicesTRI itself should reflect that faithfully (an
	// empty result, not a parse failure) - FetchNiftyIndicesTRI is
	// where an unbounded empty-result IS treated as an error.
	fixture := `{"d": "[]"}`
	points, err := ParseNiftyIndicesTRI([]byte(fixture), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("expected 0 points, got %d", len(points))
	}
}

// frankfurterTimeSeriesFixture follows the documented time-series shape
// (start_date/end_date, rates keyed by date then currency) consistent
// across every Frankfurter client library checked - not independently
// live-verified for the date-range form (only the single-date form was
// live-confirmed), flagged honestly in history.go.
const frankfurterTimeSeriesFixture = `{"amount":1,"base":"CAD","start_date":"2026-08-17","end_date":"2026-08-21","rates":{"2026-08-17":{"INR":61.23},"2026-08-18":{"INR":61.30},"2026-08-19":{"INR":61.28},"2026-08-20":{"INR":61.35},"2026-08-21":{"INR":61.41}}}`

func TestParseFrankfurterTimeSeries_ParsesFixture(t *testing.T) {
	rates, err := ParseFrankfurterTimeSeries([]byte(frankfurterTimeSeriesFixture), "CAD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rates) != 5 {
		t.Fatalf("expected 5 rates, got %d", len(rates))
	}
	// Sorted by date - first and last should be the boundary dates.
	if rates[0].Date != "2026-08-17" || rates[0].INRPerUnit != 61.23 {
		t.Errorf("first rate = %+v, want {2026-08-17 CAD 61.23}", rates[0])
	}
	if rates[4].Date != "2026-08-21" || rates[4].INRPerUnit != 61.41 {
		t.Errorf("last rate = %+v, want {2026-08-21 CAD 61.41}", rates[4])
	}
	for _, r := range rates {
		if r.Currency != "CAD" {
			t.Errorf("rate %+v has wrong currency, want CAD", r)
		}
	}
}

func TestParseFrankfurterTimeSeries_MissingINRSkipped(t *testing.T) {
	// A date entry with no INR key at all (shouldn't normally happen
	// given symbols=INR, but must not be silently fabricated into a
	// zero rate if it does).
	fixture := `{"amount":1,"base":"CAD","start_date":"2026-08-17","end_date":"2026-08-18","rates":{"2026-08-17":{"INR":61.23},"2026-08-18":{"USD":0.72}}}`
	rates, err := ParseFrankfurterTimeSeries([]byte(fixture), "CAD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("expected only the 1 date with an INR rate, got %d", len(rates))
	}
	if rates[0].Date != "2026-08-17" {
		t.Errorf("rates[0].Date = %q, want 2026-08-17", rates[0].Date)
	}
}

func TestParseYahooAdjClose_NormalShapeParses(t *testing.T) {
	fixture := `{"2026-08-25":{"^NSEI":24800.5},"2026-08-26":{"^NSEI":24850.75}}`
	points, err := ParseYahooAdjClose([]byte(fixture), "^NSEI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d: %+v", len(points), points)
	}
	if points[0].Date != "2026-08-25" || points[0].Price != 24800.5 {
		t.Errorf("points[0] = %+v, want {2026-08-25 24800.5}", points[0])
	}
	if points[1].Date != "2026-08-26" || points[1].Price != 24850.75 {
		t.Errorf("points[1] = %+v, want {2026-08-26 24850.75}", points[1])
	}
}

func TestParseYahooAdjClose_NullTickerOnOneDateIsSkippedNotError(t *testing.T) {
	// A real trading gap (market holiday) - documented as JSON null for
	// that ticker on that date, not an error.
	fixture := `{"2026-08-25":{"^NSEI":24800.5},"2026-08-26":{"^NSEI":null}}`
	points, err := ParseYahooAdjClose([]byte(fixture), "^NSEI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 || points[0].Date != "2026-08-25" {
		t.Errorf("expected only the 1 real point, got %+v", points)
	}
}

func TestParseYahooAdjClose_BareStringDateValueSkippedNotFatal(t *testing.T) {
	// This is the exact confirmed real failure: a narrow/incremental
	// date range returned at least one date whose ENTIRE value was a
	// bare string, not a {ticker: price} object - previously this made
	// json.Unmarshal fail for the WHOLE response ("cannot unmarshal
	// string into Go value of type map[string]interface{}"), reported
	// as a hard failure for 6 different benchmark tickers the moment
	// their fetch window narrowed to 1-2 days. One bad date must not
	// take down an otherwise-good response.
	fixture := `{"2026-08-25":{"^NSEI":24800.5},"2026-08-26":"Data not available"}`
	points, err := ParseYahooAdjClose([]byte(fixture), "^NSEI")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 1 || points[0].Date != "2026-08-25" {
		t.Errorf("expected the 1 good point to survive, got %+v", points)
	}
}

func TestParseYahooAdjClose_EveryDateUnparseableIsAnError(t *testing.T) {
	// Unlike a few stray bad dates mixed with good data, a response
	// where EVERY date is the unexpected shape suggests a genuinely
	// broken ticker/mapping - this must surface as an error, not
	// silently report "0 new, no error" (which would look identical to
	// "already up to date" and hide the problem indefinitely).
	fixture := `{"2026-08-25":"Data not available","2026-08-26":"Data not available"}`
	_, err := ParseYahooAdjClose([]byte(fixture), "^NSMIDCP")
	if err == nil {
		t.Fatal("expected an error when every date is unparseable, got nil")
	}
}

func TestParseYahooAdjClose_EmptyResponseIsNotAnError(t *testing.T) {
	// A genuinely empty response (e.g. incremental fetch, nothing new
	// published yet) is success with zero points, not an error - the
	// caller (UpdateHistoricalPrice/UpdateBenchmarkHistory) decides
	// whether zero points is fine based on whether history already
	// existed.
	points, err := ParseYahooAdjClose([]byte(`{}`), "^NSEI")
	if err != nil {
		t.Fatalf("unexpected error for empty response: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("expected 0 points, got %+v", points)
	}
}
