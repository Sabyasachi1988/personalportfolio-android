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

func TestFirstPercentAfterFindsNearbyValue(t *testing.T) {
	text := "Asset Allocation Large Cap : 62.30 % Mid Cap : 21.5% Small Cap 10%"
	large, ok := firstPercentAfter(text, []string{"Large Cap"}, 60)
	if !ok || large != 62.3 {
		t.Fatalf("Large Cap: got %v ok=%v, want 62.3", large, ok)
	}
	mid, ok := firstPercentAfter(text, []string{"Mid Cap"}, 60)
	if !ok || mid != 21.5 {
		t.Fatalf("Mid Cap: got %v ok=%v, want 21.5", mid, ok)
	}
	small, ok := firstPercentAfter(text, []string{"Small Cap"}, 60)
	if !ok || small != 10 {
		t.Fatalf("Small Cap: got %v ok=%v, want 10", small, ok)
	}
}

func TestFirstPercentAfterMissingLabel(t *testing.T) {
	_, ok := firstPercentAfter("nothing relevant here", []string{"Large Cap"}, 60)
	if ok {
		t.Fatalf("expected no match, got a match")
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
