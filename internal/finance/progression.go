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
			isEquity = EffectiveAssetClass(asset.AssetClass, asset.Name) == "Equity"
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

// ComputeAssetProgression computes a weekly (plus today) progression
// series for exactly ONE asset (a single fund), letting someone browse
// a specific holding's own growth story rather than only ever seeing it
// blended into a whole-portfolio or whole-axis view. Unlike
// ComputeProgression, this ignores axis/member scoping entirely - a
// single fund is already fully scoped by definition - and always weighs
// that one asset at 1.0.
func ComputeAssetProgression(p *store.Portfolio, assetID string, today time.Time) []ProgressionPoint {
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

	dates := WeeklyDates(p, today)
	points := make([]ProgressionPoint, 0, len(dates))
	for _, date := range dates {
		points = append(points, computeProgressionPoint(p, accountByID, assetByID, included, weights, date))
	}
	return points
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
func ComputeProgression(p *store.Portfolio, memberID string, axis ProgressionAxis, today time.Time) []ProgressionPoint {
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

	dates := WeeklyDates(p, today)
	points := make([]ProgressionPoint, 0, len(dates))
	for _, date := range dates {
		points = append(points, computeProgressionPoint(p, accountByID, assetByID, included, weights, date))
	}
	return points
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
