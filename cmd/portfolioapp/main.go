// Command portfolioapp is the real PersonalPortfolio app: Member/Account/
// Asset/Transaction management, CAS PDF import (MFCentral format), price
// lookups (AMFI + Yahoo Finance), and backup/reset - in one window.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"ledger/internal/casimport"
	"ledger/internal/finance"
	"ledger/internal/priceapi"
	"ledger/internal/store"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"github.com/ledongthuc/pdf"
)

var (
	portfolio     *store.Portfolio
	portfolioPath string
	mw            *walk.MainWindow
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("%v\n\n%s", r, debug.Stack())
			walk.MsgBox(nil, "Startup crashed (please copy this text)", msg, walk.MsgBoxIconError)
		}
	}()

	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	appDir := filepath.Join(dir, "PersonalPortfolio")
	_ = os.MkdirAll(appDir, 0755)
	portfolioPath = filepath.Join(appDir, "portfolio.json")

	p, err := store.Load(portfolioPath)
	if err != nil {
		// An incompatible/corrupt file shouldn't block the app from
		// starting - back it up out of the way and start fresh, but tell
		// the person clearly so they know their old file wasn't silently
		// discarded.
		backupPath := portfolioPath + ".unreadable-" + timeStamp()
		if renameErr := os.Rename(portfolioPath, backupPath); renameErr == nil {
			walk.MsgBox(nil, "Portfolio file could not be read",
				fmt.Sprintf("The existing portfolio file couldn't be loaded:\n\n%v\n\nIt has been moved to:\n%s\n\nStarting with an empty portfolio.", err, backupPath),
				walk.MsgBoxIconWarning)
		} else {
			walk.MsgBox(nil, "Portfolio file could not be read",
				fmt.Sprintf("%v\n\nStarting with an empty portfolio. The unreadable file was left in place at:\n%s", err, portfolioPath),
				walk.MsgBoxIconWarning)
		}
		p = &store.Portfolio{}
	}
	portfolio = p

	MainWindow{
		AssignTo: &mw,
		Title:    "PersonalPortfolio",
		MinSize:  Size{Width: 900, Height: 650},
		Layout:   VBox{},
		Children: []Widget{
			TabWidget{
				Pages: []TabPage{
					portfolioPage(),
					allocationPage(),
					memberPage(),
					accountPage(),
					assetPage(),
					transactionPage(),
					importPage(),
					pricePage(),
					backupPage(),
				},
			},
		},
	}.Create()

	mw.Run()
}

func timeStamp() string {
	return time.Now().Format("20060102-150405")
}

// safeRecover, called via defer at the top of every event handler, turns
// a panic into a visible error dialog (with the message and a stack
// trace) instead of silently killing the whole app. This exists purely
// for diagnosis: if something is going to crash, we want to see why.
func safeRecover() {
	if r := recover(); r != nil {
		msg := fmt.Sprintf("%v\n\n%s", r, debug.Stack())
		walk.MsgBox(nil, "Something went wrong (please copy this text)", msg, walk.MsgBoxIconError)
	}
}

func saveOrWarn(mw *walk.MainWindow) {
	if err := store.Save(portfolioPath, portfolio); err != nil {
		walk.MsgBox(mw, "Could not save", err.Error(), walk.MsgBoxIconError)
	}
}

func promptText(owner walk.Form, title, label, initial string) (string, bool) {
	var dlg *walk.Dialog
	var edit *walk.LineEdit
	var result string
	var ok bool

	Dialog{
		AssignTo: &dlg,
		Title:    title,
		MinSize:  Size{Width: 350, Height: 120},
		Layout:   VBox{},
		Children: []Widget{
			Label{Text: label},
			LineEdit{AssignTo: &edit, Text: initial},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					HSpacer{},
					PushButton{
						Text: "OK",
						OnClicked: func() {
							defer safeRecover()
							result = edit.Text()
							ok = true
							dlg.Accept()
						},
					},
					PushButton{
						Text: "Cancel",
						OnClicked: func() {
							defer safeRecover()
							dlg.Cancel()
						},
					},
				},
			},
		},
	}.Run(owner)

	return result, ok
}

// ---------- Portfolio tab ----------

