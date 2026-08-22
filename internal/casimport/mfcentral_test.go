package casimport

import "testing"

// These page texts are copied verbatim (per-page) from the user's real
// GetPlainText() output, confirmed to have genuine newline-separated
// fields rather than the run-together blob the earlier row-based
// extraction produced.

const cp1 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
PAN: APPPR4110Q
SABYASACHI ROY
4669, DRUMMOND DRIVEVANCOUVERVANCOUVER - 0, BRITISH CO, CANADAMobile: 17783256624Email: SABYASACH2@GMAIL.COM
The Consolidated Account Statement is brought to you as an investor friendly initiative byCAMS and KFintech, and list the transactions, balances and valuation of Mutual Funds in which youare holding investments. The consolidation has been carried out based on your PAN.If you find any folios missing in this consolidation, please check if your PAN is updated across all yourMutual Fund folios.
Page 1 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Allocation by Asset Class
83.78%
16.22%
EQUITY
FOF`

const cp2 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 2 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date`

const cp3 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 3 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
Nippon India Mutual Fund
FOLIO NO: 499388482035
NIPPON INDIA GROWTH MID CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION (Advisor: /DIRECT)  ISIN: INF204K01E54
KYC : OK
Opening Unit Balance: 0.000
01-JUL-2025
Purchase Trxn.Ref.No.pay_QnjoMAgGPZYGW0//Icici Bank Limited -036001076406//netbanking
24,998.75
5.409
4,621.60
5.409
08-JUL-2025
Purchase Trxn.Ref.No.pay_QqVpg1ZAdY7AL1//Icici Bank Limited -036001076406//netbanking
24,998.75
5.475
4,565.72
10.884
17-SEP-2025
Purchase Trxn.Ref.No.pay_RIbOhLXOHuJRon//Icici Bank Limited - 036001076406//
24,998.75
5.386
4,641.10
65.875`

const cp4 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 4 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
17-SEP-2025
netbanking
24,998.75
5.386
4,641.10
65.875
23-SEP-2025
Purchase Trxn.Ref.No.pay_RKzKFNNTqCoycJ//Icici Bank Limited -036001076406//netbanking
24,998.75
5.418
4,613.80
71.293
16-MAR-2026
Purchase
24,998.75
5.700
4,385.99
142.449`

const cp5 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 5 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
23-MAR-2026
Purchase Trxn.Ref.No.pay_SUakRGLq8qaDyb-NA-NETBANKING//Icici Bank Limited -036001076406/netbanking
24,998.75
5.930
4,215.88
148.379
Closing Unit Balance: 148.379
Nav as on 07-AUG-2026: INR 5,049.2657
Valuation on 09-Aug-2026 : INR 7,49,205.00`

const cp6 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 6 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
FOLIO NO: 499388482035
NIPPON INDIA INDEX FUND - NIFTY 50 PLAN - DIRECT GROWTH PLAN GROWTH OPTION (Advisor: /DIRECT)  ISIN: INF204K01H36
KYC : OK
Opening Unit Balance: 0.000
14-JAN-2025
Sys. Investment Trxn.Ref.No.pay_PjFcLpqzHGeDgZ//ICICI BANK LIMITED -036001076406/netbanking (1/28)
24,998.75
595.435
41.98
595.435`

