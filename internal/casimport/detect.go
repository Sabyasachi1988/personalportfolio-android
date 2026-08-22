package casimport

import "strings"

// DetectFormat inspects the full extracted text of a CAS PDF (all pages
// concatenated) and decides which parser should handle it. Detection is
// signature-based rather than assumed, since the whole point of this
// package is not to box the user into one issuer's portal.
//
// MFCentral's re-rendered CAS carries a distinctive combination that a
// native single-issuer CAMS or KFintech statement never has:
//   - the footer filename stem "MFCentralDetailCAS"
//   - the "mf central" product wordmark
//   - both "SoA Holdings" and "Demat Holdings" tab labels together
//
// Any one of these alone would be a reasonable signal; requiring the
// footer stem specifically (the most stable of the three) as primary,
// falling back to the other two, avoids a false MFCentral detection on a
// native CAS that happens to mention "holdings" somewhere.
func DetectFormat(fullText string) string {
	lower := strings.ToLower(fullText)

	if strings.Contains(lower, "mfcentraldetailcas") {
		return "MFCENTRAL"
	}
	if strings.Contains(lower, "mf central") &&
		strings.Contains(lower, "soa holdings") &&
		strings.Contains(lower, "demat holdings") {
		return "MFCENTRAL"
	}
	return "CAMS_KFINTECH_NATIVE"
}
