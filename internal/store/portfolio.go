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
	Source      string // "MANUAL", "CAS_IMPORT", "CSV_IMPORT"
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
type Portfolio struct {
	Members         []Member
	Accounts        []Account
	Assets          []Asset
	Transactions    []StoredTransaction
	Prices          []PriceRecord
	CapCompositions []CapComposition
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
