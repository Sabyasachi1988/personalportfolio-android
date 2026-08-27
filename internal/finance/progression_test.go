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

func TestComputeTagProgression_SumsEveryAssetCarryingTheTag(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "a-nippon", AccountID: "acc-in", Name: "Nippon India Growth Mid Cap Fund", Tags: []string{"Mid Cap", "Growth"}},
			{ID: "a-hdfc", AccountID: "acc-in", Name: "HDFC Mid Cap Opportunities Fund", Tags: []string{"Mid Cap"}},
			{ID: "a-other", AccountID: "acc-in", Name: "Some Large Cap Fund", Tags: []string{"Large Cap"}}, // no "Mid Cap" tag - must be excluded
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a-nippon", Date: "2024-01-16", Type: store.Purchase, Amount: 10000, Units: units(100)},
			{ID: "t2", AccountID: "acc-in", AssetID: "a-hdfc", Date: "2024-01-16", Type: store.Purchase, Amount: 5000, Units: units(50)},
			{ID: "t3", AccountID: "acc-in", AssetID: "a-other", Date: "2024-01-16", Type: store.Purchase, Amount: 99999, Units: units(999)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a-nippon", Date: "2024-01-22", Price: 110},
			{AssetID: "a-hdfc", Date: "2024-01-22", Price: 105},
			{AssetID: "a-other", Date: "2024-01-22", Price: 1},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	points := ComputeTagProgression(p, "", "Mid Cap", today, nil)
	if len(points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(points))
	}
	pt := points[0]
	if pt.Invested != 15000 {
		t.Errorf("Invested = %v, want 15000 (10000+5000 - the two Mid Cap-tagged assets, Large Cap's 99999 excluded)", pt.Invested)
	}
	wantValue := 100.0*110 + 50.0*105 // 11000 + 5250
	if pt.Value != wantValue {
		t.Errorf("Value = %v, want %v", pt.Value, wantValue)
	}
}

