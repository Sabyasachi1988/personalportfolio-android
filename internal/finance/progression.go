package finance

import (
	"strings"
	"time"

	"ledger/internal/store"
)

// ProgressionAxis selects which classified subset of the portfolio a
// progression series covers.
type ProgressionAxis string

const (
	AxisWholePortfolio      ProgressionAxis = "WholePortfolio"
	AxisIndianEquity        ProgressionAxis = "IndianEquity"
	AxisInternationalEquity ProgressionAxis = "InternationalEquity"
	AxisCombinedEquity      ProgressionAxis = "CombinedEquity"
)

// ProgressionPoint is one point-in-time snapshot of the portfolio (or a
// classified subset of it, per ProgressionAxis), as of Date. Invested,
// Value and Gain are always computed and stored in INR - the portfolio's
// implicit base currency (see store.FXRate's doc comment) - regardless
// of which axis or which currencies the underlying holdings are in.
// INRPerCAD is the INR-per-1-CAD rate as of this exact point's own Date
// (not today's rate), cached here so a caller can convert to CAD for
// display without a second lookup; HasINRPerCAD is false only if no FX
// history covers this date yet (see store.FXRateAsOf /
// UpdateHistoricalFX).
type ProgressionPoint struct {
	Date         string
	Invested     float64
	Value        float64
	Gain         float64
	GainPercent  float64
	XIRR         float64
	HasXIRR      bool
	INRPerCAD    float64
	HasINRPerCAD bool
}

// WeeklyDates returns the weekly checkpoint dates for a progression
// view: every Monday from the earliest transaction across the WHOLE
// portfolio (not axis-scoped - the browsing calendar is the same
// regardless of which axis is later selected) through the most recent
// Monday on/before today, with today itself appended as an extra final
// point whenever today isn't itself a Monday - so the series is never
// more than 6 days stale (e.g. sitting on a Wednesday includes the
// latest Monday and that Wednesday, no further).
//
// The first checkpoint is the Monday ON OR AFTER the earliest
// transaction (not before) - so the very first point already reflects
// having invested something, rather than opening on an all-zero
// baseline week before any money moved. This is a reasonable reading of
// "from the first transaction", not a verified spec, and is easy to
// flip to "Monday on/before" if that's not the intended convention.
//
// Returns "YYYY-MM-DD" strings, ascending, no duplicates. Returns nil
// if the portfolio has no transactions at all (nothing to browse yet).
func WeeklyDates(p *store.Portfolio, today time.Time) []string {
	var earliest time.Time
	found := false
	for _, t := range p.Transactions {
		d, err := time.Parse(dateLayout, t.Date)
		if err != nil {
			continue
		}
		if !found || d.Before(earliest) {
			earliest = d
			found = true
		}
	}
	if !found {
		return nil
	}

	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	earliest = time.Date(earliest.Year(), earliest.Month(), earliest.Day(), 0, 0, 0, 0, time.UTC)

	firstMonday := earliest
	for firstMonday.Weekday() != time.Monday {
		firstMonday = firstMonday.AddDate(0, 0, 1)
	}

	lastMonday := today
	for lastMonday.Weekday() != time.Monday {
		lastMonday = lastMonday.AddDate(0, 0, -1)
	}

	var dates []string
	if !firstMonday.After(lastMonday) {
		for d := firstMonday; !d.After(lastMonday); d = d.AddDate(0, 0, 7) {
			dates = append(dates, d.Format(dateLayout))
		}
	}
	if today.Weekday() != time.Monday {
		dates = append(dates, today.Format(dateLayout))
	}
	return dates
}

// classifyForeignAsset determines whether a manually-entered foreign
// (non-INR-account) holding - a Canadian-brokerage ETF or stock, which
// carries no AMFI category - is Equity, Debt, or Commodity, from its
// fund name. Unlike GuessMarketCapSegment (built for Indian AMFI
// fund-naming conventions), this targets the much smaller vocabulary
// actually seen in Canadian-brokerage ETF/stock names.
//
// Defaults to Equity when nothing matches: the overwhelming majority of
// retail equity/index ETF names carry no distinguishing keyword at all
// (e.g. "S&P 500 ETF", "Nasdaq 100 ETF", or a plain stock ticker). This
// default is a starting assumption, not a verified fact - it should be
// reconsidered if a bond or money-market ETF is ever added manually.
func classifyForeignAsset(name string) string {
	n := strings.ToLower(name)
	switch {
	case containsAny(n, "gold", "silver", "commodity", "commodities"):
		return "Commodity"
	case containsAny(n, "bond", "treasury", "aggregate bond", "money market", "t-bill"):
		return "Debt"
	default:
		return "Equity"
	}
}

