package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"time"
)

// Member is a person whose holdings are tracked (the user, a family
// member, etc).
type Member struct {
	ID   string
	Name string
}

// Account is a place holdings live: a mutual fund AMC relationship, a
// brokerage account, a LIRA, etc.
type Account struct {
	ID       string
	MemberID string
	Name     string
	Currency string // "INR", "CAD", ...
}

// Asset is one holding definition: a specific mutual fund scheme, stock,
// or ETF, tracked within an Account.
type Asset struct {
	ID         string
	AccountID  string
	Name       string
	ISIN       string
	Type       string // "MutualFund", "Stock", "ETF"
	Symbol     string // Yahoo-style symbol for stocks/ETFs, e.g. "RELIANCE.NS"
	AssetClass string // AMFI/SEBI category, e.g. "Equity", "Debt", "Other" (set on price refresh)
	// ETMoneyURL and GroupLabel are deliberately NOT omitempty, despite
	// being newer, optional-feeling fields - Gson's default Kotlin
	// deserialization uses unsafe field allocation (bypasses the actual
	// Kotlin constructor for performance), which means it does NOT
	// apply a Kotlin data class's `= ""` default for a JSON key that's
	// genuinely ABSENT - it silently leaves the field as null instead,
	// despite the Kotlin type system declaring it a non-null String.
	// Every other field here has no omitempty and is therefore always
	// present in the JSON (as "" when unset), which is what makes them
	// safe. omitempty on these two meant the key was completely missing
	// for every asset that had never had one set - which was EVERY
	// asset for GroupLabel, confirmed as a real crash: Kotlin code that
	// trusts its own non-null type (no defensive null-check, since
	// there was no reason to expect one) called a method directly on
	// the "always non-null" field and got an immediate
	// NullPointerException the moment it actually was null at runtime.
	ETMoneyURL string // full ETMoney fund page URL (e.g. https://www.etmoney.com/mutual-funds/<slug>/<id>), entered once so cap composition can be auto-fetched from it; empty means "not linked, manual entry only"
	// GroupLabel is a free-text bucket the person assigns themselves,
	// e.g. "Nifty 50" - so several DIFFERENT real assets (different
	// AMC, different ISIN - e.g. a Nippon India Nifty 50 fund, a Navi
	// Nifty 50 fund, and an HDFC Nifty 50 ETF) that all represent the
	// same underlying exposure can be viewed as one consolidated line
	// when useful, without merging their actual stored data - each
	// Asset/transaction/ISIN stays exactly as it is; GroupLabel is
	// purely a display-time aggregation key (see
	// finance.GroupHoldingsByLabel). Empty means "not grouped, show
	// individually" - the default and current behavior for everything
	// until the person deliberately labels something.
	//
	// Deliberately NOT omitempty - see ETMoneyURL's comment above for
	// why: this exact field, with omitempty, caused a confirmed real
	// crash (a Kotlin NullPointerException from Gson leaving a
	// "non-null" field actually null when its JSON key was missing,
	// which it was for every asset until one was explicitly labeled).
	GroupLabel string
	// Tags is a free-text, MANY-TO-MANY set of characteristics the person
	// assigns themselves - e.g. a "Nippon India Growth Mid Cap Fund"
	// might carry ["Mid Cap", "Growth", "Long Term"] all at once. This is
	// deliberately a separate concept from GroupLabel above: GroupLabel
	// means "this is the same underlying exposure as that other asset"
	// (one label, used to consolidate/sum), Tags means "this asset HAS
	// these characteristics" (many tags, used to slice/filter) - a fund
	// can belong to at most one GroupLabel bucket but any number of tag
	// buckets simultaneously. Order matters: it's insertion order (new
	// tags append to the end, never resorted), which is what
	// EffectiveTag falls back to when PrimaryTag isn't set - see its own
	// doc comment. Nil is normalized to an empty (non-nil) slice by
	// Load() - see PrimaryTag's comment just below for why a genuinely
	// nil/missing slice here is dangerous, not just untidy.
	//
	// Deliberately NOT omitempty - same Gson-unsafe-allocation landmine
	// as GroupLabel above: a missing "Tags" key would leave a Kotlin
	// `List<String> = emptyList()` field null instead of applying that
	// default, and a nil Go slice marshals to JSON `null` (not `[]`)
	// without omitempty, which is the SAME landmine even with the key
	// present - hence Load() additionally normalizes nil to []string{}
	// on every load, so this field is never nil by the time anything
	// marshals it back out to Kotlin.
	Tags []string
	// PrimaryTag optionally overrides which single tag represents this
	// asset for MUTUALLY EXCLUSIVE groupings (a pie/donut chart, where a
	// fund can only contribute to exactly one slice) - see EffectiveTag.
	// Empty means "no override, use the first tag in Tags" - the
	// common case; this field exists only for the rarer moment two tags
	// on the same fund would otherwise collide in the same pie chart and
	// the person wants to deliberately choose which one wins, rather
	// than relying on insertion order. Progression grouping never uses
	// this - a fund contributing to several tags' progression lines
	// simultaneously is correct, not a collision, so only pie/donut-style
	// views need an exclusive choice at all.
	//
	// Deliberately NOT omitempty - same reasoning as GroupLabel/Tags
	// above.
	PrimaryTag string
}

