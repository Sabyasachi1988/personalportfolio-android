// Command casimportgui is a small standalone window for checking CAS PDF
// import against the tested MFCentral parser, without needing a terminal.
// It does not touch or write any portfolio data - it only reads a PDF you
// pick and shows you what would be staged.
package main

import (
	"fmt"
	"strings"

	"ledger/internal/casimport"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"github.com/ledongthuc/pdf"
)

func main() {
	var mw *walk.MainWindow
	var summaryLabel *walk.Label
	var stagedBox *walk.TextEdit
	var reviewBox *walk.TextEdit
	var debugBox *walk.TextEdit

	MainWindow{
		AssignTo: &mw,
		Title:    "CAS Import Checker",
		MinSize:  Size{Width: 700, Height: 500},
		Layout:   VBox{},
		Children: []Widget{
			PushButton{
				Text: "Choose CAS PDF...",
				OnClicked: func() {
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
					runImport(dlg.FilePath, summaryLabel, stagedBox, reviewBox, debugBox)
				},
			},
			PushButton{
				Text: "Copy Debug Info to Clipboard",
				OnClicked: func() {
					text := debugBox.Text()
					if text == "" {
						walk.MsgBox(mw, "Nothing to copy", "Choose a CAS PDF first.", walk.MsgBoxIconInformation)
						return
					}
					if err := walk.Clipboard().SetText(text); err != nil {
						walk.MsgBox(mw, "Error", err.Error(), walk.MsgBoxIconError)
						return
					}
					walk.MsgBox(mw, "Copied", "Debug info copied to clipboard - paste it into the chat.", walk.MsgBoxIconInformation)
				},
			},
			Label{
				AssignTo: &summaryLabel,
				Text:     "Pick a CAS PDF to check. Nothing is written or changed by doing this.",
			},
			GroupBox{
				Title:  "Would be staged as transactions",
				Layout: VBox{},
				Children: []Widget{
					TextEdit{
						AssignTo: &stagedBox,
						ReadOnly: true,
						VScroll:  true,
					},
				},
			},
			GroupBox{
				Title:  "Needs manual review (not auto-imported)",
				Layout: VBox{},
				Children: []Widget{
					TextEdit{
						AssignTo: &reviewBox,
						ReadOnly: true,
						VScroll:  true,
					},
				},
			},
			GroupBox{
				Title:  "Debug info (click 'Copy Debug Info' above, paste into chat)",
				Layout: VBox{},
				Children: []Widget{
					TextEdit{
						AssignTo: &debugBox,
						ReadOnly: true,
						VScroll:  true,
					},
				},
			},
		},
	}.Create()

	mw.Run()
}

func runImport(path string, summaryLabel *walk.Label, stagedBox, reviewBox, debugBox *walk.TextEdit) {
	f, r, err := pdf.Open(path)
	if err != nil {
		summaryLabel.SetText(fmt.Sprintf("Could not open PDF: %v", err))
		return
	}
	defer f.Close()

	numPages := r.NumPage()
	pageTexts := make([]string, 0, numPages)
	fonts := make(map[string]*pdf.Font)
	var debugLines []string

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
			debugLines = append(debugLines, fmt.Sprintf("[page %d] extraction error: %v", pageIdx, err))
			continue
		}
		pageTexts = append(pageTexts, text)
		debugLines = append(debugLines, fmt.Sprintf("[page %d] %s", pageIdx, text))
	}

	format := casimport.DetectFormat(strings.Join(pageTexts, "\n"))
	debugBox.SetText(strings.Join(debugLines, "\r\n"))

	if format != "MFCENTRAL" {
		summaryLabel.SetText(fmt.Sprintf("Detected format: %s. This build only handles MFCentral-format statements - nothing was imported.", format))
		stagedBox.SetText("")
		reviewBox.SetText("")
		return
	}

	result := casimport.ParseMFCentral(pageTexts)

	summaryLabel.SetText(fmt.Sprintf("Detected format: MFCentral. %d transaction(s) would be staged, %d line(s) need manual review.",
		len(result.Staged), len(result.ManualReview)))

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
}