// assetProgressionWeights computes, for every Asset in the portfolio,
// the fraction of that asset's INR value/cash-flows that count toward
// the given axis. A weight of 0 means the asset contributes nothing to
// this axis at all (e.g. a debt fund under any Equity axis).
//
// Classification is applied retroactively using TODAY's current
// EffectiveAssetClass / EquityOriginComposition entries across all of
// history - compositions aren't tracked as time-varying in this app, so
// there is no historical alternative to apply instead (confirmed design
// decision, not an oversight).
//
// Indian-side (INR-account) equity funds split Indian/International by
// their entered EquityOriginComposition (default 100% Indian - see that
// type's doc comment). Foreign-side (non-INR-account) equity holdings -
// the actual Canadian-brokerage ETFs/stocks - count 100% toward
// International and never toward Indian, regardless of what index they
// track; "International equity" in this app specifically means real
// foreign-brokerage holdings, not an Indian fund-of-fund wrapper (see
// classifyForeignAsset).
func assetProgressionWeights(p *store.Portfolio, accountByID map[string]store.Account, axis ProgressionAxis) map[string]float64 {
	weights := make(map[string]float64, len(p.Assets))
	for _, asset := range p.Assets {
		acct, ok := accountByID[asset.AccountID]
		if !ok {
			continue
		}

		var isEquity bool
		if acct.Currency == "INR" {
			isEquity = EffectiveAssetClass(asset.AssetClass, asset.Name, asset.AssetClassOverride) == "Equity"
		} else {
			isEquity = classifyForeignAsset(asset.Name) == "Equity"
		}

		switch axis {
		case AxisWholePortfolio:
			weights[asset.ID] = 1.0
			continue
		}

		if !isEquity {
			weights[asset.ID] = 0
			continue
		}

		switch axis {
		case AxisCombinedEquity:
			weights[asset.ID] = 1.0
		case AxisIndianEquity:
			if acct.Currency != "INR" {
				weights[asset.ID] = 0
				continue
			}
			indianPct := 100.0
			if comp, ok := p.GetEquityOriginComposition(asset.ID); ok {
				if sum := comp.Indian + comp.International; sum > 0 {
					indianPct = comp.Indian / sum * 100
				}
			}
			weights[asset.ID] = indianPct / 100
		case AxisInternationalEquity:
			if acct.Currency != "INR" {
				weights[asset.ID] = 1.0
				continue
			}
			intlPct := 0.0
			if comp, ok := p.GetEquityOriginComposition(asset.ID); ok {
				if sum := comp.Indian + comp.International; sum > 0 {
					intlPct = comp.International / sum * 100
				}
			}
			weights[asset.ID] = intlPct / 100
		}
	}
	return weights
}

// DailyDatesInRange returns every calendar date from start to end
// inclusive - both explicitly bounded by the caller, unlike WeeklyDates
// (which always starts at the portfolio's earliest transaction and ends
// today). Used for the zoomed-in daily view: the caller already knows
// exactly which narrow window it wants (see ComputeProgressionDailyRange
// / ComputeAssetProgressionDailyRange), so there's no need to scan the
// whole portfolio for an earliest-transaction date the way WeeklyDates
// does.
//
// Returns "YYYY-MM-DD" strings, ascending, one per calendar day
// (weekends included - a fund simply won't have a price on a non-trading
// day, which computeProgressionPoint already handles the same way it
// handles any date with no PriceRecord). Returns nil if start is after
// end.
func DailyDatesInRange(start, end time.Time) []string {
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	if start.After(end) {
		return nil
	}
	var dates []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d.Format(dateLayout))
	}
	return dates
}

