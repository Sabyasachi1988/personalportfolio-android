package finance

import (
	"sort"
	"time"

	"ledger/internal/store"
)

// Holding summarises one asset's current position.
type Holding struct {
	AssetID   string
	AssetName string
	ISIN      string
	// CanonicalName is the SAME fund's display name, harmonized across
	// every Asset that shares this ISIN regardless of which account or
	// import gave it its own AssetName - e.g. one statement calling it
	// "Nippon India Nifty 50" and another calling it "Nippon India
	// Nifty 50 Index Fund" for the exact same underlying fund (same
	// ISIN) would otherwise show as two differently-named things in
	// any list, a confirmed real point of confusion. Resolved to
	// whichever same-ISIN Asset appears FIRST in the portfolio's own
	// Assets order (i.e. whichever was added first) - a stable,
	// deterministic choice that doesn't change as long as no earlier
	// asset is deleted. Equal to AssetName whenever this asset is the
	// only one (or the first one) with its ISIN - the common case.
	CanonicalName string
	// AlsoHeldByMembers lists OTHER members (by name, deduplicated, NOT
	// including this holding's own member) who hold an Asset with the
	// SAME ISIN - e.g. a Nippon India Nifty 50 held by both "Me" and
	// "Mom" under separate accounts. Deliberately informational only -
	// see finance.GroupHoldingsByLabel's own doc comment for why
	// holdings across different members are never merged into one row
	// even when they're genuinely the same fund: ownership matters for
	// tax/cost-basis purposes, so this stays a badge/note, not a merge.
	// Empty (never nil - see ComputeHoldings) when no other member
	// holds the same ISIN.
	AlsoHeldByMembers  []string
	AccountName        string
	MemberID           string
	MemberName         string
	GroupLabel         string   // see store.Asset.GroupLabel's doc comment - "" means ungrouped
	Tags               []string // see store.Asset.Tags' doc comment
	EffectiveTag       string   // see store.Asset.EffectiveTag's doc comment - "" means untagged
	AssetClassOverride string   // see store.Asset.AssetClassOverride's doc comment - "" means no override, use EffectiveAssetClass's normal resolution
	UnitsHeld          float64
	NetInvested        float64 // sum of purchase amounts minus redemption proceeds
	CurrentPrice       float64
	HasPrice           bool
	CurrentValue       float64
	Gain               float64 // CurrentValue - NetInvested
	GainPercent        float64
	XIRR               float64
	HasXIRR            bool
	DayGain            float64 // (latest price - prior price) * UnitsHeld, this fund's own most recent two distinct priced dates
	DayGainPercent     float64
	HasDayGain         bool // false if this asset has fewer than 2 distinct priced dates yet
}

const dateLayout = "2006-01-02"