func portfolioPage() TabPage {
	var summaryLabel *walk.Label
	var xirrLabel *walk.Label
	var table *walk.TableView
	var memberFilterCombo *walk.ComboBox
	model := NewHoldingsModel()
	var lastFilteredHoldings []finance.Holding
	var lastInvested, lastValue float64
	var filterMemberIDs []string // index-aligned with the combo box; "" means "All"

	refresh := func() {
		defer safeRecover()
		allHoldings := finance.ComputeHoldings(portfolio)

		// Rebuild the member-filter combo's options (All (family) + each
		// member), preserving the current selection where possible.
		prevIdx := memberFilterCombo.CurrentIndex()
		var prevID string
		if prevIdx >= 0 && prevIdx < len(filterMemberIDs) {
			prevID = filterMemberIDs[prevIdx]
		}
		names := []string{"All (family)"}
		filterMemberIDs = []string{""}
		for _, m := range portfolio.Members {
			names = append(names, m.Name)
			filterMemberIDs = append(filterMemberIDs, m.ID)
		}
		memberFilterCombo.SetModel(names)
		newIdx := 0
		for i, id := range filterMemberIDs {
			if id == prevID {
				newIdx = i
				break
			}
		}
		memberFilterCombo.SetCurrentIndex(newIdx)

		selectedMemberID := filterMemberIDs[newIdx]
		holdings := finance.FilterHoldingsByMember(allHoldings, selectedMemberID)
		invested, value, anyPriced := finance.PortfolioTotals(holdings)
		lastFilteredHoldings, lastInvested, lastValue = holdings, invested, value

		if len(holdings) == 0 {
			summaryLabel.SetText("No holdings yet. Import a CAS PDF or add transactions.")
			xirrLabel.SetText("")
			model.SetItems(nil)
			return
		}
		if anyPriced {
			gain := value - invested
			gainPct := 0.0
			if invested != 0 {
				gainPct = gain / invested * 100
			}
			summaryLabel.SetText(fmt.Sprintf("Total invested: %.2f   Current value: %.2f   Gain: %.2f (%.2f%%)   [priced holdings only]",
				invested, value, gain, gainPct))
			if rate, ok := finance.PortfolioXIRR(portfolio, holdings); ok {
				xirrLabel.SetText(fmt.Sprintf("Cumulative XIRR: %.2f%%", rate))
			} else {
				xirrLabel.SetText("Cumulative XIRR: not computable yet (need both an investment and a redemption or current value)")
			}
		} else {
			summaryLabel.SetText("No current prices yet - click 'Refresh Prices from AMFI' below, or enter manual prices on the Price tab.")
			xirrLabel.SetText("")
		}
		model.SetItems(holdings)
	}

	return TabPage{
		Title:  "Portfolio",
		Layout: VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Show:"},
					ComboBox{AssignTo: &memberFilterCombo, OnCurrentIndexChanged: func() { defer safeRecover(); refresh() }},
					HSpacer{},
				},
			},
			Label{AssignTo: &summaryLabel, Text: "Loading..."},
			Label{AssignTo: &xirrLabel, Text: ""},
			TableView{
				AssignTo:         &table,
				Model:            model,
				AlternatingRowBG: true,
				ColumnsOrderable: true,
				Columns: []TableViewColumn{
					{Title: "Fund", Width: 260},
					{Title: "Account", Width: 160},
					{Title: "Units", Width: 90, Format: "%.4f", Alignment: AlignFar},
					{Title: "Invested", Width: 100, Format: "%.2f", Alignment: AlignFar},
					{Title: "Price", Width: 90, Format: "%.4f", Alignment: AlignFar},
					{Title: "Value", Width: 100, Format: "%.2f", Alignment: AlignFar},
					{Title: "Gain", Width: 100, Format: "%.2f", Alignment: AlignFar},
					{Title: "Gain %", Width: 80, Format: "%.2f%%", Alignment: AlignFar},
					{Title: "XIRR %", Width: 80, Format: "%.2f%%", Alignment: AlignFar},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						Text: "Refresh Prices from AMFI (mutual funds)",
						OnClicked: func() {
							defer safeRecover()
							records, err := priceapi.FetchAmfiNav()
							if err != nil {
								walk.MsgBox(mw, "Could not fetch AMFI prices", err.Error(), walk.MsgBoxIconError)
								return
							}
							byISIN := make(map[string]priceapi.NavRecord, len(records))
							for _, r := range records {
								// AMFI lists many Plan/Option variants per ISIN;
								// last one wins, which in practice means whichever
								// sorts last in the file - fine for now since we
								// match by ISIN, which should already be specific
								// to one Plan/Option (the one from your CAS import).
								if r.ISINPayout != "" {
									byISIN[r.ISINPayout] = r
								}
								if r.ISINReinvest != "" {
									byISIN[r.ISINReinvest] = r
								}
							}
							updated := 0
							for i := range portfolio.Assets {
								a := &portfolio.Assets[i]
								if a.ISIN == "" {
									continue
								}
								rec, ok := byISIN[a.ISIN]
								if !ok {
									continue
								}
								portfolio.Prices = append(portfolio.Prices, store.PriceRecord{
									AssetID: a.ID, Date: amfiDateToISO(rec.Date), Price: rec.NAV, Source: "AMFI",
								})
								if rec.AssetClass != "" {
									a.AssetClass = rec.AssetClass
								}
								updated++
							}
							saveOrWarn(mw)
							refresh()
							walk.MsgBox(mw, "Prices updated", fmt.Sprintf("%d of %d asset(s) matched by ISIN and updated.", updated, len(portfolio.Assets)), walk.MsgBoxIconInformation)
						},
					},
					PushButton{
						Text: "Open Interactive Report (charts, in browser)",
						OnClicked: func() {
							defer safeRecover()
							if len(lastFilteredHoldings) == 0 {
								walk.MsgBox(mw, "Nothing to show", "No holdings yet.", walk.MsgBoxIconInformation)
								return
							}
							classByAsset := make(map[string]string, len(portfolio.Assets))
							for _, a := range portfolio.Assets {
								classByAsset[a.ID] = a.AssetClass
							}
							var xirr float64
							var hasXIRR bool
							if rate, ok := finance.PortfolioXIRR(portfolio, lastFilteredHoldings); ok {
								xirr, hasXIRR = rate, true
							}
							compByAsset := make(map[string]store.CapComposition, len(portfolio.CapCompositions))
							for _, c := range portfolio.CapCompositions {
								compByAsset[c.AssetID] = c
							}
							path, err := generateHTMLReport(lastFilteredHoldings, lastInvested, lastValue, classByAsset, compByAsset, xirr, hasXIRR)
							if err != nil {
								walk.MsgBox(mw, "Could not generate report", err.Error(), walk.MsgBoxIconError)
								return
							}
							if err := openInBrowser(path); err != nil {
								walk.MsgBox(mw, "Report saved but could not open automatically",
									fmt.Sprintf("Saved to:\n%s\n\n%v", path, err), walk.MsgBoxIconWarning)
							}
						},
					},
				},
			},
		},
		OnSizeChanged: func() { defer safeRecover(); refresh() },
	}
}