func TestComputeTagProgression_AssetWithMultipleTagsContributesToEach(t *testing.T) {
	// Unlike GroupLabel (exclusive), an asset carrying several tags must
	// contribute FULLY to each tag's own progression line independently
	// - see ComputeTagProgression's doc comment on why this is correct,
	// not a double-count.
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc-in", Name: "Nippon India Growth Mid Cap Fund", Tags: []string{"Mid Cap", "Growth"}},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2024-01-16", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-01-22", Price: 110},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	midCapPoints := ComputeTagProgression(p, "", "Mid Cap", today, nil)
	growthPoints := ComputeTagProgression(p, "", "Growth", today, nil)
	if len(midCapPoints) != 1 || midCapPoints[0].Invested != 10000 {
		t.Errorf("Mid Cap progression = %+v, want Invested=10000", midCapPoints)
	}
	if len(growthPoints) != 1 || growthPoints[0].Invested != 10000 {
		t.Errorf("Growth progression = %+v, want Invested=10000 (same asset, full amount, independently)", growthPoints)
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

func TestComputePeriodGains_ExcludesContributionsMadeDuringTheWindow(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "a1", AccountID: "acc-in", Name: "Nippon India Growth Mid Cap Fund"},
		},
		Transactions: []store.StoredTransaction{
			// Bought long before any window below, so windows that don't
			// reach back this far still have a well-defined starting Value.
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2023-01-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
			// A fresh top-up 3 days ago, INSIDE the Year window but not
			// the Day window - this is the contribution that must NOT
			// show up as "gain".
			{ID: "t2", AccountID: "acc-in", AssetID: "a1", Date: "2024-01-19", Type: store.Purchase, Amount: 5000, Units: units(40)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2023-01-01", Price: 100},
			{AssetID: "a1", Date: "2024-01-18", Price: 110}, // 1 day before "today" below
			{AssetID: "a1", Date: "2024-01-19", Price: 112},
			{AssetID: "a1", Date: "2024-01-22", Price: 120}, // "today"
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	gains := ComputePeriodGains(p, "", today)
	byLabel := make(map[string]PeriodGain)
	for _, g := range gains {
		byLabel[g.Label] = g
	}

	day := byLabel["Day"]
	if !day.HasData {
		t.Fatalf("Day.HasData = false, want true")
	}
	// Day window: 2024-01-21 (no price that day - PriceAsOf carries
	// forward from 2024-01-19's 112, which is ALSO where t2's 40 units
	// already landed, since 2024-01-19 is before the window even starts)
	// to 2024-01-22 (120) - 140 units held throughout the window itself,
	// pure price movement, nothing to exclude (t2 predates the window).
	wantDayGain := round2(140 * (120 - 112))
	if day.Gain != wantDayGain {
		t.Errorf("Day.Gain = %v, want %v", day.Gain, wantDayGain)
	}

	year := byLabel["Year"]
	if !year.HasData {
		t.Fatalf("Year.HasData = false, want true")
	}
	// Year window starts 2023-01-23 (before t2's 2024-01-19 purchase, so
	// StartValue = 100 units * price as of 2023-01-23, carried forward
	// from 2023-01-01's 100) to today's EndValue = 140 units * 120.
	// EndInvested - StartInvested = 5000 (exactly t2) - must be excluded.
	startValue := 100.0 * 100 // carried-forward price, only 100 units held before t2
	endValue := 140.0 * 120
	wantYearGain := round2((endValue - startValue) - 5000)
	if year.Gain != wantYearGain {
		t.Errorf("Year.Gain = %v, want %v (contribution must be excluded from gain)", year.Gain, wantYearGain)
	}
}

func TestComputePeriodGains_DayAnchorsToLatestPriceNotCalendarToday(t *testing.T) {
	// The confirmed bug this guards against: prices last fetched 3 days
	// before "today" (very ordinary - this app only fetches on request).
	// A calendar-today-anchored Day window would compare "today" against
	// "yesterday", both resolving via carry-forward to the exact same
	// stale price - reporting Day as flat (0) even though the fund
	// genuinely moved on the last day real data exists for.
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "a1", AccountID: "acc-in", Name: "Nippon India Growth Mid Cap Fund"}},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2023-01-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-01-18", Price: 110},
			{AssetID: "a1", Date: "2024-01-19", Price: 112}, // the latest available price - 3 days stale relative to "today"
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC) // no price fetched since 2024-01-19

	gains := ComputePeriodGains(p, "", today)
	var day PeriodGain
	for _, g := range gains {
		if g.Label == "Day" {
			day = g
		}
	}
	if !day.HasData {
		t.Fatalf("Day.HasData = false, want true (there IS real price history, just not fetched today)")
	}
	if day.Gain == 0 {
		t.Errorf("Day.Gain = 0, want the real 2024-01-18->2024-01-19 move (100 * (112-110) = 200) - this is exactly the bug being fixed")
	}
	wantGain := round2(100 * (112.0 - 110.0))
	if day.Gain != wantGain {
		t.Errorf("Day.Gain = %v, want %v", day.Gain, wantGain)
	}
	// StartDate/EndDate must reflect the REAL dates compared (2024-01-18
	// -> 2024-01-19), not "today" (2024-01-22) or blank - this is what
	// lets the person actually check which dates a figure is comparing
	// from the app itself, instead of guessing blind when a number looks
	// wrong (see PeriodGain's own doc comment for why this was added).
	if day.StartDate != "2024-01-18" || day.EndDate != "2024-01-19" {
		t.Errorf("Day StartDate/EndDate = %q/%q, want 2024-01-18/2024-01-19", day.StartDate, day.EndDate)
	}
}