// ComputeHoldings aggregates transactions into a per-asset summary. Prices
// come from the most recent PriceRecord for that asset, if any.
func ComputeHoldings(p *store.Portfolio) []Holding {
	accountName := make(map[string]string, len(p.Accounts))
	accountMember := make(map[string]string, len(p.Accounts))
	for _, a := range p.Accounts {
		accountName[a.ID] = a.Name
		accountMember[a.ID] = a.MemberID
	}
	memberName := make(map[string]string, len(p.Members))
	for _, m := range p.Members {
		memberName[m.ID] = m.Name
	}

	latestPrice := make(map[string]store.PriceRecord)
	for _, pr := range p.Prices {
		cur, ok := latestPrice[pr.AssetID]
		if !ok || pr.Date > cur.Date {
			latestPrice[pr.AssetID] = pr
		}
	}

	type accum struct {
		units, invested float64
		flows           []CashFlow
	}
	byAsset := make(map[string]*accum)

	for _, t := range p.Transactions {
		a, ok := byAsset[t.AssetID]
		if !ok {
			a = &accum{}
			byAsset[t.AssetID] = a
		}
		if t.Units != nil {
			a.units += *t.Units
		}
		a.invested += t.Amount

		if d, err := time.Parse(dateLayout, t.Date); err == nil {
			// Investor's cash-flow sign is the opposite of the stored
			// Amount sign: a purchase (positive Amount = cost to the
			// investor) is a cash *outflow* for XIRR purposes, and a
			// redemption (negative Amount) is an *inflow*.
			a.flows = append(a.flows, CashFlow{Date: d, Amount: -t.Amount})
		}
	}

	var holdings []Holding
	// Both computed portfolio-WIDE, before any per-holding logic below -
	// canonical name and "also held by" both need to see every Asset
	// sharing an ISIN, not just the ones with transactions (an asset
	// with a transaction obviously has one, but the canonical name
	// should still resolve consistently even if, say, the FIRST-added
	// same-ISIN asset happens to have no transactions yet for some
	// reason).
	canonicalNameByISIN := make(map[string]string)
	membersByISIN := make(map[string]map[string]bool) // ISIN -> set of member NAMES holding it
	for _, asset := range p.Assets {
		if asset.ISIN == "" {
			continue
		}
		if _, ok := canonicalNameByISIN[asset.ISIN]; !ok {
			canonicalNameByISIN[asset.ISIN] = asset.DisplayName()
		}
		mName := memberName[accountMember[asset.AccountID]]
		if mName == "" {
			continue // an Additional Fund (tracked, not owned - see store.Asset.AccountID's doc comment) has no member to attribute
		}
		if membersByISIN[asset.ISIN] == nil {
			membersByISIN[asset.ISIN] = make(map[string]bool)
		}
		membersByISIN[asset.ISIN][mName] = true
	}

	for _, asset := range p.Assets {
		acc, ok := byAsset[asset.ID]
		if !ok {
			continue // no transactions for this asset yet
		}
		var alsoHeldBy []string
		if asset.ISIN != "" {
			thisMember := memberName[accountMember[asset.AccountID]]
			for m := range membersByISIN[asset.ISIN] {
				if m != thisMember {
					alsoHeldBy = append(alsoHeldBy, m)
				}
			}
			sort.Strings(alsoHeldBy) // deterministic order - map iteration alone isn't
		}
		canonicalName := canonicalNameByISIN[asset.ISIN]
		if canonicalName == "" {
			canonicalName = asset.DisplayName() // no ISIN to harmonize against - just use this asset's own name
		}
		h := Holding{
			AssetID:            asset.ID,
			AssetName:          asset.DisplayName(),
			ISIN:               asset.ISIN,
			CanonicalName:      canonicalName,
			AlsoHeldByMembers:  alsoHeldBy,
			AccountName:        accountName[asset.AccountID],
			MemberID:           accountMember[asset.AccountID],
			MemberName:         memberName[accountMember[asset.AccountID]],
			GroupLabel:         asset.GroupLabel,
			Tags:               asset.Tags,
			EffectiveTag:       asset.EffectiveTag(),
			AssetClassOverride: asset.AssetClassOverride,
			UnitsHeld:          round4(acc.units),
			NetInvested:        round2(acc.invested),
		}
		if pr, ok := latestPrice[asset.ID]; ok {
			h.CurrentPrice = pr.Price
			h.HasPrice = true
			h.CurrentValue = round2(acc.units * pr.Price)
			h.Gain = round2(h.CurrentValue - h.NetInvested)
			if h.NetInvested != 0 {
				h.GainPercent = round2(h.Gain / h.NetInvested * 100)
			}

			if priorPrice, ok := priorDistinctPrice(p.PriceSeries(asset.ID)); ok && priorPrice > 0 {
				h.DayGain = round2((pr.Price - priorPrice) * acc.units)
				h.DayGainPercent = round2((pr.Price/priorPrice - 1) * 100)
				h.HasDayGain = true
			}

			flows := append([]CashFlow{}, acc.flows...)
			if h.UnitsHeld > 0.0001 {
				flows = append(flows, CashFlow{Date: time.Now(), Amount: h.CurrentValue})
			}
			if rate, ok := XIRR(flows); ok {
				h.XIRR = round2(rate * 100)
				h.HasXIRR = true
			}
		}
		holdings = append(holdings, h)
	}
	return holdings
}

// PortfolioXIRR pools every holding's cash flows (plus each holding's
// current value as a final inflow, for holdings still held with a known
// price) into one combined XIRR - the return on the portfolio as a
// whole, not fund-by-fund. Only transactions belonging to an asset
// present in `holdings` are included, so passing a member-filtered
// holdings slice scopes the XIRR to that member automatically.
func PortfolioXIRR(p *store.Portfolio, holdings []Holding) (float64, bool) {
	included := make(map[string]bool, len(holdings))
	for _, h := range holdings {
		included[h.AssetID] = true
	}
	var flows []CashFlow
	for _, t := range p.Transactions {
		if !included[t.AssetID] {
			continue
		}
		d, err := time.Parse(dateLayout, t.Date)
		if err != nil {
			continue
		}
		flows = append(flows, CashFlow{Date: d, Amount: -t.Amount})
	}
	for _, h := range holdings {
		if h.HasPrice && h.UnitsHeld > 0.0001 {
			flows = append(flows, CashFlow{Date: time.Now(), Amount: h.CurrentValue})
		}
	}
	rate, ok := XIRR(flows)
	if !ok {
		return 0, false
	}
	return round2(rate * 100), true
}