// amfiDateToISO converts AMFI's "20-Aug-2026" style date to "2026-08-20".
// Falls back to today's date string if parsing fails, since a price
// record needs *some* sortable date and silently dropping the price
// entirely would be worse.
func amfiDateToISO(s string) string {
	t, err := time.Parse("02-Jan-2006", strings.TrimSpace(s))
	if err != nil {
		return time.Now().Format(dateISOLayout)
	}
	return t.Format(dateISOLayout)
}

const dateISOLayout = "2006-01-02"

// ---------- Allocation tab ----------

// allocationRow tracks the live widgets for one fund's editable row, so
// the Save handler can read current field values without re-walking the
// widget tree.
type allocationRow struct {
	assetID                                 string
	largeEdit, midEdit, smallEdit, cashEdit *walk.LineEdit
	statusLabel                             *walk.Label
}

func allocationPage() TabPage {
	var scroll *walk.ScrollView
	var rows []allocationRow
	var rebuilding bool
	var lastSignature string

	rebuild := func() {
		defer safeRecover()
		if rebuilding {
			return // OnSizeChanged can re-fire while a rebuild is already
			// in progress (adding widgets itself triggers layout/size
			// events) - without this guard, that reentrancy is exactly
			// what caused rows/controls to multiply.
		}

		holdings := finance.ComputeHoldings(portfolio)
		heldAssetIDs := make(map[string]bool, len(holdings))
		for _, h := range holdings {
			heldAssetIDs[h.AssetID] = true
		}

		// Skip the (expensive, widget-churning) rebuild entirely if
		// nothing that would change the row set has actually changed
		// since last time - OnSizeChanged fires far more often than the
		// held-asset list does.
		var sigParts []string
		for _, a := range portfolio.Assets {
			if heldAssetIDs[a.ID] {
				sigParts = append(sigParts, a.ID)
			}
		}
		signature := strings.Join(sigParts, ",")
		if signature == lastSignature && len(rows) > 0 {
			return
		}
		lastSignature = signature

		rebuilding = true
		defer func() { rebuilding = false }()

		scroll.SetSuspended(true)
		defer scroll.SetSuspended(false)

		// Children().Clear() only removes walk's own bookkeeping - it does
		// NOT destroy the underlying native controls, which otherwise
		// stay alive and visible underneath whatever gets built next.
		// Dispose() each one explicitly first.
		for i := 0; i < scroll.Children().Len(); i++ {
			scroll.Children().At(i).Dispose()
		}
		scroll.Children().Clear()
		rows = nil

		classByAsset := make(map[string]string, len(portfolio.Assets))
		for _, a := range portfolio.Assets {
			classByAsset[a.ID] = a.AssetClass
		}

		header, _ := walk.NewComposite(scroll)
		header.SetLayout(walk.NewHBoxLayout())
		mkLabel := func(parent walk.Container, text string, width int) {
			l, _ := walk.NewLabel(parent)
			l.SetText(text)
			l.SetMinMaxSizePixels(walk.Size{Width: width}, walk.Size{})
		}
		mkLabel(header, "Fund", 300)
		mkLabel(header, "Large %", 65)
		mkLabel(header, "Mid %", 65)
		mkLabel(header, "Small %", 65)
		mkLabel(header, "Cash %", 65)
		mkLabel(header, "As of / source", 200)

		for _, a := range portfolio.Assets {
			if !heldAssetIDs[a.ID] {
				continue
			}
			if finance.EffectiveAssetClass(classByAsset[a.ID], a.Name) != "Equity" {
				continue
			}

			large, mid, small, cash := 0.0, 0.0, 0.0, 0.0
			statusText := "not yet entered"
			if comp, ok := portfolio.GetCapComposition(a.ID); ok {
				large, mid, small, cash = comp.Large, comp.Mid, comp.Small, comp.Cash
				statusText = fmt.Sprintf("as of %s (%s)", comp.AsOf, comp.Source)
			} else {
				switch finance.GuessMarketCapSegment(a.Name) {
				case "Large Cap":
					large = 100
				case "Mid Cap":
					mid = 100
				case "Small Cap":
					small = 100
				default:
					statusText = "not yet entered - enter from the fund's factsheet"
				}
			}

			rowComposite, _ := walk.NewComposite(scroll)
			rowComposite.SetLayout(walk.NewHBoxLayout())

			nameLabel, _ := walk.NewLabel(rowComposite)
			nameLabel.SetText(a.Name)
			nameLabel.SetMinMaxSizePixels(walk.Size{Width: 300}, walk.Size{})

			largeEdit, _ := walk.NewLineEdit(rowComposite)
			largeEdit.SetText(fmt.Sprintf("%.2f", large))
			largeEdit.SetMinMaxSizePixels(walk.Size{Width: 65}, walk.Size{})

			midEdit, _ := walk.NewLineEdit(rowComposite)
			midEdit.SetText(fmt.Sprintf("%.2f", mid))
			midEdit.SetMinMaxSizePixels(walk.Size{Width: 65}, walk.Size{})

			smallEdit, _ := walk.NewLineEdit(rowComposite)
			smallEdit.SetText(fmt.Sprintf("%.2f", small))
			smallEdit.SetMinMaxSizePixels(walk.Size{Width: 65}, walk.Size{})

			cashEdit, _ := walk.NewLineEdit(rowComposite)
			cashEdit.SetText(fmt.Sprintf("%.2f", cash))
			cashEdit.SetMinMaxSizePixels(walk.Size{Width: 65}, walk.Size{})

			statusLabel, _ := walk.NewLabel(rowComposite)
			statusLabel.SetText(statusText)
			statusLabel.SetMinMaxSizePixels(walk.Size{Width: 200}, walk.Size{})

			assetID := a.ID
			rows = append(rows, allocationRow{assetID: assetID, largeEdit: largeEdit, midEdit: midEdit, smallEdit: smallEdit, cashEdit: cashEdit, statusLabel: statusLabel})

			saveBtn, _ := walk.NewPushButton(rowComposite)
			saveBtn.SetText("Save")
			saveBtn.Clicked().Attach(func() {
				defer safeRecover()
				l, errL := strconv.ParseFloat(strings.TrimSpace(largeEdit.Text()), 64)
				m, errM := strconv.ParseFloat(strings.TrimSpace(midEdit.Text()), 64)
				s, errS := strconv.ParseFloat(strings.TrimSpace(smallEdit.Text()), 64)
				c, errC := strconv.ParseFloat(strings.TrimSpace(cashEdit.Text()), 64)
				if errL != nil || errM != nil || errS != nil || errC != nil {
					walk.MsgBox(mw, "Invalid number", "Large/Mid/Small/Cash must all be numbers.", walk.MsgBoxIconError)
					return
				}
				asOf := time.Now().Format(dateISOLayout)
				portfolio.SetCapComposition(assetID, l, m, s, c, asOf, "Manual entry")
				statusLabel.SetText(fmt.Sprintf("as of %s (Manual entry)", asOf))
				saveOrWarn(mw)
			})
		}
	}

	return TabPage{
		Title:  "Allocation",
		Layout: VBox{},
		Children: []Widget{
			Label{Text: "Real large/mid/small-cap/cash composition per equity fund. Nifty 50, Next 50, Midcap 150, and Smallcap 250 default to their exact index-methodology split (100% one segment) - everything else defaults to that same heuristic until you enter real numbers from the fund's factsheet."},
			ScrollView{AssignTo: &scroll, Layout: VBox{}},
		},
		OnSizeChanged: func() { rebuild() },
	}
}

