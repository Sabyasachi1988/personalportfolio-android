package finance

import (
	"sort"
	"time"

	"ledger/internal/store"
)

// CompositionPoint is the portfolio's market-cap / sector mix at one month-end, as percent of the
// portfolio's total value that day. Fund splits come from each fund's published month; the fund's
// units on that date (from transactions) times its NAV give the weights.
type CompositionPoint struct {
	Month     string
	Date      string
	Value     float64
	Large     float64
	Mid       float64
	Small     float64
	Cash      float64
	Debt      float64
	Foreign   float64
	Other     float64 // unclassified equity + other instruments
	NoData    float64 // value held in funds with no published split for that month
	Estimated float64 // share of value whose month was estimated or carried from another month
	Sectors   map[string]float64
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
		return *before, true, true
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
		pt := CompositionPoint{Month: ds[:7], Date: ds, Sectors: map[string]float64{}}
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
			h, has := p.FundHistoryFor(id)
			if !has {
				noData += v
				continue
			}
			m, flagged, found := fundMonthFor(h, pt.Month)
			if !found || len(m.Split) < 8 {
				noData += v
				continue
			}
			if flagged {
				est += v
			}
			pt.Large += v * m.Split[0]
			pt.Mid += v * m.Split[1]
			pt.Small += v * m.Split[2]
			pt.Other += v * (m.Split[3] + m.Split[7])
			pt.Foreign += v * m.Split[4]
			pt.Debt += v * m.Split[5]
			pt.Cash += v * m.Split[6]
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
			pt.Foreign *= f
			pt.Debt *= f
			pt.Cash *= f
			pt.NoData = noData / total * 100
			pt.Estimated = est / total * 100
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