// FilterHoldingsByMember returns only holdings belonging to memberID.
// An empty memberID returns all holdings unchanged (the "whole family"
// view).
func FilterHoldingsByMember(holdings []Holding, memberID string) []Holding {
	if memberID == "" {
		return holdings
	}
	var out []Holding
	for _, h := range holdings {
		if h.MemberID == memberID {
			out = append(out, h)
		}
	}
	return out
}

// PortfolioTotals sums current value and net invested across all holdings
// that have a known current price.
func PortfolioTotals(holdings []Holding) (invested, value float64, anyPriced bool) {
	for _, h := range holdings {
		if h.HasPrice {
			invested += h.NetInvested
			value += h.CurrentValue
			anyPriced = true
		}
	}
	return round2(invested), round2(value), anyPriced
}

// GroupedHolding is one row of a consolidated, group-aware view: either
// a single ungrouped holding shown exactly as before (IsGroup false), or
// several holdings sharing the same non-empty GroupLabel combined into
// one row (IsGroup true) - see store.Asset.GroupLabel's doc comment for
// why this exists (the same real-world exposure, e.g. "Nifty 50",
// showing up as several separately-held funds from different AMCs).
// AssetIDs always lists every underlying asset this row represents, so
// a caller can still drill into or filter by the individual holdings
// even when they're shown consolidated here.
type GroupedHolding struct {
	DisplayName string // GroupLabel if grouped, else the single constituent's CanonicalName (harmonized across same-ISIN assets - see Holding.CanonicalName's own doc comment)
	IsGroup     bool
	AssetIDs    []string
	// AlsoHeldByMembers is only meaningful (non-empty) for an UNGROUPED
	// row (IsGroup false) - see Holding.AlsoHeldByMembers' own doc
	// comment. A grouped row already consolidates several of the
	// person's OWN holdings under one label; carrying "also held by"
	// through a group as well would conflate two different kinds of
	// "this same fund appears elsewhere" and isn't attempted here -
	// always empty when IsGroup is true.
	AlsoHeldByMembers []string
	MemberID          string
	MemberName        string
	NetInvested       float64
	HasPrice          bool // true if at least one constituent has a price; value/gain/XIRR below only reflect the priced constituents
	CurrentValue      float64
	Gain              float64
	GainPercent       float64
	XIRR              float64
	HasXIRR           bool
	DayGain           float64 // sum of each priced constituent's own DayGain - see Holding.DayGain's doc comment
	DayGainPercent    float64 // DayGain as a percent of the group's CurrentValue as of the PRIOR day (CurrentValue - DayGain), not NetInvested - matches Holding.DayGainPercent's own price-over-price meaning
	HasDayGain        bool    // true only if EVERY priced constituent has day-gain data - a partial sum would understate the real day move for whichever constituent lacks it
}