// EffectiveTag resolves the single tag used for mutually-exclusive
// (pie/donut-style) groupings - see PrimaryTag's doc comment. Resolution
// order: PrimaryTag if it's set AND still actually present in Tags
// (guards against a stale override left over after the tag itself was
// removed), else the first tag in Tags (insertion order), else "" if the
// asset has no tags at all ("Untagged" is applied by the caller, e.g.
// AllocationByTag - kept out of this function so it stays a pure
// resolver over this asset's own data, not a display concern).
func (a Asset) EffectiveTag() string {
	if a.PrimaryTag != "" {
		for _, t := range a.Tags {
			if t == a.PrimaryTag {
				return a.PrimaryTag
			}
		}
	}
	if len(a.Tags) > 0 {
		return a.Tags[0]
	}
	return ""
}

// StoredTransaction is a confirmed transaction attached to a real
// Account/Asset, as opposed to the free-floating store.Transaction used
// during CAS import staging.
type StoredTransaction struct {
	ID          string
	AccountID   string
	AssetID     string
	Date        string
	Type        TransactionType
	Description string
	Amount      float64
	Units       *float64
	Price       *float64
	Balance     *float64 // running unit balance as printed on the CAS statement, when available - see transactionsMatch in mobile/bridge/bridge.go for why this matters for duplicate detection
	Source      string   // "MANUAL", "CAS_IMPORT", "CSV_IMPORT"
}

// FXRate is one historical exchange rate point: on Date, 1 unit of
// Currency equals INRPerUnit rupees. INR itself is never stored here -
// it's the implicit base and is always 1.0 by definition. This lets any
// two currencies convert pairwise through INR without needing every
// currency-pair combination fetched or stored.
type FXRate struct {
	Date       string
	Currency   string
	INRPerUnit float64
}

// PriceRecord is a manually entered or fetched price point for an Asset.
type PriceRecord struct {
	AssetID string
	Date    string
	Price   float64
	Source  string // "MANUAL", "AMFI", "YAHOO"
}

// CapComposition is the real large/mid/small-cap breakdown of one equity
// fund's actual holdings, as opposed to the single-bucket heuristic
// (GuessMarketCapSegment) used when no real data has been entered. Values
// are percentages and need not sum to exactly 100 (they're normalised by
// their own sum wherever they're used) - entering approximate factsheet
// figures shouldn't require fussing over rounding. Cash is included
// because many equity funds hold a real cash position rather than being
// fully invested at all times.
type CapComposition struct {
	AssetID string
	Large   float64
	Mid     float64
	Small   float64
	Cash    float64
	AsOf    string // ISO date the numbers were entered/sourced from
	Source  string // e.g. "Factsheet Aug 2026", "Index methodology (pure tracker)"
}

// Portfolio is the full persisted state of the app.
// TargetAllocation is the person's own chosen target market-cap mix
// (percentages, not required to sum to exactly 100). All-zero means "no
// target has been set yet" - a real target would never actually be all
// zero, so this doubles as the "is a target set" check without needing a
// separate boolean flag.
type TargetAllocation struct {
	Large float64
	Mid   float64
	Small float64
	Cash  float64
}

