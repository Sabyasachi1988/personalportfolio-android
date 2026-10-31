package finance

import (
	"sort"
	"strings"
	"time"

	"ledger/internal/store"
)

// CompositionPoint is the portfolio's market-cap / sector mix at one month-end, as percent of the
// portfolio's total value that day. Fund splits come from each fund's published month; the fund's
// units on that date (from transactions) times its NAV give the weights.
type CompositionPoint struct {
	Month        string
	Date         string
	Value        float64
	Large        float64
	Mid          float64
	Small        float64
	Cash         float64
	Debt         float64
	Foreign      float64
	Other        float64 // genuinely other instruments: fund units, REIT/InvIT, commodities, anything not equity/debt/cash
	Unclassified float64 // Indian-listed equity the AMFI list does not categorise (new listings, delisted, ISIN gaps)
	NoData       float64 // value held in funds with no published split for that month
	Estimated    float64 // share of value whose month was estimated or carried from another month
	Extra        map[string]float64 // name-guess labels with no published bucket (Multi Cap, Flexi Cap, Commodity), percent of value
	Sectors      map[string]float64
	OtherBy      map[string]float64 // fund name -> percent of portfolio value that sits in Other / unclassified
	NoDataBy     map[string]float64 // fund name -> percent of portfolio value with no published split
}

// monthFor picks the fund month to use for a given YYYY-MM: the exact month, else the latest earlier month,
// else the first later month if it is at most 6 months ahead. Reports whether the choice was not an exact,
// published month (carried / back-filled / estimated).
func fundMonthFor(h *store.FundHistory, month string) (store.FundMonth, bool, bool) {
	var before, after *store.FundMonth
	for i := range h.Months {
		m := &h.Months[i]
		if m.Month == month {
			return *m, m.Estimated, true
		}
		if m.Month < month {
			before = m
		} else if after == nil {
			after = m
		}
	}
	if before != nil {
		// A one-month carry-forward is normal (a month's disclosure is published mid-way through the next month),
		// so it is only flagged when the carried month was itself an estimate or the gap is 2+ months.
		return *before, before.Estimated || monthsBetween(before.Month, month) >= 2, true
	}
	if after != nil && monthsBetween(month, after.Month) <= 6 {
		return *after, true, true
	}
	return store.FundMonth{}, false, false
}

func monthsBetween(a, b string) int {
	ta, e1 := time.Parse("2006-01", a)
	tb, e2 := time.Parse("2006-01", b)
	if e1 != nil || e2 != nil {
		return 999
	}
	return (tb.Year()-ta.Year())*12 + int(tb.Month()-ta.Month())
}