// GroupHoldingsByLabel consolidates holdings sharing the same
// GroupLabel into one GroupedHolding row apiece, leaving ungrouped
// holdings (GroupLabel == "") as their own individual singleton rows,
// unchanged in substance from the plain Holding they came from.
//
// A group's Gain/GainPercent come from summing its constituents'
// NetInvested/CurrentValue (a plain sum is correct here - money is
// fungible across funds even though units/NAV aren't). XIRR is NOT
// averaged or summed - it's recomputed from scratch by pooling every
// constituent's own transaction cash flows (plus each priced
// constituent's current value as a final inflow) into one combined
// series, same technique as PortfolioXIRR - an average-of-XIRRs would
// be financially meaningless once the constituents have different
// investment timelines.
//
// Two different holdings with the SAME GroupLabel but belonging to
// different members are intentionally never combined into one row -
// grouping only ever consolidates within whatever `holdings` was already
// scoped to (typically already member-filtered by the caller, same
// convention as FilterHoldingsByMember/PortfolioXIRR).
func GroupHoldingsByLabel(p *store.Portfolio, holdings []Holding) []GroupedHolding {
	type bucket struct {
		label   string
		members []Holding
	}
	order := make([]string, 0, len(holdings))
	buckets := make(map[string]*bucket)

	for _, h := range holdings {
		key := h.GroupLabel
		if key == "" {
			// Ungrouped holdings each get their own unique bucket key
			// (by AssetID, not "") so they never accidentally merge
			// with each other.
			key = "\x00ungrouped:" + h.AssetID
		}
		b, ok := buckets[key]
		if !ok {
			b = &bucket{label: h.GroupLabel}
			buckets[key] = b
			order = append(order, key)
		}
		b.members = append(b.members, h)
	}

	transactionsByAsset := make(map[string][]store.StoredTransaction, len(p.Assets))
	for _, t := range p.Transactions {
		transactionsByAsset[t.AssetID] = append(transactionsByAsset[t.AssetID], t)
	}

	out := make([]GroupedHolding, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		g := GroupedHolding{
			IsGroup:    len(b.members) > 1,
			MemberID:   b.members[0].MemberID,
			MemberName: b.members[0].MemberName,
		}
		if g.IsGroup {
			g.DisplayName = b.label
		} else {
			g.DisplayName = b.members[0].CanonicalName
			if g.DisplayName == "" {
				g.DisplayName = b.members[0].AssetName // defensive fallback - CanonicalName is always populated by ComputeHoldings itself, but a Holding built some other way (e.g. directly in a test) might not set it
			}
			g.AlsoHeldByMembers = b.members[0].AlsoHeldByMembers
		}

		var flows []CashFlow
		allPricedHaveDayGain := true
		for _, h := range b.members {
			g.AssetIDs = append(g.AssetIDs, h.AssetID)
			g.NetInvested += h.NetInvested
			for _, t := range transactionsByAsset[h.AssetID] {
				if d, err := time.Parse(dateLayout, t.Date); err == nil {
					flows = append(flows, CashFlow{Date: d, Amount: -t.Amount})
				}
			}
			if h.HasPrice {
				g.HasPrice = true
				g.CurrentValue += h.CurrentValue
				if h.UnitsHeld > 0.0001 {
					flows = append(flows, CashFlow{Date: time.Now(), Amount: h.CurrentValue})
				}
				if h.HasDayGain {
					g.DayGain += h.DayGain
				} else {
					allPricedHaveDayGain = false
				}
			}
		}
		g.NetInvested = round2(g.NetInvested)
		g.CurrentValue = round2(g.CurrentValue)
		if g.HasPrice {
			g.Gain = round2(g.CurrentValue - g.NetInvested)
			if g.NetInvested != 0 {
				g.GainPercent = round2(g.Gain / g.NetInvested * 100)
			}
			if allPricedHaveDayGain {
				g.DayGain = round2(g.DayGain)
				priorValue := g.CurrentValue - g.DayGain
				if priorValue != 0 {
					g.DayGainPercent = round2(g.DayGain / priorValue * 100)
				}
				g.HasDayGain = true
			}
		}
		if rate, ok := XIRR(flows); ok {
			g.XIRR = round2(rate * 100)
			g.HasXIRR = true
		}
		out = append(out, g)
	}
	return out
}

// priorDistinctPrice returns the price at the second-most-recent DISTINCT
// date in an already-sorted-ascending series (e.g. from
// store.Portfolio.PriceSeries) - the "prior" side of a per-fund Day
// gain/loss figure (see Holding.DayGain's doc comment). Distinct dates,
// not just the second-to-last record, because a series can carry more
// than one record for the same date (e.g. a manual entry alongside a
// fetched one) - the day-over-day comparison should skip past any
// same-date duplicates to the genuinely prior trading day. Returns
// ok=false if the series has fewer than 2 distinct dates.
func priorDistinctPrice(series []store.PriceRecord) (float64, bool) {
	if len(series) < 2 {
		return 0, false
	}
	latestDate := series[len(series)-1].Date
	for i := len(series) - 2; i >= 0; i-- {
		if series[i].Date != latestDate {
			return series[i].Price, true
		}
	}
	return 0, false
}

func round2(f float64) float64 {
	return float64(int(f*100+sign(f)*0.5)) / 100
}

func round4(f float64) float64 {
	return float64(int(f*10000+sign(f)*0.5)) / 10000
}

func sign(f float64) float64 {
	if f < 0 {
		return -1
	}
	return 1
}
