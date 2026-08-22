package finance

import (
	"math"
	"testing"
	"time"

	"ledger/internal/store"
)

func TestXIRR_KnownAnswer(t *testing.T) {
	// A single lump-sum investment doubling in exactly 1 year has an
	// XIRR of exactly 100%.
	t0 := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) // 2023 isn't a leap year: exactly 365 days
	rate, ok := XIRR([]CashFlow{
		{Date: t0, Amount: -10000},
		{Date: t1, Amount: 20000},
	})
	if !ok {
		t.Fatal("expected XIRR to converge")
	}
	if math.Abs(rate-1.0) > 0.001 {
		t.Errorf("rate = %v, want ~1.0 (100%%)", rate)
	}
}

func TestXIRR_SameSignFlowsFail(t *testing.T) {
	_, ok := XIRR([]CashFlow{
		{Date: time.Now(), Amount: -100},
		{Date: time.Now().AddDate(0, 1, 0), Amount: -200},
	})
	if ok {
		t.Error("expected no solution when all cash flows are the same sign")
	}
}

func TestComputeHoldings_BasicPurchaseAndRedemption(t *testing.T) {
	units1 := 100.0
	units2 := -40.0
	p := &store.Portfolio{
		Accounts: []store.Account{{ID: "acc1", Name: "Nippon India Mutual Fund"}},
		Assets:   []store.Asset{{ID: "ast1", AccountID: "acc1", Name: "Growth Fund", ISIN: "INF123"}},
		Transactions: []store.StoredTransaction{
			{AssetID: "ast1", Date: "2024-01-01", Amount: 1000, Units: &units1, Type: store.Purchase},
			{AssetID: "ast1", Date: "2024-06-01", Amount: -500, Units: &units2, Type: store.Redemption},
		},
		Prices: []store.PriceRecord{
			{AssetID: "ast1", Date: "2024-12-01", Price: 15.0},
		},
	}

	holdings := ComputeHoldings(p)
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	h := holdings[0]

	if h.UnitsHeld != 60 {
		t.Errorf("units held = %v, want 60", h.UnitsHeld)
	}
	if h.NetInvested != 500 {
		t.Errorf("net invested = %v, want 500", h.NetInvested)
	}
	wantValue := 60 * 15.0
	if h.CurrentValue != wantValue {
		t.Errorf("current value = %v, want %v", h.CurrentValue, wantValue)
	}
	if h.Gain != wantValue-500 {
		t.Errorf("gain = %v, want %v", h.Gain, wantValue-500)
	}
	if !h.HasXIRR {
		t.Error("expected XIRR to be computable")
	}
	if h.AccountName != "Nippon India Mutual Fund" {
		t.Errorf("account name = %q", h.AccountName)
	}
}

func TestComputeHoldings_NoPriceStillReportsUnitsAndInvested(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Accounts: []store.Account{{ID: "acc1", Name: "Some AMC"}},
		Assets:   []store.Asset{{ID: "ast1", AccountID: "acc1", Name: "Fund", ISIN: "INF999"}},
		Transactions: []store.StoredTransaction{
			{AssetID: "ast1", Date: "2024-01-01", Amount: 1000, Units: &units, Type: store.Purchase},
		},
	}
	holdings := ComputeHoldings(p)
	if len(holdings) != 1 {
		t.Fatalf("expected 1 holding, got %d", len(holdings))
	}
	h := holdings[0]
	if h.UnitsHeld != 10 || h.NetInvested != 1000 {
		t.Errorf("unexpected holding: %+v", h)
	}
	if h.HasPrice {
		t.Error("expected HasPrice false when no PriceRecord exists")
	}
}

func TestComputeHoldings_AssetWithNoTransactionsIsOmitted(t *testing.T) {
	p := &store.Portfolio{
		Assets: []store.Asset{{ID: "ast1", Name: "Untouched Fund"}},
	}
	holdings := ComputeHoldings(p)
	if len(holdings) != 0 {
		t.Errorf("expected 0 holdings, got %d", len(holdings))
	}
}

func TestPortfolioTotals(t *testing.T) {
	holdings := []Holding{
		{HasPrice: true, NetInvested: 100, CurrentValue: 150},
		{HasPrice: true, NetInvested: 200, CurrentValue: 180},
		{HasPrice: false, NetInvested: 50, CurrentValue: 0}, // excluded: no price
	}
	invested, value, any := PortfolioTotals(holdings)
	if !any {
		t.Fatal("expected anyPriced true")
	}
	if invested != 300 {
		t.Errorf("invested = %v, want 300", invested)
	}
	if value != 330 {
		t.Errorf("value = %v, want 330", value)
	}
}

func TestComputeHoldings_MemberAttribution(t *testing.T) {
	units := 10.0
	p := &store.Portfolio{
		Members: []store.Member{{ID: "mem1", Name: "Saby"}, {ID: "mem2", Name: "Mother"}},
		Accounts: []store.Account{
			{ID: "acc1", MemberID: "mem1", Name: "Saby's AMC"},
			{ID: "acc2", MemberID: "mem2", Name: "Mother's AMC"},
		},
		Assets: []store.Asset{
			{ID: "ast1", AccountID: "acc1", Name: "Saby Fund"},
			{ID: "ast2", AccountID: "acc2", Name: "Mother Fund"},
		},
		Transactions: []store.StoredTransaction{
			{AssetID: "ast1", Date: "2024-01-01", Amount: 1000, Units: &units},
			{AssetID: "ast2", Date: "2024-01-01", Amount: 2000, Units: &units},
		},
	}
	holdings := ComputeHoldings(p)
	byID := make(map[string]Holding)
	for _, h := range holdings {
		byID[h.AssetID] = h
	}
	if byID["ast1"].MemberName != "Saby" {
		t.Errorf("ast1 member = %q, want Saby", byID["ast1"].MemberName)
	}
	if byID["ast2"].MemberName != "Mother" {
		t.Errorf("ast2 member = %q, want Mother", byID["ast2"].MemberName)
	}

	filtered := FilterHoldingsByMember(holdings, "mem1")
	if len(filtered) != 1 || filtered[0].AssetID != "ast1" {
		t.Errorf("expected only ast1 after filtering by mem1, got %+v", filtered)
	}

	all := FilterHoldingsByMember(holdings, "")
	if len(all) != 2 {
		t.Errorf("expected both holdings with empty memberID filter, got %d", len(all))
	}
}

func TestPortfolioXIRR_ScopesToPassedHoldings(t *testing.T) {
	unitsA, unitsB := 10.0, 10.0
	p := &store.Portfolio{
		Transactions: []store.StoredTransaction{
			{AssetID: "ast1", Date: "2023-01-01", Amount: 1000, Units: &unitsA},
			{AssetID: "ast2", Date: "2023-01-01", Amount: 1000, Units: &unitsB},
		},
	}
	// Only include ast1 in the holdings passed in - ast2's cash flow
	// should not affect the result.
	holdings := []Holding{
		{AssetID: "ast1", HasPrice: true, UnitsHeld: 10, CurrentValue: 2000},
	}
	rate, ok := PortfolioXIRR(p, holdings)
	if !ok {
		t.Fatal("expected XIRR to converge")
	}
	if rate <= 0 {
		t.Errorf("expected a positive return (doubled in under 3 years), got %v", rate)
	}
}
