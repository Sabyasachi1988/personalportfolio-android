package store

import "testing"

func TestMergeFundMonthsGuards(t *testing.T) {
	p := &Portfolio{}
	s := []float64{40, 30, 25, 0, 0, 0, 5, 0}
	st := p.MergeFundMonths("a", "1", "v1", "d", []FundMonth{{Month: "2025-01", Split: s}, {Month: "2025-02", Split: s}})
	if st.Added != 2 {
		t.Fatalf("added %d", st.Added)
	}
	// estimated never replaces published; manual never replaced; changed published replaces
	est := []float64{1, 1, 1, 0, 0, 0, 97, 0}
	p.FundHistories[0].Months[1].Source = "MANUAL"
	chg := []float64{41, 30, 24, 0, 0, 0, 5, 0}
	st = p.MergeFundMonths("a", "1", "v2", "d", []FundMonth{
		{Month: "2025-01", Split: est, Estimated: true},
		{Month: "2025-02", Split: chg},
		{Month: "2025-03", Split: chg},
	})
	if st.Kept != 2 || st.Added != 1 || st.Updated != 0 {
		t.Fatalf("stats %+v", st)
	}
	st = p.MergeFundMonths("a", "1", "v3", "d", []FundMonth{{Month: "2025-03", Split: s}})
	if st.Updated != 1 {
		t.Fatalf("expected revision, %+v", st)
	}
	if len(p.FundHistories[0].Months) != 3 {
		t.Fatal("history must be append-only")
	}
}

func TestUpsertPricesGuarded(t *testing.T) {
	p := &Portfolio{Prices: []PriceRecord{{AssetID: "a", Date: "2025-01-01", Price: 100, Source: "MANUAL"}, {AssetID: "a", Date: "2025-01-02", Price: 100, Source: "AMFI"}, {AssetID: "a", Date: "2025-01-03", Price: 100, Source: "AMFI"}}}
	st := p.UpsertPricesGuarded([]PriceRecord{
		{AssetID: "a", Date: "2025-01-01", Price: 101, Source: "T"},
		{AssetID: "a", Date: "2025-01-02", Price: 101, Source: "T"},
		{AssetID: "a", Date: "2025-01-03", Price: 150, Source: "T"},
		{AssetID: "a", Date: "2025-01-04", Price: 99, Source: "T"},
	})
	if st.Conflicts != 2 || st.Updated != 1 || st.Added != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestMergeLookThroughReplacesFofOther(t *testing.T) {
	p := &Portfolio{}
	other := []float64{0, 0, 0, 0, 0, 0, 0, 100}
	p.MergeFundMonths("a", "1", "v", "d", []FundMonth{{Month: "2025-01", Split: other}})
	st := p.MergeFundMonths("a", "1", "v2", "d", []FundMonth{{Month: "2025-01", Split: []float64{95, 5, 0, 0, 0, 0, 0, 0}, Estimated: true}})
	if st.Updated != 1 || p.FundHistories[0].Months[0].Split[0] != 95 {
		t.Fatalf("%+v", st)
	}
}