// ---------- Member tab ----------

func memberPage() TabPage {
	var list *walk.ListBox

	refresh := func() {
		var names []string
		for _, m := range portfolio.Members {
			names = append(names, m.Name)
		}
		if names == nil {
			names = []string{}
		}
		list.SetModel(names)
	}

	return TabPage{
		Title:  "Member",
		Layout: VBox{},
		Children: []Widget{
			ListBox{AssignTo: &list},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						Text: "Add...",
						OnClicked: func() {
							defer safeRecover()
							name, ok := promptText(mw, "Add Member", "Name:", "")
							if !ok || strings.TrimSpace(name) == "" {
								return
							}
							portfolio.Members = append(portfolio.Members, store.Member{
								ID: store.NewID("mem"), Name: strings.TrimSpace(name),
							})
							refresh()
							saveOrWarn(mw)
						},
					},
					PushButton{
						Text: "Rename...",
						OnClicked: func() {
							defer safeRecover()
							idx := list.CurrentIndex()
							if idx < 0 {
								return
							}
							name, ok := promptText(mw, "Rename Member", "Name:", portfolio.Members[idx].Name)
							if !ok || strings.TrimSpace(name) == "" {
								return
							}
							portfolio.Members[idx].Name = strings.TrimSpace(name)
							refresh()
							saveOrWarn(mw)
						},
					},
					PushButton{
						Text: "Delete",
						OnClicked: func() {
							defer safeRecover()
							idx := list.CurrentIndex()
							if idx < 0 {
								return
							}
							memberID := portfolio.Members[idx].ID
							for _, a := range portfolio.Accounts {
								if a.MemberID == memberID {
									walk.MsgBox(mw, "Cannot delete",
										"This member has accounts. Delete those first.", walk.MsgBoxIconWarning)
									return
								}
							}
							portfolio.Members = append(portfolio.Members[:idx], portfolio.Members[idx+1:]...)
							refresh()
							saveOrWarn(mw)
						},
					},
				},
			},
		},
		AssignTo: nil,
		OnSizeChanged: func() {
			defer safeRecover()
			refresh()
		},
	}
}

