package finance

import (
	"testing"
	"time"

	"ledger/internal/store"
)

func units(v float64) *float64 { return &v }

func TestDailyDatesInRange_BasicRange(t *testing.T) {
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	dates := DailyDatesInRange(start, end)
	want := []string{"2026-08-01", "2026-08-02", "2026-08-03", "2026-08-04", "2026-08-05"}
	if len(dates) != len(want) {
		t.Fatalf("got %v, want %v", dates, want)
	}
	for i := range want {
		if dates[i] != want[i] {
			t.Errorf("dates[%d] = %q, want %q", i, dates[i], want[i])
		}
	}
}

func TestDailyDatesInRange_StartAfterEndReturnsNil(t *testing.T) {
	start := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	if got := DailyDatesInRange(start, end); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestComputeProgressionDailyRange_MatchesPlainComputationForSameDates(t *testing.T) {
	p := buildMixedPortfolio()
	start := time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeProgressionDailyRange(p, "", AxisWholePortfolio, start, end)
	// 2024-01-16 through 2024-01-22 inclusive = 7 daily points.
	if len(points) != 7 {
		t.Fatalf("expected 7 daily points, got %d", len(points))
	}
	if points[0].Date != "2024-01-16" {
		t.Errorf("first point date = %q, want 2024-01-16", points[0].Date)
	}
	last := points[len(points)-1]
	if last.Date != "2024-01-22" {
		t.Errorf("last point date = %q, want 2024-01-22", last.Date)
	}
	// Same expected totals as TestComputeProgression_WholePortfolio_CombinesBothCurrenciesInINR
	// for its one weekly checkpoint on this same date - confirms the
	// shared computeProgressionSeries core produces identical results
	// for the same date regardless of which calendar granularity
	// requested it. Invested: 10000 INR + 1000 CAD * 60 INR/CAD = 70000.
	if last.Invested != 70000 {
		t.Errorf("last point Invested = %v, want 70000", last.Invested)
	}
}

func TestComputeAssetProgressionDailyRange_ScopedToOneAsset(t *testing.T) {
	p := buildMixedPortfolio()
	start := time.Date(2024, 1, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeAssetProgressionDailyRange(p, "a-in", start, end)
	if len(points) != 3 {
		t.Fatalf("expected 3 daily points, got %d", len(points))
	}
	last := points[len(points)-1]
	if last.Invested != 10000 {
		t.Errorf("last point Invested = %v, want 10000 (only asset a-in, not the Canadian holding)", last.Invested)
	}
}

func TestComputeGroupProgression_SumsAllSameLabeledAssets(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Zerodha", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "a-nippon", AccountID: "acc-in", Name: "NIFTYBEES", GroupLabel: "Nifty 50"},
			{ID: "a-navi", AccountID: "acc-in", Name: "Navi Nifty 50 Fund", GroupLabel: "Nifty 50"},
			{ID: "a-other", AccountID: "acc-in", Name: "Some Other Fund", GroupLabel: ""}, // ungrouped - must be excluded
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a-nippon", Date: "2024-01-16", Type: store.Purchase, Amount: 10000, Units: units(100)},
			{ID: "t2", AccountID: "acc-in", AssetID: "a-navi", Date: "2024-01-16", Type: store.Purchase, Amount: 5000, Units: units(50)},
			{ID: "t3", AccountID: "acc-in", AssetID: "a-other", Date: "2024-01-16", Type: store.Purchase, Amount: 99999, Units: units(999)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a-nippon", Date: "2024-01-22", Price: 110},
			{AssetID: "a-navi", Date: "2024-01-22", Price: 105},
			{AssetID: "a-other", Date: "2024-01-22", Price: 1},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeGroupProgression(p, "", "Nifty 50", today, nil)
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	pt := points[0]
	if pt.Invested != 15000 {
		t.Errorf("Invested = %v, want 15000 (10000+5000, the ungrouped fund's 99999 must be excluded)", pt.Invested)
	}
	wantValue := 100.0*110 + 50.0*105 // 11000 + 5250
	if pt.Value != wantValue {
		t.Errorf("Value = %v, want %v", pt.Value, wantValue)
	}
}

func TestComputeGroupProgression_DifferentMembersNeverSummed(t *testing.T) {
	p := &store.Portfolio{
		Members: []store.Member{{ID: "m1", Name: "Saby"}, {ID: "m2", Name: "Mother"}},
		Accounts: []store.Account{
			{ID: "acc1", MemberID: "m1", Name: "Saby's", Currency: "INR"},
			{ID: "acc2", MemberID: "m2", Name: "Mother's", Currency: "INR"},
		},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc1", Name: "Fund A", GroupLabel: "Nifty 50"},
			{ID: "a2", AccountID: "acc2", Name: "Fund B", GroupLabel: "Nifty 50"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc1", AssetID: "a1", Date: "2024-01-16", Type: store.Purchase, Amount: 10000, Units: units(100)},
			{ID: "t2", AccountID: "acc2", AssetID: "a2", Date: "2024-01-16", Type: store.Purchase, Amount: 20000, Units: units(200)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-01-22", Price: 110},
			{AssetID: "a2", Date: "2024-01-22", Price: 110},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeGroupProgression(p, "m1", "Nifty 50", today, nil)
	if len(points) != 1 || points[0].Invested != 10000 {
		t.Errorf("member-scoped group progression = %+v, want Invested=10000 (only Saby's asset, not Mother's)", points)
	}
}

func TestWeeklyDates_BasicRange(t *testing.T) {
	p := &store.Portfolio{
		Transactions: []store.StoredTransaction{
			{Date: "2024-01-10"}, // a Wednesday
		},
	}
	today := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC) // a Wednesday

	dates := WeeklyDates(p, today)

	// First Monday on/after 2024-01-10 is 2024-01-15.
	// Mondays: 01-15, 01-22, 01-29, then today (01-31) appended since it's not a Monday.
	want := []string{"2024-01-15", "2024-01-22", "2024-01-29", "2024-01-31"}
	if len(dates) != len(want) {
		t.Fatalf("got %v, want %v", dates, want)
	}
	for i := range want {
		if dates[i] != want[i] {
			t.Errorf("dates[%d] = %s, want %s", i, dates[i], want[i])
		}
	}
}

func TestWeeklyDates_TodayIsMonday_NotDuplicated(t *testing.T) {
	p := &store.Portfolio{
		Transactions: []store.StoredTransaction{{Date: "2024-01-01"}},
	}
	today := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC) // a Monday

	dates := WeeklyDates(p, today)
	last := dates[len(dates)-1]
	if last != "2024-01-15" {
		t.Errorf("last date = %s, want 2024-01-15", last)
	}
	// Must not appear twice.
	count := 0
	for _, d := range dates {
		if d == "2024-01-15" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("2024-01-15 appears %d times, want 1", count)
	}
}

func TestWeeklyDates_NoTransactions_ReturnsNil(t *testing.T) {
	p := &store.Portfolio{}
	dates := WeeklyDates(p, time.Now())
	if len(dates) != 0 {
		t.Errorf("expected no dates for empty portfolio, got %v", dates)
	}
}

func TestClassifyForeignAsset(t *testing.T) {
	cases := map[string]string{
		"iShares Gold ETF":        "Commodity",
		"SPDR S&P 500 ETF Trust":  "Equity",
		"Vanguard Total Bond ETF": "Debt",
		"Apple Inc":               "Equity",
	}
	for name, want := range cases {
		if got := classifyForeignAsset(name); got != want {
			t.Errorf("classifyForeignAsset(%q) = %s, want %s", name, got, want)
		}
	}
}

// buildMixedPortfolio sets up one Indian equity fund (large-cap heuristic
// name, INR account) and one Canadian equity ETF (CAD account), each with
// a single purchase and price history, plus FX history for CAD.
func buildMixedPortfolio() *store.Portfolio {
	p := &store.Portfolio{
		Members: []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{
			{ID: "acc-in", MemberID: "m1", Name: "Nippon India", Currency: "INR"},
			{ID: "acc-ca", MemberID: "m1", Name: "Questrade", Currency: "CAD"},
		},
		Assets: []store.Asset{
			{ID: "a-in", AccountID: "acc-in", Name: "Nippon India Large Cap Fund", ISIN: "INF000IN001", Type: "MutualFund"},
			{ID: "a-ca", AccountID: "acc-ca", Name: "Vanguard S&P 500 ETF", Type: "ETF", Symbol: "VFV.TO"},
		},
		Transactions: []store.StoredTransaction{
			// Tuesday, not Monday: keeps the weekly checkpoint calendar to a
			// single point (2024-01-22) for these fixture-based tests, since
			// WeeklyDates starts from the Monday on/after the earliest txn.
			{ID: "t1", AccountID: "acc-in", AssetID: "a-in", Date: "2024-01-16", Type: store.Purchase, Amount: 10000, Units: units(100)},
			{ID: "t2", AccountID: "acc-ca", AssetID: "a-ca", Date: "2024-01-16", Type: store.Purchase, Amount: 1000, Units: units(10)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a-in", Date: "2024-01-15", Price: 100},
			{AssetID: "a-in", Date: "2024-01-22", Price: 110},
			{AssetID: "a-ca", Date: "2024-01-15", Price: 100},
			{AssetID: "a-ca", Date: "2024-01-22", Price: 105},
		},
		FXRates: []store.FXRate{
			{Currency: "CAD", Date: "2024-01-15", INRPerUnit: 60.0},
			{Currency: "CAD", Date: "2024-01-22", INRPerUnit: 61.0},
		},
	}
	return p
}

func TestComputeProgression_WholePortfolio_CombinesBothCurrenciesInINR(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC) // a Monday

	points := ComputeProgression(p, "", AxisWholePortfolio, today, nil)
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d: %v", len(points), points)
	}
	pt := points[0]

	// Invested: 10000 INR (Indian) + 1000 CAD * 60 INR/CAD (historical rate on flow date) = 10000 + 60000 = 70000
	wantInvested := 70000.0
	if pt.Invested != wantInvested {
		t.Errorf("Invested = %v, want %v", pt.Invested, wantInvested)
	}

	// Value as of 2024-01-22: Indian 100 units * 110 = 11000 INR.
	// Canadian: 10 units * 105 CAD = 1050 CAD * 61 INR/CAD (rate as of THIS date, not the flow date) = 64050 INR.
	wantValue := 11000.0 + 64050.0
	if pt.Value != wantValue {
		t.Errorf("Value = %v, want %v", pt.Value, wantValue)
	}
}

// TestComputeProgression_IndianEquityAxis_IncludesBareTickerETF
// reproduces the exact bug reported: an INR-account NSE-listed ETF whose
// Name is a bare exchange ticker (e.g. "NIFTYBEES", as imported from a
// broker CSV - see bridge.inferInitialSymbol) was silently excluded
// from every equity-scoped Progression axis (Indian/International/
// Combined Equity), because GuessMarketCapSegment's patterns required a
// space ("nifty bees") that a real ticker symbol never has - it fell
// through to "Unclassified", which EffectiveAssetClass does not count
// as Equity. AxisWholePortfolio worked (it bypasses the equity check
// entirely), which is exactly what was reported: "whole portfolio
// shows it, Indian equity makes it disappear."
func TestComputeProgression_IndianEquityAxis_IncludesBareTickerETF(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Zerodha", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "a-etf", AccountID: "acc-in", Name: "NIFTYBEES", ISIN: "INF204KB14I2", Type: "Stock", Symbol: "NIFTYBEES.NS"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a-etf", Date: "2024-01-16", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a-etf", Date: "2024-01-15", Price: 100},
			{AssetID: "a-etf", Date: "2024-01-22", Price: 110},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeProgression(p, "", AxisIndianEquity, today, nil)
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	if points[0].Value != 11000 {
		t.Errorf("Value = %v, want 11000 - the ETF was excluded from Indian Equity (this is the reported bug)", points[0].Value)
	}
	if points[0].Invested != 10000 {
		t.Errorf("Invested = %v, want 10000", points[0].Invested)
	}
}

func TestComputeProgression_IndianEquityAxis_ExcludesForeignHolding(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeProgression(p, "", AxisIndianEquity, today, nil)
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	pt := points[0]
	if pt.Invested != 10000 {
		t.Errorf("Invested = %v, want 10000 (Canadian holding must be excluded)", pt.Invested)
	}
	if pt.Value != 11000 {
		t.Errorf("Value = %v, want 11000", pt.Value)
	}
}

func TestComputeProgression_InternationalEquityAxis_OnlyForeignHolding(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeProgression(p, "", AxisInternationalEquity, today, nil)
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	pt := points[0]
	wantInvested := 1000.0 * 60.0 // CAD amount converted at the flow's own historical FX rate
	if pt.Invested != wantInvested {
		t.Errorf("Invested = %v, want %v", pt.Invested, wantInvested)
	}
	wantValue := 10.0 * 105.0 * 61.0 // units * price(as-of) * FX(as-of THIS date)
	if pt.Value != wantValue {
		t.Errorf("Value = %v, want %v", pt.Value, wantValue)
	}
	if !pt.HasINRPerCAD || pt.INRPerCAD != 61.0 {
		t.Errorf("INRPerCAD = %v (has=%v), want 61.0", pt.INRPerCAD, pt.HasINRPerCAD)
	}
}

func TestComputeProgression_CombinedEquityAxis_SumsBoth(t *testing.T) {
	p := buildMixedPortfolio()
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	whole := ComputeProgression(p, "", AxisWholePortfolio, today, nil)
	combined := ComputeProgression(p, "", AxisCombinedEquity, today, nil)
	if len(whole) != 1 || len(combined) != 1 {
		t.Fatalf("expected 1 point each")
	}
	// In this fixture every holding is equity, so Combined should equal Whole exactly.
	if combined[0].Invested != whole[0].Invested || combined[0].Value != whole[0].Value {
		t.Errorf("CombinedEquity = %+v, want to match WholePortfolio = %+v", combined[0], whole[0])
	}
}

func TestComputeProgression_EquityOriginSplit_PartialInternational(t *testing.T) {
	p := buildMixedPortfolio()
	// The Indian fund is actually a fund-of-fund tracking a foreign index:
	// 30% Indian, 70% International, per a real entered composition.
	p.EquityOriginCompositions = []store.EquityOriginComposition{
		{AssetID: "a-in", Indian: 30, International: 70, AsOf: "2026-01-01", Source: "test"},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	indian := ComputeProgression(p, "", AxisIndianEquity, today, nil)
	intl := ComputeProgression(p, "", AxisInternationalEquity, today, nil)

	// Indian axis should now carry only 30% of the Indian fund's value/invested.
	wantIndianInvested := round2(10000 * 0.30)
	if indian[0].Invested != wantIndianInvested {
		t.Errorf("Indian Invested = %v, want %v", indian[0].Invested, wantIndianInvested)
	}

	// International axis: 70% of the Indian FoF's INR value/invested, PLUS the
	// full Canadian ETF (which always counts 100% International).
	wantIntlInvestedFromFoF := 10000 * 0.70
	wantIntlInvestedFromCAD := 1000.0 * 60.0
	wantIntlInvested := round2(wantIntlInvestedFromFoF + wantIntlInvestedFromCAD)
	if intl[0].Invested != wantIntlInvested {
		t.Errorf("International Invested = %v, want %v", intl[0].Invested, wantIntlInvested)
	}
}

func TestComputeProgression_MissingFXHistory_ExcludesRatherThanGuesses(t *testing.T) {
	p := buildMixedPortfolio()
	p.FXRates = nil // no FX history fetched at all
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeProgression(p, "", AxisWholePortfolio, today, nil)
	pt := points[0]
	// Only the Indian leg should count; the Canadian leg is silently excluded
	// for lack of an FX rate, not guessed at some default/zero rate.
	if pt.Invested != 10000 {
		t.Errorf("Invested = %v, want 10000 (Canadian leg excluded, no FX history)", pt.Invested)
	}
	if pt.Value != 11000 {
		t.Errorf("Value = %v, want 11000", pt.Value)
	}
	if pt.HasINRPerCAD {
		t.Errorf("HasINRPerCAD = true, want false (no FX history at all)")
	}
}

func TestComputeProgression_MemberScoping(t *testing.T) {
	p := buildMixedPortfolio()
	p.Members = append(p.Members, store.Member{ID: "m2", Name: "Mother"})
	p.Accounts[1].MemberID = "m2" // Canadian account belongs to a different member
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeProgression(p, "m1", AxisWholePortfolio, today, nil)
	if points[0].Invested != 10000 {
		t.Errorf("Invested = %v, want 10000 (m2's Canadian holding must be excluded)", points[0].Invested)
	}
}
