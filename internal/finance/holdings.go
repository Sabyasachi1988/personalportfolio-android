package finance

import (
	"time"

	"ledger/internal/store"
)

// Holding summarises one asset's current position.
type Holding struct {
	AssetID            string
	AssetName          string
	ISIN               string
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
	for _, asset := range p.Assets {
		acc, ok := byAsset[asset.ID]
		if !ok {
			continue // no transactions for this asset yet
		}
		h := Holding{
			AssetID:            asset.ID,
			AssetName:          asset.DisplayName(),
			ISIN:               asset.ISIN,
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
	DisplayName  string // GroupLabel if grouped, else the single constituent's AssetName
	IsGroup      bool
	AssetIDs     []string
	MemberID     string
	MemberName   string
	NetInvested  float64
	HasPrice     bool // true if at least one constituent has a price; value/gain/XIRR below only reflect the priced constituents
	CurrentValue float64
	Gain         float64
	GainPercent  float64
	XIRR         float64
	HasXIRR      bool
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
			g.DisplayName = b.members[0].AssetName
		}

		var flows []CashFlow
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
			}
		}
		g.NetInvested = round2(g.NetInvested)
		g.CurrentValue = round2(g.CurrentValue)
		if g.HasPrice {
			g.Gain = round2(g.CurrentValue - g.NetInvested)
			if g.NetInvested != 0 {
				g.GainPercent = round2(g.Gain / g.NetInvested * 100)
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