// ---------- Account tab ----------

func accountPage() TabPage {
	var list *walk.ListBox

	refresh := func() {
		var lines []string
		for _, a := range portfolio.Accounts {
			memberName := "?"
			for _, m := range portfolio.Members {
				if m.ID == a.MemberID {
					memberName = m.Name
				}
			}
			lines = append(lines, fmt.Sprintf("%s (%s, %s)", a.Name, memberName, a.Currency))
		}
		if lines == nil {
			lines = []string{}
		}
		list.SetModel(lines)
	}

	return TabPage{
		Title:  "Account",
		Layout: VBox{},
		Children: []Widget{
			ListBox{AssignTo: &list},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						Text: "Add...",
						OnClicked: func() {
							defer safeRecover()
							if len(portfolio.Members) == 0 {
								walk.MsgBox(mw, "No members", "Add a member first.", walk.MsgBoxIconWarning)
								return
							}
							name, ok := promptText(mw, "Add Account", "Account name (e.g. AMC or broker name):", "")
							if !ok || strings.TrimSpace(name) == "" {
								return
							}
							currency, ok := promptText(mw, "Currency", "Currency (e.g. INR, CAD):", "INR")
							if !ok {
								currency = "INR"
							}
							// Default to the first member; renaming/reassigning
							// isn't exposed in this minimal UI yet.
							portfolio.Accounts = append(portfolio.Accounts, store.Account{
								ID: store.NewID("acc"), MemberID: portfolio.Members[0].ID,
								Name: strings.TrimSpace(name), Currency: strings.ToUpper(strings.TrimSpace(currency)),
							})
							refresh()
							saveOrWarn(mw)
						},
					},
					PushButton{
						Text: "Delete",
						OnClicked: func() {
							defer safeRecover()
							idx := list.CurrentIndex()
							if idx < 0 {
								return
							}
							accID := portfolio.Accounts[idx].ID
							for _, a := range portfolio.Assets {
								if a.AccountID == accID {
									walk.MsgBox(mw, "Cannot delete",
										"This account has assets. Delete those first.", walk.MsgBoxIconWarning)
									return
								}
							}
							portfolio.Accounts = append(portfolio.Accounts[:idx], portfolio.Accounts[idx+1:]...)
							refresh()
							saveOrWarn(mw)
						},
					},
				},
			},
		},
		OnSizeChanged: func() { defer safeRecover(); refresh() },
	}
}

// ---------- Asset tab ----------

