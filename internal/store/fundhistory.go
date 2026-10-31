package store

import "math"

// FundMonth is one fund's published month-end portfolio composition: the
// large/mid/small/cash... split (percent of NAV) and the sector split, taken from
// the fund house's monthly disclosure and the AMFI cap lists (see the fund-data repo).
type FundMonth struct {
	Month     string             // "YYYY-MM"
	Split     []float64          // large, mid, small, unclassified, foreign, debt, cash, other (percent of NAV)
	Sectors   map[string]float64 // sector display name -> percent of NAV
	Estimated bool               // true when the month was filled from a neighbouring month rather than a disclosure
	Source    string             // "FUND_DATA" (published) or "MANUAL" (typed in; never overwritten by a sync)
}

// FundHistory holds the cap/sector history of one asset's fund. Pid is the fund-data portfolio id
// (empty when the fund isn't covered); Version is the fund-data manifest version this was last synced to.
type FundHistory struct {
	AssetID  string
	Pid      string
	Version  string
	SyncedAt string
	Note     string // plain-language result of the last cap/sector sync (what was added / kept)
	NavNote  string // plain-language result of the last NAV pull (new days, kept-on-conflict)
	Months   []FundMonth
}

// FundSyncStats describes what a merge did, for the user-visible note.
type FundSyncStats struct {
	Added, Updated, Kept int
}

func (p *Portfolio) FundHistoryFor(assetID string) (*FundHistory, bool) {
	for i := range p.FundHistories {
		if p.FundHistories[i].AssetID == assetID {
			return &p.FundHistories[i], true
		}
	}
	return nil, false
}

// MergeFundMonths merges incoming months into the asset's history without ever losing better data:
//   - a MANUAL month is never replaced;
//   - a published month is never replaced by an estimated one;
//   - otherwise a month is added when new and replaced only when the values actually differ.
//
// Months already stored but absent from the incoming set are kept (history is append-only).
func (p *Portfolio) MergeFundMonths(assetID, pid, version, syncedAt string, incoming []FundMonth) FundSyncStats {
	h, ok := p.FundHistoryFor(assetID)
	if !ok {
		p.FundHistories = append(p.FundHistories, FundHistory{AssetID: assetID, Months: []FundMonth{}})
		h = &p.FundHistories[len(p.FundHistories)-1]
	}
	h.Pid, h.Version, h.SyncedAt = pid, version, syncedAt
	idx := make(map[string]int, len(h.Months))
	for i, m := range h.Months {
		idx[m.Month] = i
	}
	var st FundSyncStats
	for _, in := range incoming {
		if in.Source == "" {
			in.Source = "FUND_DATA"
		}
		i, exists := idx[in.Month]
		if !exists {
			h.Months = append(h.Months, in)
			idx[in.Month] = len(h.Months) - 1
			st.Added++
			continue
		}
		cur := h.Months[i]
		// An estimated month never replaces a published one - except a fund-of-funds month that is ~100% "other"
		// (only the underlying unit line was visible), which the data side replaces by a look-through to the tracked index.
		lookThrough := in.Estimated && !cur.Estimated && len(cur.Split) >= 8 && len(in.Split) >= 8 && cur.Split[7] >= 90 && in.Split[7] < 90
		if cur.Source == "MANUAL" || (in.Estimated && !cur.Estimated && !lookThrough) || sameMonth(cur, in) {
			st.Kept++
			continue
		}
		h.Months[i] = in
		st.Updated++
	}
	sortMonths(h.Months)
	return st
}

func sameMonth(a, b FundMonth) bool {
	if a.Estimated != b.Estimated || len(a.Split) != len(b.Split) {
		return false
	}
	for i := range a.Split {
		if math.Abs(a.Split[i]-b.Split[i]) > 0.005 {
			return false
		}
	}
	if len(a.Sectors) != len(b.Sectors) {
		return false
	}
	for k, v := range a.Sectors {
		if w, ok := b.Sectors[k]; !ok || math.Abs(v-w) > 0.005 {
			return false
		}
	}
	return true
}

func sortMonths(ms []FundMonth) {
	for i := 1; i < len(ms); i++ {
		for j := i; j > 0 && ms[j].Month < ms[j-1].Month; j-- {
			ms[j], ms[j-1] = ms[j-1], ms[j]
		}
	}
}

// PriceMergeStats describes a guarded NAV merge.
type PriceMergeStats struct {
	Added, Updated, Same, Conflicts int
}

// UpsertPricesGuarded merges fetched prices with guard rails. A fetched price for a date that already
// has one is only allowed to replace it when the stored one is not MANUAL and the two agree within 2%
// (a tiny revision). A larger disagreement keeps the stored value and is counted as a conflict, so a bad
// or differently-adjusted source can never silently rewrite history you already have.
func (p *Portfolio) UpsertPricesGuarded(records []PriceRecord) PriceMergeStats {
	index := make(map[string]int, len(p.Prices))
	for i, r := range p.Prices {
		index[r.AssetID+"|"+r.Date] = i
	}
	var st PriceMergeStats
	for _, r := range records {
		if r.Price <= 0 {
			continue
		}
		key := r.AssetID + "|" + r.Date
		i, exists := index[key]
		if !exists {
			p.Prices = append(p.Prices, r)
			index[key] = len(p.Prices) - 1
			st.Added++
			continue
		}
		cur := p.Prices[i]
		rel := math.Abs(cur.Price-r.Price) / math.Max(cur.Price, 1e-9)
		switch {
		case rel < 1e-6:
			st.Same++
		case cur.Source == "MANUAL" || rel > 0.02:
			st.Conflicts++
		default:
			p.Prices[i] = r
			st.Updated++
		}
	}
	p.invalidatePriceIndex()
	return st
}

// EnsureFundHistory returns the asset's FundHistory, creating an empty one if needed.
func (p *Portfolio) EnsureFundHistory(assetID string) *FundHistory {
	if h, ok := p.FundHistoryFor(assetID); ok {
		return h
	}
	p.FundHistories = append(p.FundHistories, FundHistory{AssetID: assetID, Months: []FundMonth{}})
	return &p.FundHistories[len(p.FundHistories)-1]
}
