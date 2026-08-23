package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	ETMoneyURL string `json:",omitempty"` // full ETMoney fund page URL (e.g. https://www.etmoney.com/mutual-funds/<slug>/<id>), entered once so cap composition can be auto-fetched from it; empty means "not linked, manual entry only"
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
	Source      string // "MANUAL", "CAS_IMPORT", "CSV_IMPORT"
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
}

// PriceAsOf returns the latest price on or before the given date for an
// asset, from whatever mix of manual entries and fetched history
// (AMFI/TigZig/YAHOO) exists in Prices. Unlike ComputeHoldings' "latest
// price overall" logic, this deliberately ignores anything AFTER the
// given date - it answers "what was this worth on this date", not
// "what is it worth now", which is the whole point of a historical
// progression view. Returns ok=false if no price exists on or before
// the date at all (e.g. before the asset's first NAV).
func (p *Portfolio) PriceAsOf(assetID, date string) (price float64, ok bool) {
	bestDate := ""
	for _, rec := range p.Prices {
		if rec.AssetID != assetID {
			continue
		}
		if rec.Date > date {
			continue
		}
		if rec.Date > bestDate {
			bestDate = rec.Date
			price = rec.Price
			ok = true
		}
	}
	return price, ok
}

// FXRateAsOf returns the latest known INR-per-unit rate for a currency
// on or before the given date. For INR itself this is always 1.0,
// trivially, without needing any stored rate. Returns ok=false if no
// rate exists on or before the date (e.g. before the earliest fetched
// FX history).
func (p *Portfolio) FXRateAsOf(currency, date string) (rate float64, ok bool) {
	if currency == "INR" {
		return 1.0, true
	}
	bestDate := ""
	for _, fx := range p.FXRates {
		if fx.Currency != currency {
			continue
		}
		if fx.Date > date {
			continue
		}
		if fx.Date > bestDate {
			bestDate = fx.Date
			rate = fx.INRPerUnit
			ok = true
		}
	}
	return rate, ok
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
