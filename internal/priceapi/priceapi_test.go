package priceapi

import "testing"

// Real lines from https://www.amfiindia.com/spages/NAVAll.txt fetched live
// on 2026-08-20/21 while building this fix.
const realAmfiSample = `Scheme Code;ISIN Div Payout/ ISIN Growth;ISIN Div Reinvestment;Scheme Name;Plan;Option;Net Asset Value;Date
 
Open Ended Schemes(Debt Scheme - Banking and PSU Fund)
 
Aditya Birla Sun Life Mutual Fund
 
119551;INF209KA12Z1;INF209KA13Z9;Aditya Birla Sun Life Banking & PSU Debt Fund;Direct Plan;IDCW-Re-investment;106.9996;20-Aug-2026
108273;INF209K01LV0;-;Aditya Birla Sun Life Banking & PSU Debt Fund;Regular Plan;GROWTH;387.8554;20-Aug-2026
 
Axis Mutual Fund
 
120437;-;INF846K01CU0;Axis Banking & PSU Debt Fund;Direct Plan;Daily IDCW;1036.5080;20-Aug-2026
 
ICICI Prudential Mutual Fund
 
130897;INF109KA1B57;-;ICICI Prudential Banking & PSU Debt Fund;;;15.8889;24-Apr-2020
 
Nippon India Mutual Fund
 
118814;INF204K01C15;-;Nippon India Corporate Bond Fund;Direct Plan;Growth Option;67.1829;20-Aug-2026
`

// The pre-2026 6-field format (no Plan/Option columns), reconstructed to
// match AMFI's documented older shape, must still parse correctly.
const legacySixFieldSample = `Scheme Code;ISIN Div Payout/ ISIN Growth;ISIN Div Reinvestment;Scheme Name;Net Asset Value;Date
 
Open Ended Schemes(Debt Scheme - Banking and PSU Fund)
 
Aditya Birla Sun Life Mutual Fund
 
119551;INF209KA12Z1;INF209KA13Z9;Aditya Birla Sun Life Banking & PSU Debt Fund - DIRECT - IDCW;154.6417;21-May-2021
`

func TestParseAmfiText_RealCurrentFormat(t *testing.T) {
	records, skipped, err := ParseAmfiText(realAmfiSample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped data-like lines, got %d", skipped)
	}
	if len(records) != 5 {
		t.Fatalf("expected 5 records, got %d", len(records))
	}

	first := records[0]
	if first.SchemeCode != "119551" {
		t.Errorf("scheme code = %q, want 119551", first.SchemeCode)
	}
	if first.ISINPayout != "INF209KA12Z1" {
		t.Errorf("ISIN payout = %q", first.ISINPayout)
	}
	if first.SchemeName != "Aditya Birla Sun Life Banking & PSU Debt Fund" {
		t.Errorf("scheme name = %q", first.SchemeName)
	}
	if first.Plan != "Direct Plan" {
		t.Errorf("plan = %q, want %q", first.Plan, "Direct Plan")
	}
	if first.Option != "IDCW-Re-investment" {
		t.Errorf("option = %q", first.Option)
	}
	if first.NAV != 106.9996 {
		t.Errorf("nav = %v, want 106.9996", first.NAV)
	}
	if first.Date != "20-Aug-2026" {
		t.Errorf("date = %q", first.Date)
	}

	// Dash-for-missing-ISIN case.
	daily := records[2]
	if daily.SchemeCode != "120437" {
		t.Fatalf("expected 3rd record to be scheme 120437, got %s", daily.SchemeCode)
	}
	if daily.ISINPayout != "" {
		t.Errorf("ISIN payout for '-' should be empty, got %q", daily.ISINPayout)
	}
	if daily.ISINReinvest != "INF846K01CU0" {
		t.Errorf("ISIN reinvest = %q", daily.ISINReinvest)
	}

	// Empty Plan/Option case (line has ";;;" between name and NAV).
	empty := records[3]
	if empty.SchemeCode != "130897" {
		t.Fatalf("expected 4th record to be scheme 130897, got %s", empty.SchemeCode)
	}
	if empty.Plan != "" || empty.Option != "" {
		t.Errorf("expected empty plan/option, got plan=%q option=%q", empty.Plan, empty.Option)
	}
	if empty.NAV != 15.8889 {
		t.Errorf("nav = %v, want 15.8889", empty.NAV)
	}
}