// HasTarget reports whether a real target has been entered.
func (t TargetAllocation) HasTarget() bool {
	return t.Large != 0 || t.Mid != 0 || t.Small != 0 || t.Cash != 0
}

// EquityOriginComposition is the real Indian vs. International split of
// one equity fund's actual holdings - percentages, normalised by their
// own sum wherever used, same convention as CapComposition. Only
// meaningful for funds classified as Equity (see
// finance.EffectiveAssetClass); a fund with no entry here defaults to
// 100% Indian when used, since the large majority of Indian AMC schemes
// are domestic-only. That default is a reasonable starting assumption,
// not a verified fact - it should be overridden by entering the real
// composition for any fund that actually holds international exposure
// (e.g. a fund-of-fund tracking a US index).
type EquityOriginComposition struct {
	AssetID       string
	Indian        float64
	International float64
	AsOf          string
	Source        string
}

// PortfolioClassTarget is the person's own chosen target Equity/Debt/
// Commodity/Others mix (percentages, not required to sum to exactly
// 100). All-zero means "no target set yet", same convention as
// TargetAllocation.
type PortfolioClassTarget struct {
	Equity    float64
	Debt      float64
	Commodity float64
	Others    float64
}

// HasTarget reports whether a real target has been entered.
func (t PortfolioClassTarget) HasTarget() bool {
	return t.Equity != 0 || t.Debt != 0 || t.Commodity != 0 || t.Others != 0
}

type Portfolio struct {
	Members                  []Member
	Accounts                 []Account
	Assets                   []Asset
	Transactions             []StoredTransaction
	Prices                   []PriceRecord
	CapCompositions          []CapComposition
	TargetAllocation         TargetAllocation
	EquityOriginCompositions []EquityOriginComposition
	PortfolioClassTarget     PortfolioClassTarget
	FXRates                  []FXRate

	// Lazily-built, per-instance lookup caches for PriceAsOf/FXRateAsOf -
	// see those methods' doc comments for why they exist (a plain linear
	// scan over ALL Prices/FXRates on every single call became the
	// dominant cost of computing a progression series once real
	// multi-year history accumulated). Unexported: invisible to JSON
	// (de)serialization, and only ever meaningful for the lifetime of
	// one in-memory *Portfolio instance - a freshly-unmarshaled Portfolio
	// always starts with these unbuilt (Go zero values), which is
	// correct, since there's nothing to reuse across separate loads.
	priceIndexBuilt bool
	priceIndex      map[string][]PriceRecord // per assetID, sorted ascending by Date
	fxIndexBuilt    bool
	fxIndex         map[string][]FXRate // per currency, sorted ascending by Date
}

// invalidatePriceIndex discards the cached price index, if any - called
// from anything that mutates Prices after the index may have already
// been built for this instance, so a later PriceAsOf call rebuilds
// against the current data rather than serving stale results from
// before the mutation. Current callers in this codebase only ever
// read OR write prices within a single Portfolio instance's lifetime,
// never both - but this makes that a safety property enforced by the
// code, not just an unstated assumption a future caller could silently
// violate.
func (p *Portfolio) invalidatePriceIndex() {
	p.priceIndexBuilt = false
	p.priceIndex = nil
}

func (p *Portfolio) invalidateFXIndex() {
	p.fxIndexBuilt = false
	p.fxIndex = nil
}

// idCounter guarantees NewID is unique even when called many times within
// the same clock tick - which happens routinely (e.g. creating several
// assets in a tight loop while committing a CAS import), and on Windows
// in particular time.Now()'s effective resolution can be coarse enough
// for a timestamp-only ID to collide. A monotonically increasing counter
// makes collisions impossible regardless of clock resolution.
var idCounter uint64

// NewID returns a unique ID, safe to call many times in a tight loop.
func NewID(prefix string) string {
	n := atomic.AddUint64(&idCounter, 1)
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), n)
}