func TestComputePeriodGains_DayAnchorIsSharedAcrossMemberScopes(t *testing.T) {
	// The exact confirmed real bug (reported live: mother's own Day
	// figure showed a real -614 move, but "Me" and "All (family)" both
	// showed a flat 0 - even though family is only Me + Mother). Root
	// cause: each member scope picked its OWN latest-priced date as the
	// Day anchor, so "Mother" anchored to HER OWN most-recent price date
	// (where she genuinely had a fresh price update), while "family"
	// anchored to whichever member's data was MORE recent overall (in
	// the real report, "Me"'s) - a date where Mother's fund had no fresh
	// price yet, so it carried forward flat and contributed nothing to
	// the family total. Two different scopes were answering "what
	// changed" for two different CALENDAR DAYS, so they could never be
	// expected to add up. Fixed by anchoring Day to the latest price
	// date across the WHOLE portfolio, shared by every member scope - a
	// member without a fresh price exactly on that shared date now
	// consistently shows flat (carried forward) in EVERY scope that
	// includes them, rather than surfacing a real move only in their own
	// standalone view on a different day nobody else is looking at.
	p := &store.Portfolio{
		Members: []store.Member{{ID: "me", Name: "Me"}, {ID: "mom", Name: "Mother"}},
		Accounts: []store.Account{
			{ID: "acc-me", MemberID: "me", Name: "My Account", Currency: "INR"},
			{ID: "acc-mom", MemberID: "mom", Name: "Mother's Account", Currency: "INR"},
		},
		Assets: []store.Asset{
			{ID: "a-me", AccountID: "acc-me", Name: "My Fund"},
			{ID: "a-mom", AccountID: "acc-mom", Name: "Mother's Fund"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t-me", AccountID: "acc-me", AssetID: "a-me", Date: "2023-01-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
			{ID: "t-mom", AccountID: "acc-mom", AssetID: "a-mom", Date: "2023-01-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			// My fund: priced up through the portfolio's overall latest
			// date (2024-01-19), with a real move that day.
			{AssetID: "a-me", Date: "2024-01-18", Price: 100},
			{AssetID: "a-me", Date: "2024-01-19", Price: 106}, // +6 per unit, real move
			// Mother's fund: last priced 2024-01-17 - two days STALER
			// than the portfolio's overall latest date. She has no
			// price recorded for 2024-01-18 or 2024-01-19 at all, so
			// both carry forward the same 94 - flat, not a real
			// same-day comparison.
			{AssetID: "a-mom", Date: "2024-01-16", Price: 100},
			{AssetID: "a-mom", Date: "2024-01-17", Price: 94},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	dayGain := func(memberID string) PeriodGain {
		for _, g := range ComputePeriodGains(p, memberID, today) {
			if g.Label == "Day" {
				return g
			}
		}
		t.Fatalf("no Day entry returned for memberID %q", memberID)
		return PeriodGain{}
	}

	family := dayGain("")
	mother := dayGain("mom")
	me := dayGain("me")

	// Mother has no fresh price on the shared anchor date (2024-01-19) -
	// both start and end carry forward the same 2024-01-17 price, so her
	// OWN scope now correctly shows flat too, consistent with family,
	// instead of surfacing a real move dated two days earlier that no
	// other scope was looking at.
	if mother.Gain != 0 {
		t.Errorf("mother.Gain = %v, want 0 (no fresh price on the shared anchor date, so it carries forward flat)", mother.Gain)
	}
	// I DO have a fresh price on the shared anchor date - my real move
	// must still surface.
	wantMeGain := round2(100 * (106.0 - 100.0)) // +600
	if me.Gain != wantMeGain {
		t.Errorf("me.Gain = %v, want %v (my real move on the shared anchor date)", me.Gain, wantMeGain)
	}
	// The core property that was broken: family must equal the sum of
	// its members, computed on the SAME calendar day.
	if family.Gain != mother.Gain+me.Gain {
		t.Errorf("family.Gain = %v, want %v (mother %v + me %v) - family must equal the sum of its members", family.Gain, mother.Gain+me.Gain, mother.Gain, me.Gain)
	}
	if family.Gain != wantMeGain {
		t.Errorf("family.Gain = %v, want %v - my real move must reach the family total, not vanish", family.Gain, wantMeGain)
	}
}

func TestComputePeriodGains_DayAnchorIgnoresBenchmarkPrices(t *testing.T) {
	// The exact confirmed real regression from the fix above: Benchmark
	// price history lives in the SAME p.Prices slice as fund NAV history
	// (keyed by Benchmark.ID acting as an AssetID - see store.Benchmark's
	// doc comment), and a tracked index gets refreshed on its own
	// schedule, independent of any actual holding. The first version of
	// the whole-portfolio Day anchor didn't filter Benchmark records out,
	// so a benchmark refreshed MORE RECENTLY than any real holding made
	// EVERY member's Day figure - and the family total - report a flat
	// ₹0 simultaneously: the anchor landed on a date no real holding had
	// any data for, so every holding carried forward flat at both ends
	// of the window. Reported live as "all three (family, me, mother)
	// show precisely zero" - nothing to do with device timezone, since
	// this whole computation never reads a clock, only date strings
	// already stored in Prices.
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "me", Name: "Me"}},
		Accounts: []store.Account{{ID: "acc-me", MemberID: "me", Name: "My Account", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "a-me", AccountID: "acc-me", Name: "My Fund"}},
		Benchmarks: []store.Benchmark{
			{ID: "bench-nifty", Name: "Nifty 50", YahooTicker: "^NSEI"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t-me", AccountID: "acc-me", AssetID: "a-me", Date: "2023-01-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			// My actual holding: real move on 2024-01-18->19.
			{AssetID: "a-me", Date: "2024-01-18", Price: 100},
			{AssetID: "a-me", Date: "2024-01-19", Price: 106}, // +6 per unit, real move
			// The benchmark (NOT a real holding) was refreshed a full
			// week LATER - refreshing an index is a separate action from
			// updating fund NAVs, so this is a routine, expected gap.
			{AssetID: "bench-nifty", Date: "2024-01-25", Price: 24000},
			{AssetID: "bench-nifty", Date: "2024-01-26", Price: 24100},
		},
	}
	today := time.Date(2024, 1, 30, 0, 0, 0, 0, time.UTC)

	var day PeriodGain
	for _, g := range ComputePeriodGains(p, "", today) {
		if g.Label == "Day" {
			day = g
		}
	}

	if !day.HasData {
		t.Fatalf("Day.HasData = false, want true (there IS real holding history)")
	}
	// The buggy version anchored to 2024-01-26 (the benchmark's latest
	// date), where my fund has no data at all, carrying forward flat and
	// reporting exactly 0 - the confirmed real symptom.
	wantGain := round2(100 * (106.0 - 100.0)) // +600
	if day.Gain != wantGain {
		t.Errorf("Day.Gain = %v, want %v - a benchmark's fresher price date must not anchor Day away from real holdings", day.Gain, wantGain)
	}
}

func TestComputePeriodGains_DayAnchorIgnoresLiveIntradayQuotes(t *testing.T) {
	// Diagnosed live, from the dialog's own date output ("Comparing
	// 2026-08-26 -> 2026-08-27" while the 27th's trading session was
	// still ongoing - that date hadn't SETTLED yet): a live ETF/stock
	// quote (Source "YAHOO" - see RefreshSymbolPrices' doc comment) is
	// continuously dated "today" throughout market hours, but mutual
	// fund NAV (the majority of a typical portfolio) publishes only
	// ONCE per day, after the session closes - "today's" NAV genuinely
	// doesn't exist yet while trading is still happening. A live quote
	// pulled the shared Day anchor forward into a day where every
	// mutual fund's NAV carries forward flat, reporting a false ₹0
	// during market hours even though the LAST SETTLED day-over-day
	// move was real and nonzero. Fixed by excluding Source == "YAHOO"
	// (live quotes only) from the anchor - AMFI, TIGZIG_HISTORY, and
	// YAHOO_HISTORY (Yahoo's own settled daily close, distinct from the
	// live current-quote path) all remain eligible, since each is a
	// real, once-published, settled value.
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "me", Name: "Me"}},
		Accounts: []store.Account{{ID: "acc-me", MemberID: "me", Name: "My Account", Currency: "INR"}},
		Assets: []store.Asset{
			{ID: "a-fund", AccountID: "acc-me", Name: "My Mutual Fund"},
			{ID: "a-etf", AccountID: "acc-me", Name: "My ETF"},
		},
		Transactions: []store.StoredTransaction{
			{ID: "t-fund", AccountID: "acc-me", AssetID: "a-fund", Date: "2023-01-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
			{ID: "t-etf", AccountID: "acc-me", AssetID: "a-etf", Date: "2023-01-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			// Mutual fund NAV: settled through 2026-08-26 (yesterday) -
			// a real move on the last two SETTLED days.
			{AssetID: "a-fund", Date: "2026-08-25", Price: 100, Source: "TIGZIG_HISTORY"},
			{AssetID: "a-fund", Date: "2026-08-26", Price: 106, Source: "TIGZIG_HISTORY"}, // +6 per unit, real move
			// ETF: today's session (2026-08-27) is STILL OPEN - this is
			// a live, mid-session quote, not a settled close.
			{AssetID: "a-etf", Date: "2026-08-25", Price: 50, Source: "YAHOO_HISTORY"},
			{AssetID: "a-etf", Date: "2026-08-26", Price: 50, Source: "YAHOO_HISTORY"},
			{AssetID: "a-etf", Date: "2026-08-27", Price: 51, Source: "YAHOO"}, // live quote, today, session still open
		},
	}
	today := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	var day PeriodGain
	for _, g := range ComputePeriodGains(p, "", today) {
		if g.Label == "Day" {
			day = g
		}
	}

	if !day.HasData {
		t.Fatalf("Day.HasData = false, want true (there IS real, settled holding history)")
	}
	if day.EndDate != "2026-08-26" {
		t.Errorf("Day.EndDate = %q, want 2026-08-26 (the last SETTLED day) - the live ETF quote must not pull the anchor into an unsettled trading session", day.EndDate)
	}
	// The buggy version anchored to 2026-08-27 (the live ETF quote's
	// date), where the mutual fund has no data at all yet, carrying
	// forward flat and reporting exactly 0 - the confirmed real symptom.
	wantGain := round2(100 * (106.0 - 100.0)) // +600, the fund's real settled move
	if day.Gain != wantGain {
		t.Errorf("Day.Gain = %v, want %v - a live intraday quote must not anchor Day away from settled NAV data", day.Gain, wantGain)
	}
}

func TestComputePeriodGains_InsufficientHistoryReportsNoData(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "a1", AccountID: "acc-in", Name: "Nippon India Growth Mid Cap Fund"}},
		Transactions: []store.StoredTransaction{
			// Only 10 days of history - the 365-day Year window can't
			// reach back to a real starting point.
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2024-01-12", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-01-12", Price: 100},
			{AssetID: "a1", Date: "2024-01-22", Price: 110},
		},
	}
	today := time.Date(2024, 1, 22, 0, 0, 0, 0, time.UTC)

	gains := ComputePeriodGains(p, "", today)
	byLabel := make(map[string]PeriodGain)
	for _, g := range gains {
		byLabel[g.Label] = g
	}

	if !byLabel["Day"].HasData {
		t.Errorf("Day.HasData = false, want true (10 days of history covers a 1-day window)")
	}
	if byLabel["Year"].HasData {
		t.Errorf("Year.HasData = true, want false (only 10 days of history, can't cover a 365-day window)")
	}
}