func assetPage() TabPage {
	var list *walk.ListBox

	refresh := func() {
		var lines []string
		for _, a := range portfolio.Assets {
			accName := "?"
			for _, acc := range portfolio.Accounts {
				if acc.ID == a.AccountID {
					accName = acc.Name
				}
			}
			lines = append(lines, fmt.Sprintf("%s [%s] (%s / %s)", a.Name, a.ISIN, accName, a.Type))
		}
		if lines == nil {
			lines = []string{}
		}
		list.SetModel(lines)
	}

	return TabPage{
		Title:  "Asset",
		Layout: VBox{},
		Children: []Widget{
			ListBox{AssignTo: &list},
			Label{Text: "New assets are usually created automatically during CAS import - use the Import tab. This list is for review, and for adding stocks/ETFs you track manually."},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						Text: "Add Stock/ETF...",
						OnClicked: func() {
							defer safeRecover()
							if len(portfolio.Accounts) == 0 {
								walk.MsgBox(mw, "No accounts", "Add an account first.", walk.MsgBoxIconWarning)
								return
							}
							name, ok := promptText(mw, "Add Asset", "Name:", "")
							if !ok || strings.TrimSpace(name) == "" {
								return
							}
							symbol, _ := promptText(mw, "Symbol", "Yahoo symbol (e.g. RELIANCE.NS):", "")
							portfolio.Assets = append(portfolio.Assets, store.Asset{
								ID: store.NewID("ast"), AccountID: portfolio.Accounts[0].ID,
								Name: strings.TrimSpace(name), Type: "Stock", Symbol: strings.TrimSpace(symbol),
							})
							refresh()
							saveOrWarn(mw)
						},
					},
					PushButton{
						Text: "Delete",
						OnClicked: func() {
							defer safeRecover()
							idx := list.CurrentIndex()
							if idx < 0 {
								return
							}
							assetID := portfolio.Assets[idx].ID
							for _, t := range portfolio.Transactions {
								if t.AssetID == assetID {
									walk.MsgBox(mw, "Cannot delete",
										"This asset has transactions. Delete those first.", walk.MsgBoxIconWarning)
									return
								}
							}
							portfolio.Assets = append(portfolio.Assets[:idx], portfolio.Assets[idx+1:]...)
							refresh()
							saveOrWarn(mw)
						},
					},
				},
			},
		},
		OnSizeChanged: func() { defer safeRecover(); refresh() },
	}
}

// ---------- Transaction tab ----------

func transactionPage() TabPage {
	var box *walk.TextEdit

	refresh := func() {
		var lines []string
		for _, t := range portfolio.Transactions {
			assetName := "?"
			for _, a := range portfolio.Assets {
				if a.ID == t.AssetID {
					assetName = a.Name
				}
			}
			units := "-"
			if t.Units != nil {
				units = fmt.Sprintf("%.3f", *t.Units)
			}
			lines = append(lines, fmt.Sprintf("%s  %-14s  amt=%.2f  units=%s  %s  [%s]",
				t.Date, string(t.Type), t.Amount, units, assetName, t.Source))
		}
		box.SetText(strings.Join(lines, "\r\n"))
	}

	return TabPage{
		Title:  "Transaction",
		Layout: VBox{},
		Children: []Widget{
			Label{Text: fmt.Sprintf("%d transaction(s). Import via the Import tab, or review here.", len(portfolio.Transactions))},
			TextEdit{AssignTo: &box, ReadOnly: true, VScroll: true},
		},
		OnSizeChanged: func() { defer safeRecover(); refresh() },
	}
}

// ---------- Import tab ----------

func importPage() TabPage {
	var summaryLabel *walk.Label
	var stagedBox *walk.TextEdit
	var reviewBox *walk.TextEdit
	var memberCombo *walk.ComboBox
	var lastResult casimport.ImportResult
	var hasResult bool

	refreshMembers := func() {
		defer safeRecover()
		var names []string
		for _, m := range portfolio.Members {
			names = append(names, m.Name)
		}
		if names == nil {
			names = []string{}
		}
		prev := memberCombo.CurrentIndex()
		memberCombo.SetModel(names)
		if prev >= 0 && prev < len(names) {
			memberCombo.SetCurrentIndex(prev)
		} else if len(names) > 0 {
			memberCombo.SetCurrentIndex(0)
		}
	}

	return TabPage{
		Title:  "Import",
		Layout: VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Import for:"},
					ComboBox{AssignTo: &memberCombo},
				},
			},
			PushButton{
				Text: "Choose CAS PDF...",
				OnClicked: func() {
					defer safeRecover()
					if len(portfolio.Members) == 0 {
						walk.MsgBox(mw, "No members", "Add a member first (Member tab), so imported holdings can be attributed to someone.", walk.MsgBoxIconWarning)
						return
					}
					dlg := new(walk.FileDialog)
					dlg.Title = "Choose CAS PDF"
					dlg.Filter = "PDF files (*.pdf)|*.pdf"
					ok, err := dlg.ShowOpen(mw)
					if err != nil {
						walk.MsgBox(mw, "Error", err.Error(), walk.MsgBoxIconError)
						return
					}
					if !ok {
						return
					}
					result, err := runCASImport(dlg.FilePath)
					if err != nil {
						summaryLabel.SetText("Error: " + err.Error())
						return
					}
					lastResult = result
					hasResult = true
					summaryLabel.SetText(fmt.Sprintf("Format: %s. %d transaction(s) staged, %d need manual review. Click 'Commit Staged Transactions' to add them.",
						result.Format, len(result.Staged), len(result.ManualReview)))

					var stagedLines []string
					for _, s := range result.Staged {
						t := s.Txn
						units := "-"
						if t.Units != nil {
							units = fmt.Sprintf("%.3f", *t.Units)
						}
						stagedLines = append(stagedLines, fmt.Sprintf("%s  %-14s  amt=%.2f  units=%s  %s  (%s / %s)",
							t.Date, string(t.Type), t.Amount, units, t.Description, t.AMC, t.Scheme))
					}
					stagedBox.SetText(strings.Join(stagedLines, "\r\n"))

					var reviewLines []string
					for _, m := range result.ManualReview {
						reviewLines = append(reviewLines, fmt.Sprintf("[folio %s] %s -- %s", m.Folio, m.Reason, m.Text))
					}
					reviewBox.SetText(strings.Join(reviewLines, "\r\n"))
				},
			},
			Label{
				AssignTo: &summaryLabel,
				Text:     "Pick a CAS PDF (MFCentral format supported). Nothing is committed until you click Commit.",
			},
			GroupBox{
				Title:  "Would be staged",
				Layout: VBox{},
				Children: []Widget{
					TextEdit{AssignTo: &stagedBox, ReadOnly: true, VScroll: true},
				},
			},
			GroupBox{
				Title:  "Needs manual review",
				Layout: VBox{},
				Children: []Widget{
					TextEdit{AssignTo: &reviewBox, ReadOnly: true, VScroll: true},
				},
			},
			PushButton{
				Text: "Commit Staged Transactions",
				OnClicked: func() {
					defer safeRecover()
					if !hasResult || len(lastResult.Staged) == 0 {
						walk.MsgBox(mw, "Nothing to commit", "Choose a CAS PDF first.", walk.MsgBoxIconInformation)
						return
					}
					idx := memberCombo.CurrentIndex()
					if idx < 0 || idx >= len(portfolio.Members) {
						walk.MsgBox(mw, "No member selected", "Choose who this import belongs to.", walk.MsgBoxIconWarning)
						return
					}
					n := commitStagedTransactions(lastResult, portfolio.Members[idx].ID)
					saveOrWarn(mw)
					walk.MsgBox(mw, "Committed", fmt.Sprintf("%d transaction(s) added for %s.", n, portfolio.Members[idx].Name), walk.MsgBoxIconInformation)
				},
			},
		},
		OnSizeChanged: func() { refreshMembers() },
	}
}

