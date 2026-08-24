package priceapi

import "testing"

func TestHtmlToPlainTextStripsScriptsAndTags(t *testing.T) {
	html := `<html><head><style>.x{color:red}</style></head><body><script>var x=1;</script><div class="alloc">Large Cap <span>62.3%</span></div></body></html>`
	text := htmlToPlainText(html)
	if want := "Large Cap"; !contains(text, want) {
		t.Fatalf("expected plain text to contain %q, got: %s", want, text)
	}
	if contains(text, "color:red") {
		t.Fatalf("style block leaked into plain text: %s", text)
	}
	if contains(text, "var x=1") {
		t.Fatalf("script block leaked into plain text: %s", text)
	}
}

func TestPercentNearAnyOccurrence_FindsNearbyValue(t *testing.T) {
	text := "Asset Allocation Large Cap : 62.30 % Mid Cap : 21.5% Small Cap 10%"
	large, ok := percentNearAnyOccurrence(text, []string{"Large Cap"}, 60)
	if !ok || large != 62.3 {
		t.Fatalf("Large Cap: got %v ok=%v, want 62.3", large, ok)
	}
	mid, ok := percentNearAnyOccurrence(text, []string{"Mid Cap"}, 60)
	if !ok || mid != 21.5 {
		t.Fatalf("Mid Cap: got %v ok=%v, want 21.5", mid, ok)
	}
	small, ok := percentNearAnyOccurrence(text, []string{"Small Cap"}, 60)
	if !ok || small != 10 {
		t.Fatalf("Small Cap: got %v ok=%v, want 10", small, ok)
	}
}

func TestPercentNearAnyOccurrence_MissingLabel(t *testing.T) {
	_, ok := percentNearAnyOccurrence("nothing relevant here", []string{"Large Cap"}, 60)
	if ok {
		t.Fatalf("expected no match, got a match")
	}
}

// TestPercentNearAnyOccurrence_SkipsHeadingAndFindsRealTable reproduces
// the exact reported bug: a fund named "Nippon India Growth Mid Cap
// Fund" has its FIRST "Mid Cap" occurrence in the page's own heading -
// not followed by a percentage - with the real allocation table's
// "Mid Cap: 65.85%" appearing only later in the page. The earlier
// first-occurrence-only version of this function gave up at the
// heading and never found the real number; this version must keep
// scanning past a non-matching occurrence.
func TestPercentNearAnyOccurrence_SkipsHeadingAndFindsRealTable(t *testing.T) {
	text := "Nippon India Growth Mid Cap Fund - Direct Growth Plan " +
		"Overview Returns Portfolio " +
		"Asset Allocation Large Cap 21.98 % Mid Cap 65.85 % Small Cap 11.26 %"
	large, largeOK := percentNearAnyOccurrence(text, []string{"Large Cap"}, 60)
	mid, midOK := percentNearAnyOccurrence(text, []string{"Mid Cap"}, 60)
	small, smallOK := percentNearAnyOccurrence(text, []string{"Small Cap"}, 60)

	if !largeOK || large != 21.98 {
		t.Errorf("Large Cap: got %v ok=%v, want 21.98", large, largeOK)
	}
	if !midOK || mid != 65.85 {
		t.Errorf("Mid Cap: got %v ok=%v, want 65.85 (the heading's bare 'Mid Cap' with no nearby percent must be skipped)", mid, midOK)
	}
	if !smallOK || small != 11.26 {
		t.Errorf("Small Cap: got %v ok=%v, want 11.26", small, smallOK)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (func() bool {
		for i := 0; i+len(needle) <= len(haystack); i++ {
			if haystack[i:i+len(needle)] == needle {
				return true
			}
		}
		return false
	})()
}
