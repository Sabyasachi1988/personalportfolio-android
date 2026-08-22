package casimport

import "strings"

// Classify returns the best-guess transaction type for a description, and
// whether the match was confident enough to skip manual review. Anything
// not confidently matched should be routed to manual review by the caller
// rather than silently defaulted to MISC.
func Classify(description string) (typ string, confident bool) {
	d := strings.ToLower(description)

	switch {
	case containsAny(d, "switch out - merger", "switch-out - merger"):
		return "SWITCH_OUT_MERGER", true
	case containsAny(d, "switch in - merger", "switch-in - merger"):
		return "SWITCH_IN_MERGER", true
	case containsAny(d, "switch out", "switch-out"):
		return "SWITCH_OUT", true
	case containsAny(d, "switch in", "switch-in"):
		return "SWITCH_IN", true
	case containsAny(d, "redemption", "redeem"):
		return "REDEMPTION", true
	case containsAny(d, "dividend reinvest", "idcw reinvest"):
		return "DIVIDEND_REINVEST", true
	case containsAny(d, "dividend", "idcw"):
		return "DIVIDEND_PAYOUT", true
	case containsAny(d, "stamp duty"):
		return "STAMP_DUTY_TAX", true
	case containsAny(d, "stt"):
		return "STT_TAX", true
	case containsAny(d, "tds"):
		return "TDS_TAX", true
	case containsAny(d, "segregat"):
		return "SEGREGATION", true
	case containsAny(d, "reversal", "reversed", "rejected"):
		return "REVERSAL", true
	case containsAny(d, "sys. investment", "sys investment", "sip"):
		return "PURCHASE_SIP", true
	case containsAny(d, "purchase"):
		return "PURCHASE", true
	default:
		return "UNKNOWN", false
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