// ComputeCompositionHistory builds one point per month-end from the member's first transaction to today.
func ComputeCompositionHistory(p *store.Portfolio, memberID string, today time.Time) []CompositionPoint {
	accountByID := make(map[string]store.Account, len(p.Accounts))
	for _, a := range p.Accounts {
		accountByID[a.ID] = a
	}
	included := map[string]store.Asset{}
	for _, a := range p.Assets {
		acct, ok := accountByID[a.AccountID]
		if !ok || acct.Currency != "INR" || (memberID != "" && acct.MemberID != memberID) {
			continue
		}
		included[a.ID] = a
	}
	first := ""
	for _, t := range p.Transactions {
		if _, ok := included[t.AssetID]; ok && (first == "" || t.Date < first) {
			first = t.Date
		}
	}
	if first == "" {
		return []CompositionPoint{}
	}
	start, err := time.Parse(dateLayout, first)
	if err != nil {
		return []CompositionPoint{}
	}
	var dates []time.Time
	for y, m := start.Year(), start.Month(); ; {
		end := time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC)
		if end.After(today) {
			td := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
			if len(dates) == 0 || td.After(dates[len(dates)-1]) {
				dates = append(dates, td)
			}
			break
		}
		dates = append(dates, end)
		m++
		if m > 12 {
			m = 1
			y++
		}
	}
	// units as of each date, per asset, accumulated in one pass over date-sorted transactions
	txs := make([]store.StoredTransaction, 0, len(p.Transactions))
	for _, t := range p.Transactions {
		if _, ok := included[t.AssetID]; ok && t.Units != nil {
			txs = append(txs, t)
		}
	}
	sort.Slice(txs, func(i, j int) bool { return txs[i].Date < txs[j].Date })
	units := map[string]float64{}
	ti := 0
	out := make([]CompositionPoint, 0, len(dates))
	for _, d := range dates {
		ds := d.Format(dateLayout)
		for ti < len(txs) && txs[ti].Date <= ds {
			units[txs[ti].AssetID] += *txs[ti].Units
			ti++
		}
		pt := CompositionPoint{Month: ds[:7], Date: ds, Sectors: map[string]float64{}, OtherBy: map[string]float64{}, NoDataBy: map[string]float64{}}
		otherBy := map[string]float64{}
		noDataBy := map[string]float64{}
		var total, noData, est float64
		sec := map[string]float64{}
		for id := range included {
			u := units[id]
			if u <= 0.0001 {
				continue
			}
			price, ok := p.PriceAsOf(id, ds)
			if !ok {
				continue
			}
			v := u * price
			total += v
			name := included[id].Name
			mix, src := MarketCapMixFor(p, id, name, pt.Month)
			if src == "name" || src == "entered" {
				// no published figures for this holding: it still counts (same rule as the donut) but is reported as such
				noData += v
				why := name
				if strings.TrimSpace(included[id].ISIN) == "" {
					why += " (no ISIN stored)"
				}
				noDataBy[why] += v
			} else if src == "estimated" {
				est += v
			}
			for label, w := range mix {
				x := v * w * 100 // percent-of-value units, matching the factor below
				switch label {
				case "Large Cap":
					pt.Large += x
				case "Mid Cap":
					pt.Mid += x
				case "Small Cap":
					pt.Small += x
				case "Unclassified":
					pt.Unclassified += x
				case "International Equity":
					pt.Foreign += x
				case "Debt":
					pt.Debt += x
				case "Cash":
					pt.Cash += x
				case "Others":
					pt.Other += x
					otherBy[name] += x
				default:
					if pt.Extra == nil {
						pt.Extra = map[string]float64{}
					}
					pt.Extra[label] += x
				}
			}
			h, has := p.FundHistoryFor(id)
			if !has {
				continue
			}
			m, _, found := fundMonthFor(h, pt.Month)
			if !found {
				continue
			}
			for k, x := range m.Sectors {
				sec[k] += v * x
			}
		}
		pt.Value = total
		if total > 0 {
			f := 1 / total // fractions of value -> percent below (splits are already in percent, so *1/total gives percent)
			pt.Large *= f
			pt.Mid *= f
			pt.Small *= f
			pt.Other *= f
			pt.Unclassified *= f
			pt.Foreign *= f
			for k := range pt.Extra {
				pt.Extra[k] *= f
			}
			pt.Debt *= f
			pt.Cash *= f
			pt.NoData = noData / total * 100
			pt.Estimated = est / total * 100
			for k, x := range otherBy {
				if x*f >= 0.5 {
					pt.OtherBy[k] = x * f
				}
			}
			for k, x := range noDataBy {
				if x/total*100 >= 0.5 {
					pt.NoDataBy[k] = x / total * 100
				}
			}
			for k, x := range sec {
				if x*f >= 0.05 {
					pt.Sectors[k] = x * f
				}
			}
		}
		out = append(out, pt)
	}
	return out
}

// ComputeFundComposition is the month-by-month mix of ONE fund (owned or only tracked), straight from its stored
// published history - no holdings or prices involved. Value is 0 (not applicable); Estimated is 100 for an estimated month.
func ComputeFundComposition(p *store.Portfolio, assetID string) []CompositionPoint {
	h, ok := p.FundHistoryFor(assetID)
	if !ok {
		return []CompositionPoint{}
	}
	out := make([]CompositionPoint, 0, len(h.Months))
	for _, m := range h.Months {
		if len(m.Split) < 8 {
			continue
		}
		pt := CompositionPoint{
			Month: m.Month, Date: m.Month + "-01",
			Large: m.Split[0], Mid: m.Split[1], Small: m.Split[2], Unclassified: m.Split[3],
			Foreign: m.Split[4], Debt: m.Split[5], Cash: m.Split[6], Other: m.Split[7],
			Sectors: m.Sectors, OtherBy: map[string]float64{}, NoDataBy: map[string]float64{},
		}
		if pt.Sectors == nil {
			pt.Sectors = map[string]float64{}
		}
		if m.Estimated {
			pt.Estimated = 100
		}
		out = append(out, pt)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Month < out[j].Month })
	return out
}

