// Command cascli is a diagnostic tool, not the final GUI import path.
// It exists to let a real CAS PDF be run through casimport's parser and
// inspected, before that logic gets wired back into the Manage dialog.
//
// Usage: cascli path/to/statement.pdf
package main

import (
	"fmt"
	"os"
	"strings"

	"ledger/internal/casimport"

	"github.com/ledongthuc/pdf"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: cascli path/to/statement.pdf")
		os.Exit(1)
	}
	path := os.Args[1]

	f, r, err := pdf.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not open PDF: %v\n", err)
		os.Exit(1)
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
			fmt.Fprintf(os.Stderr, "page %d: could not extract text: %v\n", pageIdx, err)
			continue
		}
		pageTexts = append(pageTexts, text)
	}

	format := casimport.DetectFormat(strings.Join(pageTexts, "\n"))
	fmt.Printf("Detected format: %s\n\n", format)

	if format != "MFCENTRAL" {
		fmt.Println("This build only wires up the MFCentral path. Native CAMS/KFintech")
		fmt.Println("parsing was not reconstructed this session (see chat) - nothing was")
		fmt.Println("parsed for this file.")
		return
	}

	result := casimport.ParseMFCentral(pageTexts)

	fmt.Printf("Staged transactions: %d\n", len(result.Staged))
	fmt.Printf("Manual review lines: %d\n\n", len(result.ManualReview))

	fmt.Println("=== Staged ===")
	for _, s := range result.Staged {
		t := s.Txn
		units := "-"
		if t.Units != nil {
			units = fmt.Sprintf("%.3f", *t.Units)
		}
		fmt.Printf("%-10s %-14s amt=%10.2f units=%-9s %s [%s / %s]\n",
			t.Date, string(t.Type), t.Amount, units, truncate(t.Description, 40), t.AMC, t.Scheme)
	}

	if len(result.ManualReview) > 0 {
		fmt.Println("\n=== Needs manual review ===")
		for _, m := range result.ManualReview {
			fmt.Printf("folio=%-14s reason=%-45s | %s\n", m.Folio, m.Reason, m.Text)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