// Load reads the portfolio JSON file at path. A missing file returns an
// empty Portfolio, not an error, so first-run works without ceremony.
func Load(path string) (*Portfolio, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Portfolio{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading portfolio file: %w", err)
	}
	var p Portfolio
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing portfolio file: %w", err)
	}
	// A portfolio.json saved before Asset.Tags existed simply has no
	// "Tags" key at all for any asset, which unmarshals to a nil slice -
	// and a nil slice marshals right back out as JSON `null`, not `[]`,
	// which is the exact Gson-unsafe-allocation landmine documented on
	// Asset.Tags itself. Normalizing here, once, right after every load,
	// means every OTHER code path in this app (every bridge function
	// that receives an already-loaded portfolio's JSON and re-marshals
	// it) never has to think about this again - see LoadPortfolio's
	// position as the one true entry point for on-disk data.
	for i := range p.Assets {
		if p.Assets[i].Tags == nil {
			p.Assets[i].Tags = []string{}
		}
	}
	return &p, nil
}

// Save writes the portfolio JSON file at path, first copying the existing
// file to a timestamped backup (if one exists) so a bad write is never
// the only copy.
func Save(path string, p *Portfolio) error {
	if _, err := os.Stat(path); err == nil {
		if err := backupBeforeWrite(path); err != nil {
			return fmt.Errorf("backing up before save: %w", err)
		}
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding portfolio: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing portfolio file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("finalising portfolio file: %w", err)
	}
	return nil
}

func backupBeforeWrite(path string) error {
	dir := filepath.Dir(path)
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	stamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("portfolio-%s.json", stamp))
	return os.WriteFile(backupPath, data, 0644)
}

// FindAssetByISIN returns the first Asset with the given ISIN, if any.
func (p *Portfolio) FindAssetByISIN(isin string) (Asset, bool) {
	for _, a := range p.Assets {
		if a.ISIN != "" && a.ISIN == isin {
			return a, true
		}
	}
	return Asset{}, false
}

// FindAccountByName returns the first Account with the given name under
// the given member, if any.
func (p *Portfolio) FindAccountByName(memberID, name string) (Account, bool) {
	for _, a := range p.Accounts {
		if a.MemberID == memberID && a.Name == name {
			return a, true
		}
	}
	return Account{}, false
}

// GetCapComposition returns the saved cap-wise breakdown for an asset, if
// one has been entered.

// UpsertPrices merges new price records into Prices, replacing any
// existing record for the same (AssetID, Date) pair rather than
// duplicating it. Used when caching fetched NAV/price history, where
// re-running "Update History" should refresh existing dates, not pile
// up duplicates every time.
func (p *Portfolio) UpsertPrices(records []PriceRecord) {
	index := make(map[string]int, len(p.Prices))
	for i, r := range p.Prices {
		index[r.AssetID+"|"+r.Date] = i
	}
	for _, r := range records {
		key := r.AssetID + "|" + r.Date
		if i, exists := index[key]; exists {
			p.Prices[i] = r
		} else {
			p.Prices = append(p.Prices, r)
			index[key] = len(p.Prices) - 1
		}
	}
	p.invalidatePriceIndex()
}

// UpsertFXRates merges new FX rate records the same way, keyed by
// (Currency, Date).
func (p *Portfolio) UpsertFXRates(rates []FXRate) {
	index := make(map[string]int, len(p.FXRates))
	for i, r := range p.FXRates {
		index[r.Currency+"|"+r.Date] = i
	}
	for _, r := range rates {
		key := r.Currency + "|" + r.Date
		if i, exists := index[key]; exists {
			p.FXRates[i] = r
		} else {
			p.FXRates = append(p.FXRates, r)
			index[key] = len(p.FXRates) - 1
		}
	}
	p.invalidateFXIndex()
}

// ensurePriceIndex builds priceIndex (grouped by AssetID, sorted
// ascending by Date) the first time it's needed for this instance, then
// leaves it in place for every subsequent PriceAsOf call - the exact
// access pattern a progression computation has (one instance, hundreds
// of PriceAsOf calls across many checkpoints and assets).
func (p *Portfolio) ensurePriceIndex() {
	if p.priceIndexBuilt {
		return
	}
	index := make(map[string][]PriceRecord, len(p.Assets))
	for _, rec := range p.Prices {
		index[rec.AssetID] = append(index[rec.AssetID], rec)
	}
	for assetID, recs := range index {
		sort.Slice(recs, func(i, j int) bool { return recs[i].Date < recs[j].Date })
		index[assetID] = recs
	}
	p.priceIndex = index
	p.priceIndexBuilt = true
}

