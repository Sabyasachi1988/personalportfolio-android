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
