package casimport

import "ledger/internal/store"

// StagedRow is a parsed transaction awaiting user confirmation in the
// Import tab, mirroring the CSV-import staging shape so CAS-derived rows
// flow through the exact same confirm/reject/resolve UI.
type StagedRow struct {
	Txn         store.Transaction
	Status      string // "NEW", "DUPLICATE", "UNMATCHED"
	SourcePage  int    // 1-based PDF page the row came from, for troubleshooting
	SourceFolio string
}

// ManualReviewLine is raw text the parser could not confidently turn into a
// transaction. Per the existing design principle, nothing is ever silently
// dropped — anything the parser can't classify lands here instead.
type ManualReviewLine struct {
	Page   int
	Folio  string
	Text   string
	Reason string
}

// ImportResult is the top-level output of parsing a CAS PDF.
type ImportResult struct {
	Format       string // "CAMS_KFINTECH_NATIVE" or "MFCENTRAL"
	Staged       []StagedRow
	ManualReview []ManualReviewLine
}
