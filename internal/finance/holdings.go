package finance

import (
	"time"

	"ledger/internal/store"
)

// Holding summarises one asset's current position.
type Holding struct {
	AssetID      string
	AssetName    string
	ISIN         string
	AccountName  string
	MemberID     string
	MemberName   string
	UnitsHeld    float64
	NetInvested  float64 // sum of purchase amounts minus redemption proceeds
	CurrentPrice float64
	HasPrice     bool
	CurrentValue float64
	Gain         float64 // CurrentValue - NetInvested
	GainPercent  float64
	XIRR         float64
	HasXIRR      bool
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
			AssetID:     asset.ID,
			AssetName:   asset.Name,
			ISIN:        asset.ISIN,
			AccountName: accountName[asset.AccountID],
			MemberID:    accountMember[asset.AccountID],
			MemberName:  memberName[accountMember[asset.AccountID]],
			UnitsHeld:   round4(acc.units),
			NetInvested: round2(acc.invested),
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