func TestParseAmfiText_LegacySixFieldFormatStillWorks(t *testing.T) {
	records, skipped, err := ParseAmfiText(legacySixFieldSample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}
	r := records[0]
	if r.SchemeName != "Aditya Birla Sun Life Banking & PSU Debt Fund - DIRECT - IDCW" {
		t.Errorf("scheme name = %q", r.SchemeName)
	}
	if r.Plan != "" || r.Option != "" {
		t.Errorf("legacy format has no plan/option columns, got plan=%q option=%q", r.Plan, r.Option)
	}
	if r.NAV != 154.6417 {
		t.Errorf("nav = %v, want 154.6417", r.NAV)
	}
	if r.Date != "21-May-2021" {
		t.Errorf("date = %q", r.Date)
	}
}

func TestParseAmfiText_SectionAndAMCHeadersSkipped(t *testing.T) {
	records, skipped, err := ParseAmfiText(realAmfiSample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = records
	// Section headers like "Open Ended Schemes(...)" and AMC name lines
	// like "Aditya Birla Sun Life Mutual Fund" have no ';' at all, so they
	// never reach the "len(fields) < 6" skip path as a *counted* skip -
	// they're filtered out before that check even applies. This test just
	// guards against them being misparsed into bogus records.
	for _, r := range records {
		if r.SchemeName == "" {
			t.Errorf("found a record with empty scheme name - a header line leaked through: %+v", r)
		}
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped data-like lines in the real sample, got %d", skipped)
	}
}

func TestParseAmfiText_CategoryHeadersCaptured(t *testing.T) {
	// Real section headers, confirmed live: debt-fund one fetched
	// directly, equity one confirmed via a secondary source that itself
	// quotes the live file verbatim.
	sample := `Open Ended Schemes(Debt Scheme - Banking and PSU Fund)
 
Aditya Birla Sun Life Mutual Fund
 
119551;INF209KA12Z1;INF209KA13Z9;Aditya Birla Sun Life Banking & PSU Debt Fund;Direct Plan;IDCW-Re-investment;106.9996;20-Aug-2026
 
Open Ended Schemes(Equity Scheme - Large Cap Fund)
 
Some AMC
 
200001;INF000000001;-;Some Large Cap Fund;Direct Plan;Growth;50.0000;20-Aug-2026
`
	records, skipped, err := ParseAmfiText(sample)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped, got %d", skipped)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(records))
	}
	if records[0].AssetClass != "Debt" {
		t.Errorf("first record asset class = %q, want Debt", records[0].AssetClass)
	}
	if records[0].Category != "Open Ended Schemes(Debt Scheme - Banking and PSU Fund)" {
		t.Errorf("first record category = %q", records[0].Category)
	}
	if records[1].AssetClass != "Equity" {
		t.Errorf("second record asset class = %q, want Equity", records[1].AssetClass)
	}
}

func TestParseAmfiText_GenuinelyBrokenLineIsCounted(t *testing.T) {
	// A line with enough semicolons to look like data, but a non-numeric
	// NAV field, should be counted as skipped rather than silently eaten
	// or silently accepted - this is the exact failure mode of the
	// original bug (reading "Direct Plan" text where a NAV number was
	// expected), reproduced deliberately here.
	broken := `119551;INF209KA12Z1;INF209KA13Z9;Some Fund;Direct Plan;IDCW;NOT_A_NUMBER;20-Aug-2026`
	records, skipped, err := ParseAmfiText(broken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records from a broken NAV field, got %d", len(records))
	}
	if skipped != 1 {
		t.Errorf("expected 1 skipped data-like line, got %d", skipped)
	}
}