// computeProgressionSeries is the shared core behind ComputeProgression,
// ComputeAssetProgression, and their DailyRange counterparts - only the
// `dates` list differs between weekly (WeeklyDates) and bounded-daily
// (DailyDatesInRange) callers; every point itself is computed identically
// regardless of which calendar granularity produced its date.
func computeProgressionSeries(
	p *store.Portfolio,
	accountByID map[string]store.Account,
	assetByID map[string]store.Asset,
	included map[string]bool,
	weights map[string]float64,
	dates []string,
) []ProgressionPoint {
	points := make([]ProgressionPoint, 0, len(dates))
	for _, date := range dates {
		points = append(points, computeProgressionPoint(p, accountByID, assetByID, included, weights, date))
	}
	return points
}

// PeriodGain is one window's "how's it doing lately" summary - see
// ComputePeriodGains' doc comment for exactly what Gain/Percent do (and
// deliberately don't) include. Used both for the ROLLING windows
// ComputePeriodGains returns and the single CALENDAR window
// ComputeCalendarYearGain returns - same methodology either way, only
// how the start date is chosen differs.
type PeriodGain struct {
	Label   string  // e.g. "Day", "Year", "Calendar Year"
	Gain    float64 // INR, market-movement only - contributions during the window excluded, see doc comment
	Percent float64 // Gain / start-of-window Value * 100
	HasData bool    // false if the portfolio's history doesn't yet reach back to this window's start date
}

// ComputePeriodGains computes rolling Day and Year (365d) gains for the
// WHOLE portfolio (always AxisWholePortfolio - a dashboard-level
// summary, not scoped to any one axis/fund/group/tag), for a compact
// "how's it doing lately" strip. See periodGainForWindow's doc comment
// for exactly what Gain/Percent mean and why.
//
// Day is DELIBERATELY anchored to the most recent date ANY included
// asset actually has price data for - NOT literal calendar today. This
// mirrors a confirmed bug fix in finance.ComputeTrailingReturn (see its
// doc comment): if prices haven't been re-fetched today (the normal
// case - AMFI/exchange data usually lags, and this app only updates on
// request), comparing "today" against "yesterday" would resolve BOTH
// through carry-forward to the exact same cached price for every stale
// asset, silently reporting Day as flat every single time regardless of
// what actually happened in the market. Anchoring to whatever the
// freshest available data actually is means an asset that DID get a new
// price shows its real move, and one that didn't correctly contributes
// zero (because relative to what we know, it genuinely hasn't moved) -
// Year keeps the literal calendar-today anchor since it isn't vulnerable
// to this exact collapse (365 days is long enough that the start and end
// virtually never land on the same cached price purely from data being a
// few days stale).
func ComputePeriodGains(p *store.Portfolio, memberID string, today time.Time) []PeriodGain {
	accountByID, assetByID, included, weights := progressionInputs(p, memberID, AxisWholePortfolio)
	earliestDate := earliestIncludedTransactionDate(p, included)

	results := make([]PeriodGain, 0, 2)

	if dayAnchorStr := latestIncludedPriceDate(p, included); dayAnchorStr != "" {
		if dayAnchorTime, err := time.Parse(dateLayout, dayAnchorStr); err == nil {
			dayEndPoint := computeProgressionPoint(p, accountByID, assetByID, included, weights, dayAnchorStr)
			dayStartStr := dayAnchorTime.AddDate(0, 0, -1).Format(dateLayout)
			results = append(results, periodGainForWindow(p, accountByID, assetByID, included, weights, earliestDate, "Day", dayStartStr, dayEndPoint))
		} else {
			results = append(results, PeriodGain{Label: "Day", HasData: false})
		}
	} else {
		results = append(results, PeriodGain{Label: "Day", HasData: false})
	}

	yearEndPoint := computeProgressionPoint(p, accountByID, assetByID, included, weights, today.Format(dateLayout))
	yearStartStr := today.AddDate(0, 0, -365).Format(dateLayout)
	results = append(results, periodGainForWindow(p, accountByID, assetByID, included, weights, earliestDate, "Year", yearStartStr, yearEndPoint))

	return results
}