// PublishedSplitsByAsset returns, per asset, the 8-way split (large, mid, small, unclassified, foreign, debt, cash, other)
// from its published fund history for the month containing `now` (or the latest earlier one - fundMonthFor). A
// cap composition the person entered by hand that is NEWER than that published month wins (their own, more recent
// figure), so those assets are left out and the caller falls back to the entered composition.
func PublishedSplitsByAsset(p *store.Portfolio, now time.Time) map[string][]float64 {
	out := map[string][]float64{}
	month := now.Format("2006-01")
	for i := range p.FundHistories {
		h := &p.FundHistories[i]
		m, _, ok := fundMonthFor(h, month)
		if !ok || len(m.Split) < 8 {
			continue
		}
		if c, has := p.GetCapComposition(h.AssetID); has && len(c.AsOf) >= 7 && c.AsOf[:7] > m.Month {
			continue
		}
		out[h.AssetID] = m.Split
	}
	return out
}

// The eight published buckets, as the labels every market-cap view in the app uses.
var capLabels = [8]string{"Large Cap", "Mid Cap", "Small Cap", "Unclassified", "International Equity", "Debt", "Cash", "Others"}

// MarketCapMixFor is THE single place that decides how one holding splits across market-cap labels for a month
// ("YYYY-MM"). Every screen (dashboard donut, Allocation donut, target drift, holdings-by-segment, Over Time) calls it,
// so the same number appears everywhere. Order of precedence:
//  1. the fund's published monthly disclosure (month, else the latest earlier month) - unless a cap composition the
//     person entered by hand is NEWER than that published month (and not after `month`), which then wins;
//  2. an entered cap composition;
//  3. a guess from the fund's name (GuessMarketCapSplit).
//
// Returns fractions (sum ~1) by label and the source: "published", "estimated" (a published month that is itself an
// estimate or carried 2+ months), "entered" or "name".
func MarketCapMixFor(p *store.Portfolio, assetID, assetName, month string) (map[string]float64, string) {
	manual, hasManual := p.GetCapComposition(assetID)
	if h, ok := p.FundHistoryFor(assetID); ok {
		if m, flagged, found := fundMonthFor(h, month); found && len(m.Split) >= 8 {
			manualWins := hasManual && len(manual.AsOf) >= 7 && manual.AsOf[:7] > m.Month && manual.AsOf[:7] <= month
			if !manualWins {
				out := map[string]float64{}
				for i, w := range m.Split[:8] {
					if w > 0 {
						out[capLabels[i]] = w / 100
					}
				}
				if flagged {
					return out, "estimated"
				}
				return out, "published"
			}
		}
	}
	if hasManual {
		if sum := manual.Large + manual.Mid + manual.Small + manual.Cash; sum > 0 {
			out := map[string]float64{}
			for k, v := range map[string]float64{"Large Cap": manual.Large, "Mid Cap": manual.Mid, "Small Cap": manual.Small, "Cash": manual.Cash} {
				if v > 0 {
					out[k] = v / sum
				}
			}
			return out, "entered"
		}
	}
	return GuessMarketCapSplit(assetName), "name"
}

// AllocationByMarketCapMix is the market-cap donut: every priced holding's current value spread over the labels
// MarketCapMixFor gives it.
func AllocationByMarketCapMix(p *store.Portfolio, holdings []Holding, now time.Time) []AllocationSlice {
	month := now.Format("2006-01")
	totals := map[string]float64{}
	var total float64
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		mix, _ := MarketCapMixFor(p, h.AssetID, h.AssetName, month)
		for k, w := range mix {
			totals[k] += h.CurrentValue * w
		}
		total += h.CurrentValue
	}
	return toSlices(totals, total)
}

// HoldingsInMarketCapSegment lists the holdings that put any value in the given label, by the same rule as the donut.
func HoldingsInMarketCapSegment(p *store.Portfolio, holdings []Holding, label string, now time.Time) []Holding {
	month := now.Format("2006-01")
	var out []Holding
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		if mix, _ := MarketCapMixFor(p, h.AssetID, h.AssetName, month); mix[label] > 0 {
			out = append(out, h)
		}
	}
	return out
}
