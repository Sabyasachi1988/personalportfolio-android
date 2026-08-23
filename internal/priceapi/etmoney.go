package priceapi

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CapCompositionResult is a fetched Large/Mid/Small/Cash breakdown,
// together with the raw label text matched (for surfacing to the person
// so a plausible-but-wrong match is easy to catch, rather than silently
// trusting it).
type CapCompositionResult struct {
	Large      float64
	Mid        float64
	Small      float64
	Cash       float64
	MatchedSum float64 // Large+Mid+Small+Cash, for the caller to sanity-check
}

var (
	scriptOrStyleTag = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	anyTag           = regexp.MustCompile(`(?s)<[^>]+>`)
	multiSpace       = regexp.MustCompile(`\s+`)
)

// htmlToPlainText strips script/style blocks and all remaining tags,
// then collapses whitespace, so label-proximity matching below works
// against readable text rather than raw markup. Deliberately simple
// (not a real HTML parser) - good enough for finding "Large Cap ... NN%"
// style label/value pairs regardless of the exact tag structure around
// them.
func htmlToPlainText(html string) string {
	noScripts := scriptOrStyleTag.ReplaceAllString(html, " ")
	noTags := anyTag.ReplaceAllString(noScripts, " ")
	return multiSpace.ReplaceAllString(noTags, " ")
}

// firstPercentAfter finds the first occurrence of any of `labels` in
// `text` (case-insensitive) and returns the first N.N%-style number
// within the following `window` characters. This is a proximity
// heuristic, not a structured parse - ETMoney's actual markup was not
// independently verified before this was written (see FetchETMoneyCapComposition
// doc comment), so this deliberately looks for the same simple pattern
// ("label", then shortly after, a percentage) that would survive most
// reasonable markup changes, rather than depending on exact tag/class
// names that could break silently.
func firstPercentAfter(text string, labels []string, window int) (float64, bool) {
	lower := strings.ToLower(text)
	percentRe := regexp.MustCompile(`(\d{1,3}(?:\.\d{1,2})?)\s*%`)

	for _, label := range labels {
		idx := strings.Index(lower, strings.ToLower(label))
		if idx == -1 {
			continue
		}
		end := idx + len(label) + window
		if end > len(text) {
			end = len(text)
		}
		segment := text[idx+len(label) : end]
		match := percentRe.FindStringSubmatch(segment)
		if match == nil {
			continue
		}
		val, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}
		return val, true
	}
	return 0, false
}

// FetchETMoneyCapComposition fetches the given ETMoney mutual fund page
// URL and attempts to extract its Large/Mid/Small-cap and Cash
// allocation percentages.
//
// IMPORTANT CAVEAT: this was built against the documented shape of an
// ETMoney fund page (e.g.
// https://www.etmoney.com/mutual-funds/<slug>/<id>), but the live page's
// exact HTML/markup could not be independently fetched and inspected
// before writing this - so unlike FetchAmfiNav (verified reliable) this
// is UNVERIFIED, same status FetchYahooQuote's ETF path had before its
// first real on-device test. If this returns an error or MatchedSum is
// far from 100, say so plainly rather than silently trusting a
// mismatched or partial parse - manual entry remains available as a
// fallback regardless of what this returns.
func FetchETMoneyCapComposition(url string) (CapCompositionResult, error) {
	if !strings.HasPrefix(url, "https://www.etmoney.com/") && !strings.HasPrefix(url, "https://etmoney.com/") {
		return CapCompositionResult{}, fmt.Errorf("URL doesn't look like an ETMoney fund page: %s", url)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return CapCompositionResult{}, err
	}
	// Many sites 403 a request with no browser-like headers at all -
	// same reasoning as FetchYahooQuote's User-Agent.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14; Mobile) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return CapCompositionResult{}, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CapCompositionResult{}, fmt.Errorf("fetching %s: unexpected status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CapCompositionResult{}, err
	}

	text := htmlToPlainText(string(body))

	const window = 60 // characters after a label to look for its percentage
	large, largeOK := firstPercentAfter(text, []string{"Large Cap", "Large-cap", "Largecap"}, window)
	mid, midOK := firstPercentAfter(text, []string{"Mid Cap", "Mid-cap", "Midcap"}, window)
	small, smallOK := firstPercentAfter(text, []string{"Small Cap", "Small-cap", "Smallcap"}, window)
	// "Cash" alone is a common false-positive trigger word (appears in
	// unrelated marketing copy), so it's tried last and is allowed to
	// come back as 0/not-found without failing the whole fetch - unlike
	// Large/Mid/Small, which are essential.
	cash, _ := firstPercentAfter(text, []string{"Cash & Others", "Cash and Others", "Cash Equivalent", "Cash"}, window)

	if !largeOK || !midOK || !smallOK {
		return CapCompositionResult{}, fmt.Errorf("could not find Large/Mid/Small cap percentages on the page (large found=%v, mid found=%v, small found=%v) — page layout may not match what this was built against; enter manually instead", largeOK, midOK, smallOK)
	}

	sum := large + mid + small + cash
	return CapCompositionResult{Large: large, Mid: mid, Small: small, Cash: cash, MatchedSum: sum}, nil
}
