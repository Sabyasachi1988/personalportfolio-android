package priceapi

import "testing"

// realPPFASFootnote mirrors PPFAS's own May 2026 factsheet text, live-
// fetched and confirmed - note the NUMBER comes AFTER the parenthetical
// here, a real PDF-table-extraction ordering quirk, not a typo.
const realPPFASFootnote = `Risk free rate assumed to be · (FBIL Overnight MIBOR as on May 31, 2026) 5.52% Beta · 0.60 · 9.90% 0.89 · 41.55% 17.17% Standard Deviation · Sharpe Ratio · Portfolio Turnover`

// realKotakFootnote mirrors Kotak's own factsheet text, live-fetched
// and confirmed - here the number comes BEFORE the parenthetical, the
// opposite order from PPFAS's.
const realKotakFootnote = `## Risk rate assumed to be 6.40% (FBIL Overnight MIBOR rate as on 31st May 2023).**Total Expense Ratio includes applicable B30 fee and GST.`

// realNipponFootnote mirrors Nippon India's own factsheet text (a
// shorter fragment than the other two, as actually captured).
const realNipponFootnote = `Overnight MIBOR as on 28/02/2025).`

func TestParseRiskFreeRateFootnote_PPFASOrdering(t *testing.T) {
	rate, asOf, found := ParseRiskFreeRateFootnote(realPPFASFootnote)
	if !found {
		t.Fatalf("expected to find a rate in the real PPFAS footnote text, didn't")
	}
	if rate != 5.52 {
		t.Errorf("rate = %v, want 5.52", rate)
	}
	if asOf != "May 31, 2026" {
		t.Errorf("asOf = %q, want %q", asOf, "May 31, 2026")
	}
}

func TestParseRiskFreeRateFootnote_KotakOrdering(t *testing.T) {
	rate, asOf, found := ParseRiskFreeRateFootnote(realKotakFootnote)
	if !found {
		t.Fatalf("expected to find a rate in the real Kotak footnote text, didn't")
	}
	if rate != 6.40 {
		t.Errorf("rate = %v, want 6.40", rate)
	}
	if asOf != "31st May 2023" {
		t.Errorf("asOf = %q, want %q", asOf, "31st May 2023")
	}
}

func TestParseRiskFreeRateFootnote_ImplausibleValueRejected(t *testing.T) {
	// A genuinely mis-extracted stray number (e.g. a Beta or Sharpe
	// ratio caught by a too-loose regex) should be rejected by the
	// plausibility bound, not accepted as if it were a real rate.
	text := `Some unrelated text (FBIL Overnight MIBOR as on 1 Jan 2026) 88.00% more text`
	_, _, found := ParseRiskFreeRateFootnote(text)
	if found {
		t.Errorf("expected an implausible 88%% rate to be rejected, but it was accepted")
	}
}

func TestParseRiskFreeRateFootnote_NoMIBORMentionReturnsNotFound(t *testing.T) {
	_, _, found := ParseRiskFreeRateFootnote("This document has no risk-free-rate footnote in it at all.")
	if found {
		t.Errorf("expected not found, got found=true")
	}
}