// ComputeCalendarYearGain is ComputePeriodGains' calendar-bound sibling:
// one PeriodGain (labeled "Calendar Year") from January 1st of today's
// year through today - i.e. year-to-date, unlike ComputePeriodGains'
// "Year" entry which is a ROLLING trailing-365-days window. Both use the
// exact same net-of-contributions methodology - see
// periodGainForWindow's doc comment.
func ComputeCalendarYearGain(p *store.Portfolio, memberID string, today time.Time) PeriodGain {
	accountByID, assetByID, included, weights := progressionInputs(p, memberID, AxisWholePortfolio)
	earliestDate := earliestIncludedTransactionDate(p, included)
	endPoint := computeProgressionPoint(p, accountByID, assetByID, included, weights, today.Format(dateLayout))

	jan1 := time.Date(today.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	return periodGainForWindow(p, accountByID, assetByID, included, weights, earliestDate, "Calendar Year", jan1.Format(dateLayout), endPoint)
}

// periodGainForWindow computes one PeriodGain from startDateStr through
// endPoint's own date (the caller has already computed endPoint once and
// reuses it across every window sharing the same "today").
//
// Gain is DELIBERATELY net of contributions - money added or withdrawn
// during the window is excluded, so a fresh SIP or lump-sum investment
// doesn't inflate "gain", and a redemption doesn't look like a loss.
// Concretely:
//
//	Gain = (ValueEnd - ValueStart) - (InvestedEnd - InvestedStart)
//
// where (InvestedEnd - InvestedStart) is exactly the net contribution
// during the window - computeProgressionPoint already tracks Invested
// this way for the same reason XIRR needs it. Percent is
// Gain / ValueStart * 100 - the market return on whatever was ALREADY
// invested at the start of the window, ignoring both the timing and the
// size of any contribution made during it. This is a simplification of
// the standard Modified Dietz method (which would time-weight a
// mid-period contribution by the fraction of the period it was actually
// invested for) - acceptable here because excluding contributions from
// the numerator already prevents the one distortion that would matter
// for a glance-level figure (a same-day lump sum inflating "gain"); a
// full Modified Dietz treatment would be more precise but isn't worth
// the added complexity here the way it already IS worth it for the
// dedicated Progression/XIRR screens, which do this properly per-flow.
//
// HasData is false (Gain/Percent are then meaningless zeros, and the
// caller should show something like "Not enough history yet" rather
// than a misleading 0.00%) when the portfolio's earliest transaction
// doesn't reach back to the window's start date - e.g. a Calendar Year
// figure requested in February for a portfolio opened that same month
// has no real January 1st baseline to compare against.
func periodGainForWindow(
	p *store.Portfolio,
	accountByID map[string]store.Account,
	assetByID map[string]store.Asset,
	included map[string]bool,
	weights map[string]float64,
	earliestDate string,
	label string,
	startDateStr string,
	endPoint ProgressionPoint,
) PeriodGain {
	if earliestDate == "" || startDateStr < earliestDate {
		return PeriodGain{Label: label, HasData: false}
	}
	startPoint := computeProgressionPoint(p, accountByID, assetByID, included, weights, startDateStr)
	gain := (endPoint.Value - startPoint.Value) - (endPoint.Invested - startPoint.Invested)
	var percent float64
	if startPoint.Value != 0 {
		percent = gain / startPoint.Value * 100
	}
	return PeriodGain{Label: label, Gain: round2(gain), Percent: round2(percent), HasData: true}
}

// latestIncludedPriceDate returns the most recent date any included
// asset actually has a stored price for, or "" if none do - used by
// ComputePeriodGains to anchor the "Day" window to real data rather
// than literal calendar today (see that function's doc comment for why).
func latestIncludedPriceDate(p *store.Portfolio, included map[string]bool) string {
	latest := ""
	for _, rec := range p.Prices {
		if !included[rec.AssetID] {
			continue
		}
		if rec.Date > latest {
			latest = rec.Date
		}
	}
	return latest
}

// earliestIncludedTransactionDate returns the earliest transaction date
// among only the assets in `included` (i.e. respecting whatever
// member/axis scope the caller already resolved), or "" if there are
// none - used by ComputePeriodGains to tell "genuinely no history that
// far back yet" apart from "there's history, it just happens to be flat
// that far back". Deliberately scoped, unlike WeeklyDates' earliest-
// transaction scan (see its own doc comment), because HasData needs to
// reflect what THIS caller can actually see, not the whole file's
// earliest transaction regardless of member/axis scope.
func earliestIncludedTransactionDate(p *store.Portfolio, included map[string]bool) string {
	earliest := ""
	for _, t := range p.Transactions {
		if !included[t.AssetID] {
			continue
		}
		if earliest == "" || t.Date < earliest {
			earliest = t.Date
		}
	}
	return earliest
}

// ComputeGroupProgression computes a weekly (plus today) progression
// series for every asset sharing the same fund-group label (see
// store.Asset.GroupLabel's doc comment and GroupHoldingsByLabel) -
// letting someone browse a CONSOLIDATED group's own growth story (e.g.
// several different-AMC "Nifty 50" funds combined) the same way
// ComputeAssetProgression does for a single fund. All matching assets
// are weighted 1.0 (fully summed together), NOT a fractional axis
// split - grouping means "add these up", same convention
// GroupHoldingsByLabel already uses for the Holdings screen's
// consolidated view. memberID scopes to one member's assets first
// (empty = whole family), matching ComputeGroupedHoldings' own
// convention for the same reason: two different members holding
// same-labeled funds should never silently sum together.
//
// cache may be nil - see ComputeProgression's doc comment.
func ComputeGroupProgression(p *store.Portfolio, memberID, groupLabel string, today time.Time, cache *ProgressionCache) []ProgressionPoint {
	accountByID, assetByID, included, weights := groupProgressionInputs(p, memberID, groupLabel)
	dates := WeeklyDates(p, today)
	if cache == nil {
		return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
	}
	cacheKey := "group:" + memberID + ":" + groupLabel
	return computeProgressionSeriesCached(p, accountByID, assetByID, included, weights, dates, cache, cacheKey)
}

// ComputeGroupProgressionDailyRange is ComputeGroupProgression's
// daily-granularity counterpart, bounded to [start, end] - see
// DailyDatesInRange's doc comment and ComputeAssetProgressionDailyRange's
// caveat about scope (a zoomed-in window, not full-history browsing).
func ComputeGroupProgressionDailyRange(p *store.Portfolio, memberID, groupLabel string, start, end time.Time) []ProgressionPoint {
	accountByID, assetByID, included, weights := groupProgressionInputs(p, memberID, groupLabel)
	dates := DailyDatesInRange(start, end)
	return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
}

func groupProgressionInputs(p *store.Portfolio, memberID, groupLabel string) (map[string]store.Account, map[string]store.Asset, map[string]bool, map[string]float64) {
	accountByID := make(map[string]store.Account, len(p.Accounts))
	for _, a := range p.Accounts {
		accountByID[a.ID] = a
	}
	assetByID := make(map[string]store.Asset, len(p.Assets))
	for _, a := range p.Assets {
		assetByID[a.ID] = a
	}

	included := make(map[string]bool)
	weights := make(map[string]float64)
	for _, a := range p.Assets {
		if a.GroupLabel != groupLabel {
			continue
		}
		acct, ok := accountByID[a.AccountID]
		if !ok {
			continue
		}
		if memberID != "" && acct.MemberID != memberID {
			continue
		}
		included[a.ID] = true
		weights[a.ID] = 1.0
	}

	return accountByID, assetByID, included, weights
}

// ComputeTagProgression computes a weekly (plus today) progression series
// for every asset carrying the given tag (see store.Asset.Tags' doc
// comment) - letting someone browse a tag's combined growth story (e.g.
// every fund tagged "Mid Cap", regardless of AMC or GroupLabel) the same
// way ComputeGroupProgression does for a GroupLabel. All matching assets
// are weighted 1.0 (fully summed together) - same "add these up"
// convention as GroupLabel grouping. Unlike a pie/donut chart, this
// deliberately does NOT use EffectiveTag/PrimaryTag exclusivity - an
// asset carrying several tags correctly contributes to each of THEIR
// progression lines independently; that's not a collision the way it
// would be for a single pie slice, so nothing here needs to pick a
// "winning" tag. memberID scopes to one member's assets first (empty =
// whole family), same convention as ComputeGroupProgression.
//
// cache may be nil - see ComputeProgression's doc comment.
func ComputeTagProgression(p *store.Portfolio, memberID, tag string, today time.Time, cache *ProgressionCache) []ProgressionPoint {
	accountByID, assetByID, included, weights := tagProgressionInputs(p, memberID, tag)
	dates := WeeklyDates(p, today)
	if cache == nil {
		return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
	}
	cacheKey := "tag:" + memberID + ":" + tag
	return computeProgressionSeriesCached(p, accountByID, assetByID, included, weights, dates, cache, cacheKey)
}

// ComputeTagProgressionDailyRange is ComputeTagProgression's
// daily-granularity counterpart, bounded to [start, end] - see
// DailyDatesInRange's doc comment and ComputeAssetProgressionDailyRange's
// caveat about scope (a zoomed-in window, not full-history browsing).
func ComputeTagProgressionDailyRange(p *store.Portfolio, memberID, tag string, start, end time.Time) []ProgressionPoint {
	accountByID, assetByID, included, weights := tagProgressionInputs(p, memberID, tag)
	dates := DailyDatesInRange(start, end)
	return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
}

func tagProgressionInputs(p *store.Portfolio, memberID, tag string) (map[string]store.Account, map[string]store.Asset, map[string]bool, map[string]float64) {
	accountByID := make(map[string]store.Account, len(p.Accounts))
	for _, a := range p.Accounts {
		accountByID[a.ID] = a
	}
	assetByID := make(map[string]store.Asset, len(p.Assets))
	for _, a := range p.Assets {
		assetByID[a.ID] = a
	}

	included := make(map[string]bool)
	weights := make(map[string]float64)
	for _, a := range p.Assets {
		if !containsTag(a.Tags, tag) {
			continue
		}
		acct, ok := accountByID[a.AccountID]
		if !ok {
			continue
		}
		if memberID != "" && acct.MemberID != memberID {
			continue
		}
		included[a.ID] = true
		weights[a.ID] = 1.0
	}

	return accountByID, assetByID, included, weights
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ComputeAssetProgression computes a weekly (plus today) progression
// series for exactly ONE asset (a single fund), letting someone browse
// a specific holding's own growth story rather than only ever seeing it
// blended into a whole-portfolio or whole-axis view. Unlike
// ComputeProgression, this ignores axis/member scoping entirely - a
// single fund is already fully scoped by definition - and always weighs
// that one asset at 1.0.
//
// cache may be nil - see ComputeProgression's doc comment.
func ComputeAssetProgression(p *store.Portfolio, assetID string, today time.Time, cache *ProgressionCache) []ProgressionPoint {
	accountByID, assetByID, included, weights := singleAssetProgressionInputs(p, assetID)
	dates := WeeklyDates(p, today)
	if cache == nil {
		return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
	}
	return computeProgressionSeriesCached(p, accountByID, assetByID, included, weights, dates, cache, "asset:"+assetID)
}

// ComputeAssetProgressionDailyRange is ComputeAssetProgression's
// daily-granularity counterpart, bounded to [start, end] - see
// DailyDatesInRange's doc comment. Intended for a zoomed-in chart
// window, not full-history browsing (which would mean thousands of
// points for a fund with many years of history - see this package's
// benchmark for the actual cost at scale).
func ComputeAssetProgressionDailyRange(p *store.Portfolio, assetID string, start, end time.Time) []ProgressionPoint {
	accountByID, assetByID, included, weights := singleAssetProgressionInputs(p, assetID)
	dates := DailyDatesInRange(start, end)
	return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
}

func singleAssetProgressionInputs(p *store.Portfolio, assetID string) (map[string]store.Account, map[string]store.Asset, map[string]bool, map[string]float64) {
	accountByID := make(map[string]store.Account, len(p.Accounts))
	for _, a := range p.Accounts {
		accountByID[a.ID] = a
	}
	assetByID := make(map[string]store.Asset, len(p.Assets))
	for _, a := range p.Assets {
		assetByID[a.ID] = a
	}
	included := map[string]bool{assetID: true}
	weights := map[string]float64{assetID: 1.0}
	return accountByID, assetByID, included, weights
}

// ComputeProgression computes a currency-aware weekly (plus today)
// progression series for the given axis, scoped to a member (empty
// memberID = whole family). Every point is computed in INR (see
// ProgressionPoint's doc comment); any Canadian-account cash flow or
// valuation is converted to INR using the FX rate as of ITS OWN
// historical date (see store.FXRateAsOf), never today's rate, so
// invested-amount accuracy is preserved across years of INR/CAD
// movement. A transaction or valuation date falling before the earliest
// fetched FX history for that currency is silently excluded from that
// point (rather than guessed) - run "Update History" with an earlier
// `since` date if a point looks incomplete.
//
// cache may be nil, which simply disables caching (always computes
// everything fresh) - see ProgressionCache's doc comment for what
// passing a real cache buys.
func ComputeProgression(p *store.Portfolio, memberID string, axis ProgressionAxis, today time.Time, cache *ProgressionCache) []ProgressionPoint {
	accountByID, assetByID, included, weights := progressionInputs(p, memberID, axis)
	dates := WeeklyDates(p, today)
	if cache == nil {
		return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
	}
	cacheKey := "axis:" + memberID + ":" + string(axis)
	return computeProgressionSeriesCached(p, accountByID, assetByID, included, weights, dates, cache, cacheKey)
}

// ComputeProgressionDailyRange is ComputeProgression's daily-granularity
// counterpart, bounded to [start, end] - see DailyDatesInRange's doc
// comment and ComputeAssetProgressionDailyRange's caveat about scope
// (this is for a zoomed-in window, not full-history daily browsing).
func ComputeProgressionDailyRange(p *store.Portfolio, memberID string, axis ProgressionAxis, start, end time.Time) []ProgressionPoint {
	accountByID, assetByID, included, weights := progressionInputs(p, memberID, axis)
	dates := DailyDatesInRange(start, end)
	return computeProgressionSeries(p, accountByID, assetByID, included, weights, dates)
}

func progressionInputs(p *store.Portfolio, memberID string, axis ProgressionAxis) (map[string]store.Account, map[string]store.Asset, map[string]bool, map[string]float64) {
	accountByID := make(map[string]store.Account, len(p.Accounts))
	for _, a := range p.Accounts {
		accountByID[a.ID] = a
	}
	assetByID := make(map[string]store.Asset, len(p.Assets))
	for _, a := range p.Assets {
		assetByID[a.ID] = a
	}

	weights := assetProgressionWeights(p, accountByID, axis)

	included := make(map[string]bool, len(p.Assets))
	for _, asset := range p.Assets {
		acct, ok := accountByID[asset.AccountID]
		if !ok {
			continue
		}
		if memberID != "" && acct.MemberID != memberID {
			continue
		}
		if weights[asset.ID] > 0 {
			included[asset.ID] = true
		}
	}

	return accountByID, assetByID, included, weights
}

func computeProgressionPoint(
	p *store.Portfolio,
	accountByID map[string]store.Account,
	assetByID map[string]store.Asset,
	included map[string]bool,
	weights map[string]float64,
	date string,
) ProgressionPoint {
	var invested, value float64
	var flows []CashFlow
	unitsAsOf := make(map[string]float64, len(included))

	for _, t := range p.Transactions {
		if !included[t.AssetID] || t.Date > date {
			continue
		}
		asset := assetByID[t.AssetID]
		acct := accountByID[asset.AccountID]

		inrAmount := t.Amount
		if acct.Currency != "INR" {
			fx, ok := p.FXRateAsOf(acct.Currency, t.Date)
			if !ok {
				continue // no FX history covering this flow's date yet - excluded, not guessed
			}
			inrAmount = t.Amount * fx
		}

		weighted := inrAmount * weights[t.AssetID]
		invested += weighted

		if d, err := time.Parse(dateLayout, t.Date); err == nil {
			flows = append(flows, CashFlow{Date: d, Amount: -weighted})
		}
		if t.Units != nil {
			unitsAsOf[t.AssetID] += *t.Units
		}
	}

	for assetID := range included {
		units := unitsAsOf[assetID]
		if units <= 0.0001 {
			continue
		}
		asset := assetByID[assetID]
		acct := accountByID[asset.AccountID]

		price, ok := p.PriceAsOf(assetID, date)
		if !ok {
			continue
		}
		inrValue := units * price
		if acct.Currency != "INR" {
			fx, ok := p.FXRateAsOf(acct.Currency, date)
			if !ok {
				continue
			}
			inrValue *= fx
		}
		value += inrValue * weights[assetID]
	}

	pointDate, _ := time.Parse(dateLayout, date)
	if value > 0.0001 {
		flows = append(flows, CashFlow{Date: pointDate, Amount: value})
	}

	point := ProgressionPoint{
		Date:     date,
		Invested: round2(invested),
		Value:    round2(value),
		Gain:     round2(value - invested),
	}
	if invested != 0 {
		point.GainPercent = round2(point.Gain / invested * 100)
	}
	if rate, ok := XIRR(flows); ok {
		point.XIRR = round2(rate * 100)
		point.HasXIRR = true
	}
	if fx, ok := p.FXRateAsOf("CAD", date); ok {
		point.INRPerCAD = fx
		point.HasINRPerCAD = true
	}
	return point
}