// runCASImport reads a PDF file and runs it through casimport, exactly as
// the standalone CASImportChecker does.
func runCASImport(path string) (casimport.ImportResult, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return casimport.ImportResult{}, fmt.Errorf("could not open PDF: %w", err)
	}
	defer f.Close()

	numPages := r.NumPage()
	pageTexts := make([]string, 0, numPages)
	fonts := make(map[string]*pdf.Font)

	for pageIdx := 1; pageIdx <= numPages; pageIdx++ {
		page := r.Page(pageIdx)
		if page.V.IsNull() {
			continue
		}
		for _, name := range page.Fonts() {
			if _, ok := fonts[name]; !ok {
				f := page.Font(name)
				fonts[name] = &f
			}
		}
		text, err := page.GetPlainText(fonts)
		if err != nil {
			continue
		}
		pageTexts = append(pageTexts, text)
	}

	format := casimport.DetectFormat(strings.Join(pageTexts, "\n"))
	if format != "MFCENTRAL" {
		return casimport.ImportResult{Format: format}, fmt.Errorf("detected format %s - only MFCentral is supported so far", format)
	}
	return casimport.ParseMFCentral(pageTexts), nil
}

// commitStagedTransactions turns staged CAS rows into real
// StoredTransactions, auto-creating an Account (by AMC name, under
// defaultMemberID) and an Asset (matched by ISIN, or created) as needed.
func commitStagedTransactions(result casimport.ImportResult, defaultMemberID string) int {
	committed := 0
	for _, s := range result.Staged {
		t := s.Txn

		acc, ok := portfolio.FindAccountByName(defaultMemberID, t.AMC)
		if !ok {
			acc = store.Account{ID: store.NewID("acc"), MemberID: defaultMemberID, Name: t.AMC, Currency: "INR"}
			portfolio.Accounts = append(portfolio.Accounts, acc)
		}

		asset, ok := portfolio.FindAssetByISIN(t.ISIN)
		if !ok {
			asset = store.Asset{ID: store.NewID("ast"), AccountID: acc.ID, Name: t.Scheme, ISIN: t.ISIN, Type: "MutualFund"}
			portfolio.Assets = append(portfolio.Assets, asset)
		}

		portfolio.Transactions = append(portfolio.Transactions, store.StoredTransaction{
			ID: store.NewID("txn"), AccountID: acc.ID, AssetID: asset.ID,
			Date: t.Date, Type: t.Type, Description: t.Description,
			Amount: t.Amount, Units: t.Units, Price: t.Price, Source: "CAS_IMPORT",
		})
		committed++
	}
	return committed
}

// ---------- Price tab ----------

