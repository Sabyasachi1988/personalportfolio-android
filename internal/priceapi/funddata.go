package priceapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// The fund-data repo (public) publishes per-fund monthly cap/sector splits. Primary host is GitHub raw;
// jsDelivr is the fallback. No key or token is involved.
var fundDataBases = []string{
	"https://raw.githubusercontent.com/Sabyasachi1988/fund-data/main/pub/",
	"https://cdn.jsdelivr.net/gh/Sabyasachi1988/fund-data@main/pub/",
}

type FundDataManifest struct {
	Version string `json:"version"`
	Latest  string `json:"latest"`
	Funds   int    `json:"funds"`
	Months  int    `json:"months"`
}

type FundDataMonth struct {
	S []float64          `json:"s"`
	X map[string]float64 `json:"x"`
	E int                `json:"e"`
}

type FundDataFile struct {
	Pid    string                   `json:"pid"`
	Name   string                   `json:"name"`
	AMC    string                   `json:"amc"`
	Months map[string]FundDataMonth `json:"months"`
}

func fundDataGet(rel string, out interface{}) error {
	client := &http.Client{Timeout: 20 * time.Second}
	var lastErr error
	for _, base := range fundDataBases {
		resp, err := client.Get(base + rel)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusNotFound {
			return errFundNotFound
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			continue
		}
		if err := json.Unmarshal(body, out); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("fund-data unreachable: %w", lastErr)
}

var errFundNotFound = fmt.Errorf("not found in fund-data")

func IsFundNotFound(err error) bool { return err == errFundNotFound }

func FetchFundDataManifest() (FundDataManifest, error) {
	var m FundDataManifest
	err := fundDataGet("manifest.json", &m)
	return m, err
}

func FetchFundDataFile(pid string) (FundDataFile, error) {
	var f FundDataFile
	err := fundDataGet("funds/"+pid+".json", &f)
	return f, err
}

var (
	codesMu      sync.Mutex
	codesData    map[string]string
	codesVersion string
)

// ResolveFundPid maps an ISIN to the fund-data portfolio id: ISIN -> AMFI scheme code (mfapi list)
// -> pid (codes.json). version identifies the codes.json generation so a stale in-memory copy is refreshed.
func ResolveFundPid(isin, version string) (string, error) {
	code, err := ResolveMfapiSchemeCode(isin)
	if err != nil {
		return "", err
	}
	codesMu.Lock()
	defer codesMu.Unlock()
	if codesData == nil || codesVersion != version {
		m := map[string]string{}
		if err := fundDataGet("codes.json", &m); err != nil {
			return "", err
		}
		codesData, codesVersion = m, version
	}
	pid, ok := codesData[strconv.Itoa(code)]
	if !ok {
		return "", errFundNotFound
	}
	return pid, nil
}