func TestComputeCalendarYearGain_ExcludesContributionsMadeSinceJan1(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "a1", AccountID: "acc-in", Name: "Nippon India Growth Mid Cap Fund"}},
		Transactions: []store.StoredTransaction{
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2023-06-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
			// A top-up in February - AFTER Jan 1st, inside the calendar-
			// year window - must be excluded from the gain.
			{ID: "t2", AccountID: "acc-in", AssetID: "a1", Date: "2024-02-10", Type: store.Purchase, Amount: 5000, Units: units(40)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2023-12-31", Price: 100}, // last price before Jan 1st
			{AssetID: "a1", Date: "2024-02-10", Price: 112},
			{AssetID: "a1", Date: "2024-03-15", Price: 120}, // "today"
		},
	}
	today := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	g := ComputeCalendarYearGain(p, "", today)
	if g.Label != "Calendar Year" {
		t.Errorf("Label = %q, want \"Calendar Year\"", g.Label)
	}
	if !g.HasData {
		t.Fatalf("HasData = false, want true")
	}
	// Jan 1st 2024: only t1's 100 units held (t2 is Feb, after Jan 1st),
	// price carried forward from 2023-12-31's 100 -> StartValue = 10000,
	// StartInvested = 10000. Today: 140 units * 120 = 16800,
	// EndInvested = 15000. EndInvested-StartInvested = 5000 (t2) excluded.
	wantGain := round2((140.0*120 - 100.0*100) - 5000)
	if g.Gain != wantGain {
		t.Errorf("Gain = %v, want %v (t2's Feb contribution must be excluded)", g.Gain, wantGain)
	}
}

func TestComputeCalendarYearGain_InsufficientHistoryForCurrentJan1ReportsNoData(t *testing.T) {
	p := &store.Portfolio{
		Members:  []store.Member{{ID: "m1", Name: "Saby"}},
		Accounts: []store.Account{{ID: "acc-in", MemberID: "m1", Name: "Nippon India Mutual Fund", Currency: "INR"}},
		Assets:   []store.Asset{{ID: "a1", AccountID: "acc-in", Name: "Nippon India Growth Mid Cap Fund"}},
		Transactions: []store.StoredTransaction{
			// First transaction is AFTER this year's Jan 1st - no real
			// baseline exists for a "since Jan 1st" figure yet.
			{ID: "t1", AccountID: "acc-in", AssetID: "a1", Date: "2024-02-01", Type: store.Purchase, Amount: 10000, Units: units(100)},
		},
		Prices: []store.PriceRecord{
			{AssetID: "a1", Date: "2024-02-01", Price: 100},
			{AssetID: "a1", Date: "2024-03-15", Price: 110},
		},
	}
	today := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)

	g := ComputeCalendarYearGain(p, "", today)
	if g.HasData {
		t.Errorf("HasData = true, want false (portfolio didn't exist yet on Jan 1st this year)")
	}
}
