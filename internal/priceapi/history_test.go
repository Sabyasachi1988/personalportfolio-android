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