func (p *Portfolio) ensureFXIndex() {
	if p.fxIndexBuilt {
		return
	}
	index := make(map[string][]FXRate)
	for _, fx := range p.FXRates {
		index[fx.Currency] = append(index[fx.Currency], fx)
	}
	for currency, rates := range index {
		sort.Slice(rates, func(i, j int) bool { return rates[i].Date < rates[j].Date })
		index[currency] = rates
	}
	p.fxIndex = index
	p.fxIndexBuilt = true
}

// PriceAsOf returns the latest price on or before the given date for an
// asset, from whatever mix of manual entries and fetched history
// (AMFI/TigZig/YAHOO) exists in Prices. Unlike ComputeHoldings' "latest
// price overall" logic, this deliberately ignores anything AFTER the
// given date - it answers "what was this worth on this date", not
// "what is it worth now", which is the whole point of a historical
// progression view. Returns ok=false if no price exists on or before
// the date at all (e.g. before the asset's first NAV).
//
// Backed by a per-asset, date-sorted index (see ensurePriceIndex) with a
// binary search per call, rather than a linear scan of the WHOLE Prices
// slice - the naive version's cost scaled with (checkpoints x included
// assets x TOTAL price records ever stored across the whole portfolio),
// which at a real multi-year, multi-asset scale made computing a
// progression series take tens of seconds. Confirmed via
// internal/benchmark before and after this change - see that package's
// doc comment for the measured numbers.
func (p *Portfolio) PriceAsOf(assetID, date string) (price float64, ok bool) {
	p.ensurePriceIndex()
	recs := p.priceIndex[assetID]
	// First index with Date > target date; everything before it is <=
	// target date, so idx-1 is the latest one on or before it.
	idx := sort.Search(len(recs), func(i int) bool { return recs[i].Date > date })
	if idx == 0 {
		return 0, false
	}
	rec := recs[idx-1]
	return rec.Price, true
}

// FXRateAsOf returns the latest known INR-per-unit rate for a currency
// on or before the given date. For INR itself this is always 1.0,
// trivially, without needing any stored rate. Returns ok=false if no
// rate exists on or before the date (e.g. before the earliest fetched
// FX history).
//
// Same indexed-binary-search approach as PriceAsOf, for the same reason
// - see that method's doc comment.
func (p *Portfolio) FXRateAsOf(currency, date string) (rate float64, ok bool) {
	if currency == "INR" {
		return 1.0, true
	}
	p.ensureFXIndex()
	rates := p.fxIndex[currency]
	idx := sort.Search(len(rates), func(i int) bool { return rates[i].Date > date })
	if idx == 0 {
		return 0, false
	}
	rec := rates[idx-1]
	return rec.INRPerUnit, true
}
func (p *Portfolio) GetCapComposition(assetID string) (CapComposition, bool) {
	for _, c := range p.CapCompositions {
		if c.AssetID == assetID {
			return c, true
		}
	}
	return CapComposition{}, false
}

// SetCapComposition creates or updates the single current cap-composition
// record for an asset (there is only ever one "current" entry per asset,
// not a history - entering a new value simply overwrites the old one,
// matching "defaults to whatever was last saved").
func (p *Portfolio) SetCapComposition(assetID string, large, mid, small, cash float64, asOf, source string) {
	for i := range p.CapCompositions {
		if p.CapCompositions[i].AssetID == assetID {
			p.CapCompositions[i] = CapComposition{AssetID: assetID, Large: large, Mid: mid, Small: small, Cash: cash, AsOf: asOf, Source: source}
			return
		}
	}
	p.CapCompositions = append(p.CapCompositions, CapComposition{AssetID: assetID, Large: large, Mid: mid, Small: small, Cash: cash, AsOf: asOf, Source: source})
}

