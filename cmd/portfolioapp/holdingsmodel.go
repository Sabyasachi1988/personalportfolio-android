package main

import (
	"sort"

	"ledger/internal/finance"

	"github.com/lxn/walk"
)

// HoldingsModel is a sortable walk.TableModel backing the Portfolio tab's
// table view.
type HoldingsModel struct {
	walk.TableModelBase
	walk.SorterBase
	sortColumn int
	sortOrder  walk.SortOrder
	items      []finance.Holding
}

func NewHoldingsModel() *HoldingsModel {
	return &HoldingsModel{sortColumn: 0, sortOrder: walk.SortAscending}
}

func (m *HoldingsModel) RowCount() int {
	return len(m.items)
}

// Column order must match the TableViewColumn list in portfolioPage.
func (m *HoldingsModel) Value(row, col int) interface{} {
	h := m.items[row]
	switch col {
	case 0:
		return h.AssetName
	case 1:
		return h.AccountName
	case 2:
		return h.UnitsHeld
	case 3:
		return h.NetInvested
	case 4:
		if h.HasPrice {
			return h.CurrentPrice
		}
		return nil
	case 5:
		if h.HasPrice {
			return h.CurrentValue
		}
		return nil
	case 6:
		if h.HasPrice {
			return h.Gain
		}
		return nil
	case 7:
		if h.HasPrice {
			return h.GainPercent
		}
		return nil
	case 8:
		if h.HasXIRR {
			return h.XIRR
		}
		return nil
	}
	panic("unexpected column")
}

func (m *HoldingsModel) Sort(col int, order walk.SortOrder) error {
	m.sortColumn, m.sortOrder = col, order

	sort.SliceStable(m.items, func(i, j int) bool {
		a, b := m.items[i], m.items[j]
		asc := func(less bool) bool {
			if m.sortOrder == walk.SortAscending {
				return less
			}
			return !less
		}
		switch m.sortColumn {
		case 0:
			return asc(a.AssetName < b.AssetName)
		case 1:
			return asc(a.AccountName < b.AccountName)
		case 2:
			return asc(a.UnitsHeld < b.UnitsHeld)
		case 3:
			return asc(a.NetInvested < b.NetInvested)
		case 4:
			return asc(a.CurrentPrice < b.CurrentPrice)
		case 5:
			return asc(a.CurrentValue < b.CurrentValue)
		case 6:
			return asc(a.Gain < b.Gain)
		case 7:
			return asc(a.GainPercent < b.GainPercent)
		case 8:
			return asc(a.XIRR < b.XIRR)
		}
		return false
	})

	return m.SorterBase.Sort(col, order)
}

// SetItems replaces the model's data and refreshes the table.
func (m *HoldingsModel) SetItems(items []finance.Holding) {
	m.items = items
	m.PublishRowsReset()
	_ = m.Sort(m.sortColumn, m.sortOrder)
}