func pricePage() TabPage {
	var testSymbolEdit *walk.LineEdit
	var priceList *walk.ListBox

	refresh := func() {
		var lines []string
		for _, pr := range portfolio.Prices {
			assetName := "?"
			for _, a := range portfolio.Assets {
				if a.ID == pr.AssetID {
					assetName = a.Name
				}
			}
			lines = append(lines, fmt.Sprintf("%s: %.4f on %s [%s]", assetName, pr.Price, pr.Date, pr.Source))
		}
		if lines == nil {
			lines = []string{}
		}
		priceList.SetModel(lines)
	}

	return TabPage{
		Title:  "Price",
		Layout: VBox{},
		Children: []Widget{
			GroupBox{
				Title:  "Connectivity test",
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Yahoo test symbol:"},
					LineEdit{AssignTo: &testSymbolEdit, Text: "RELIANCE.NS"},
					PushButton{
						Text: "Run Connectivity Test",
						OnClicked: func() {
							defer safeRecover()
							symbol := strings.TrimSpace(testSymbolEdit.Text())
							if symbol == "" {
								symbol = "RELIANCE.NS"
							}
							results := priceapi.RunConnectivityTest(symbol)
							var lines []string
							for _, r := range results {
								mark := "X"
								if r.OK {
									mark = "\u2713" // checkmark
								}
								lines = append(lines, fmt.Sprintf("%s %s:\n%s", mark, r.Label, r.Message))
							}
							walk.MsgBox(mw, "Connectivity test", strings.Join(lines, "\n\n"), walk.MsgBoxIconInformation)
						},
					},
				},
			},
			Label{Text: "Manually recorded prices:"},
			ListBox{AssignTo: &priceList},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					PushButton{
						Text: "Add Manual Price...",
						OnClicked: func() {
							defer safeRecover()
							if len(portfolio.Assets) == 0 {
								walk.MsgBox(mw, "No assets", "Add an asset first.", walk.MsgBoxIconWarning)
								return
							}
							assetName, ok := promptText(mw, "Asset", "Asset name (exact match):", "")
							if !ok {
								return
							}
							var target *store.Asset
							for i := range portfolio.Assets {
								if portfolio.Assets[i].Name == strings.TrimSpace(assetName) {
									target = &portfolio.Assets[i]
									break
								}
							}
							if target == nil {
								walk.MsgBox(mw, "Not found", "No asset with that exact name.", walk.MsgBoxIconWarning)
								return
							}
							priceStr, ok := promptText(mw, "Price", "Price:", "")
							if !ok {
								return
							}
							price, err := strconv.ParseFloat(strings.TrimSpace(priceStr), 64)
							if err != nil {
								walk.MsgBox(mw, "Invalid price", err.Error(), walk.MsgBoxIconError)
								return
							}
							dateStr, ok := promptText(mw, "Date", "Date (YYYY-MM-DD):", "")
							if !ok {
								return
							}
							portfolio.Prices = append(portfolio.Prices, store.PriceRecord{
								AssetID: target.ID, Date: strings.TrimSpace(dateStr), Price: price, Source: "MANUAL",
							})
							refresh()
							saveOrWarn(mw)
						},
					},
				},
			},
		},
		OnSizeChanged: func() { defer safeRecover(); refresh() },
	}
}

// ---------- Backup & Reset tab ----------

func backupPage() TabPage {
	var statusLabel *walk.Label

	return TabPage{
		Title:  "Backup && Reset",
		Layout: VBox{},
		Children: []Widget{
			Label{Text: fmt.Sprintf("Portfolio file: %s", portfolioPath)},
			Label{AssignTo: &statusLabel, Text: fmt.Sprintf("Members: %d, Accounts: %d, Assets: %d, Transactions: %d",
				len(portfolio.Members), len(portfolio.Accounts), len(portfolio.Assets), len(portfolio.Transactions))},
			PushButton{
				Text: "Backup Now",
				OnClicked: func() {
					defer safeRecover()
					if err := store.Save(portfolioPath, portfolio); err != nil {
						walk.MsgBox(mw, "Backup failed", err.Error(), walk.MsgBoxIconError)
						return
					}
					walk.MsgBox(mw, "Backed up", "A timestamped backup was saved alongside the portfolio file.", walk.MsgBoxIconInformation)
				},
			},
			PushButton{
				Text: "Reset (clear all data)",
				OnClicked: func() {
					defer safeRecover()
					if walk.MsgBox(mw, "Confirm reset",
						"This clears ALL members, accounts, assets, transactions, and prices. A backup is taken first. Continue?",
						walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
						return
					}
					_ = store.Save(portfolioPath, portfolio) // backup current state before wiping
					portfolio.Members = nil
					portfolio.Accounts = nil
					portfolio.Assets = nil
					portfolio.Transactions = nil
					portfolio.Prices = nil
					saveOrWarn(mw)
					statusLabel.SetText("Reset complete. Restart the app to see the cleared tabs.")
				},
			},
		},
	}
}