// SetAssetSymbolAndType updates an asset's Symbol and Type - the
// correction path for a CSV-imported ETF/stock whose auto-inferred
// Symbol needs an exchange suffix added (see bridge.inferInitialSymbol's
// doc comment for why that can't be guessed automatically), or whose
// Type needs fixing. No-op if the asset ID isn't found.
func (p *Portfolio) SetAssetSymbolAndType(assetID, symbol, assetType string) {
	for i := range p.Assets {
		if p.Assets[i].ID == assetID {
			p.Assets[i].Symbol = symbol
			p.Assets[i].Type = assetType
			return
		}
	}
}

// SetAssetETMoneyURL records the ETMoney fund page URL for an asset, so
// its cap composition can later be auto-fetched from that URL. Passing
// an empty url un-links the asset (falls back to manual-only entry).
// No-op if the asset ID isn't found.
func (p *Portfolio) SetAssetETMoneyURL(assetID, url string) {
	for i := range p.Assets {
		if p.Assets[i].ID == assetID {
			p.Assets[i].ETMoneyURL = url
			return
		}
	}
}

// SetAssetGroupLabel records (or clears, if label is "") the fund-group
// label for an asset - see Asset.GroupLabel's doc comment. No-op if the
// asset ID isn't found.
func (p *Portfolio) SetAssetGroupLabel(assetID, label string) {
	for i := range p.Assets {
		if p.Assets[i].ID == assetID {
			p.Assets[i].GroupLabel = label
			return
		}
	}
}

// SetAssetTags replaces the full tag list for an asset - see Asset.Tags'
// doc comment. tags is stored exactly as given, INCLUDING order (the
// caller, not this method, is responsible for appending new tags to the
// end rather than reordering - see Asset.Tags on why order matters for
// EffectiveTag's fallback). A nil tags is normalized to an empty slice,
// same reasoning as Load(). No-op if the asset ID isn't found.
func (p *Portfolio) SetAssetTags(assetID string, tags []string) {
	if tags == nil {
		tags = []string{}
	}
	for i := range p.Assets {
		if p.Assets[i].ID == assetID {
			p.Assets[i].Tags = tags
			return
		}
	}
}

// SetAssetPrimaryTag records (or clears, if tag is "") the pie-chart
// exclusivity override for an asset - see Asset.PrimaryTag's doc
// comment. Deliberately does NOT validate that tag is actually present
// in the asset's current Tags - EffectiveTag already guards against a
// stale override at read time, so a person can freely reorder/remove
// tags without this call needing to happen in lockstep. No-op if the
// asset ID isn't found.
func (p *Portfolio) SetAssetPrimaryTag(assetID, tag string) {
	for i := range p.Assets {
		if p.Assets[i].ID == assetID {
			p.Assets[i].PrimaryTag = tag
			return
		}
	}
}

// AllTags returns every distinct tag currently used by at least one
// asset, sorted alphabetically - used to populate a "pick an existing
// tag" list so the person isn't forced to retype/re-spell a tag they've
// already used elsewhere (e.g. "Mid Cap" vs "Midcap" silently becoming
// two different tags).
func (p *Portfolio) AllTags() []string {
	seen := make(map[string]bool)
	for _, a := range p.Assets {
		for _, t := range a.Tags {
			seen[t] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// GetEquityOriginComposition returns the saved Indian/International
// breakdown for an asset, if one has been entered.
func (p *Portfolio) GetEquityOriginComposition(assetID string) (EquityOriginComposition, bool) {
	for _, c := range p.EquityOriginCompositions {
		if c.AssetID == assetID {
			return c, true
		}
	}
	return EquityOriginComposition{}, false
}

// SetEquityOriginComposition creates or updates the single current
// Indian/International record for an asset (there is only ever one
// "current" entry per asset, not a history, same convention as
// SetCapComposition).
func (p *Portfolio) SetEquityOriginComposition(assetID string, indian, international float64, asOf, source string) {
	for i := range p.EquityOriginCompositions {
		if p.EquityOriginCompositions[i].AssetID == assetID {
			p.EquityOriginCompositions[i] = EquityOriginComposition{AssetID: assetID, Indian: indian, International: international, AsOf: asOf, Source: source}
			return
		}
	}
	p.EquityOriginCompositions = append(p.EquityOriginCompositions, EquityOriginComposition{AssetID: assetID, Indian: indian, International: international, AsOf: asOf, Source: source})
}