const cp28 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 28 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
Transaction
Amount (INR)
Units
Price(INR)
Unit Balance
Date
16-APR-2026
Redemption Directly credited to your Bank account (ICICI BANK-DCB)
(4,918.00)
(26.834)
186.34
3,511.853
Closing Unit Balance: 3,511.853
Nav as on 07-AUG-2026: INR 208.2889
Valuation on 09-Aug-2026 : INR 7,31,480.00`

const cp29 = `Consolidated Account Statement
To Date : 
09-Aug-2026 
)
( From Date : 
01-Dec-2024
Page 29 of 30
MFCentralDetailCAS_v1.2_1913154667-122231510_01-Dec-2024_09-Aug-2026 _-09/08/2026 9:42:27am
SoA Holdings
Demat Holdings
No Folios Found`

func TestDetectFormat_RealCleanPage1(t *testing.T) {
	if got := DetectFormat(cp1); got != "MFCENTRAL" {
		t.Fatalf("expected MFCENTRAL, got %s", got)
	}
}

func TestParseMFCentral_CleanLines_PageBreakWrap(t *testing.T) {
	result := ParseMFCentral([]string{cp1, cp2, cp3, cp4, cp5})

	if len(result.ManualReview) != 0 {
		t.Fatalf("expected no manual review, got %v", result.ManualReview)
	}
	// 01-JUL, 08-JUL, 17-SEP (wrapped p3/p4, merges to 1), 23-SEP, 16-MAR, 23-MAR = 6
	if len(result.Staged) != 6 {
		t.Fatalf("expected 6 staged transactions, got %d", len(result.Staged))
	}

	wrapped := result.Staged[2].Txn // the merged 17-SEP-2025 row
	if wrapped.Date != "2025-09-17" {
		t.Errorf("date = %q, want 2025-09-17", wrapped.Date)
	}
	wantDesc := "Purchase Trxn.Ref.No.pay_RIbOhLXOHuJRon//Icici Bank Limited - 036001076406// netbanking"
	if wrapped.Description != wantDesc {
		t.Errorf("description = %q, want %q", wrapped.Description, wantDesc)
	}
	if wrapped.Amount != 24998.75 {
		t.Errorf("amount = %v, want 24998.75", wrapped.Amount)
	}
	if wrapped.Units == nil || *wrapped.Units != 5.386 {
		t.Errorf("units = %v, want 5.386", wrapped.Units)
	}
	if wrapped.AMC != "Nippon India Mutual Fund" {
		t.Errorf("AMC = %q, want %q", wrapped.AMC, "Nippon India Mutual Fund")
	}
	if wrapped.Scheme != "NIPPON INDIA GROWTH MID CAP FUND - DIRECT GROWTH PLAN GROWTH OPTION" {
		t.Errorf("scheme = %q", wrapped.Scheme)
	}
	if wrapped.ISIN != "INF204K01E54" {
		t.Errorf("isin = %q, want INF204K01E54", wrapped.ISIN)
	}
	if wrapped.Folio != "499388482035" {
		t.Errorf("folio = %q, want 499388482035", wrapped.Folio)
	}

	last := result.Staged[5].Txn
	if last.Balance == nil || *last.Balance != 148.379 {
		t.Errorf("closing txn balance = %v, want 148.379", last.Balance)
	}

	// SourcePage regression guard: the wrapped row's description spans
	// cp3/cp4 (pages 3 and 4), and the merge logic keeps the FIRST half's
	// page since that's where the transaction actually starts.
	if result.Staged[2].SourcePage != 3 {
		t.Errorf("wrapped row SourcePage = %d, want 3", result.Staged[2].SourcePage)
	}
	// The first transaction (01-JUL) is on cp3 — cp2's own page ends
	// exactly at the "Unit Balance"/"Date" header with nothing following
	// it, so cp2 contributes zero real content lines.
	if result.Staged[0].SourcePage != 3 {
		t.Errorf("first row SourcePage = %d, want 3", result.Staged[0].SourcePage)
	}
}

func TestParseMFCentral_CleanLines_NextSchemeInheritsAMC(t *testing.T) {
	result := ParseMFCentral([]string{cp1, cp2, cp3, cp4, cp5, cp6})

	var found bool
	for _, s := range result.Staged {
		if s.Txn.ISIN == "INF204K01H36" {
			found = true
			if s.Txn.AMC != "Nippon India Mutual Fund" {
				t.Errorf("AMC = %q, want inherited %q", s.Txn.AMC, "Nippon India Mutual Fund")
			}
			if s.Txn.Type != "PURCHASE_SIP" {
				t.Errorf("type = %q, want PURCHASE_SIP for a 'Sys. Investment' row", s.Txn.Type)
			}
		}
	}
	if !found {
		t.Fatal("expected a staged transaction for scheme ISIN INF204K01H36")
	}
}

func TestParseMFCentral_CleanLines_RedemptionParenthesisNegative(t *testing.T) {
	result := ParseMFCentral([]string{cp1, cp2, cp28})

	if len(result.ManualReview) != 0 {
		t.Fatalf("expected no manual review, got %v", result.ManualReview)
	}
	if len(result.Staged) != 1 {
		t.Fatalf("expected 1 staged transaction, got %d", len(result.Staged))
	}
	txn := result.Staged[0].Txn
	if txn.Type != "REDEMPTION" {
		t.Errorf("type = %q, want REDEMPTION", txn.Type)
	}
	if txn.Amount != -4918.00 {
		t.Errorf("amount = %v, want -4918.00", txn.Amount)
	}
	if txn.Units == nil || *txn.Units != -26.834 {
		t.Errorf("units = %v, want -26.834", txn.Units)
	}
	if txn.Balance == nil || *txn.Balance != 3511.853 {
		t.Errorf("balance = %v, want 3511.853", txn.Balance)
	}
}

func TestParseMFCentral_CleanLines_NoFoliosPageContributesNothing(t *testing.T) {
	result := ParseMFCentral([]string{cp29})
	if len(result.Staged) != 0 || len(result.ManualReview) != 0 {
		t.Fatalf("expected nothing from a 'No Folios Found' page, got staged=%d review=%d",
			len(result.Staged), len(result.ManualReview))
	}
}
