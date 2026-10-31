package bridge

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The all-funds screener bundle is built monthly by the fund-data repo and published as release assets:
//
//	screener.json.gz  the table (every open-ended Direct-Growth fund: category, returns, risk, cap mix)
//	nav.json.gz       weekly NAV history for every fund in the table
//
// The phone downloads both once (about 2 MB) and then screens/compares entirely offline.
var screenerBases = []string{
	"https://github.com/Sabyasachi1988/fund-data/releases/download/screener/",
}

func fetchTo(path, name string) (int64, error) {
	client := &http.Client{Timeout: 120 * time.Second}
	var lastErr error
	for _, b := range screenerBases {
		resp, err := client.Get(b + name)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("status %d for %s", resp.StatusCode, name)
			continue
		}
		tmp := path + ".part"
		f, err := os.Create(tmp)
		if err != nil {
			resp.Body.Close()
			return 0, err
		}
		n, err := io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if err != nil {
			os.Remove(tmp)
			lastErr = err
			continue
		}
		if err := os.Rename(tmp, path); err != nil {
			return 0, err
		}
		return n, nil
	}
	return 0, lastErr
}

// DownloadScreener fetches the bundle into dir (replacing any older copy only when the new one fully arrived).
// Returns {"bytes":N} or {"error":...}.
func DownloadScreener(dir string) string {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	var total int64
	for _, name := range []string{"screener.json.gz", "nav.json.gz"} {
		n, err := fetchTo(filepath.Join(dir, name), name)
		if err != nil {
			return fmt.Sprintf(`{"error":%q}`, "could not download "+name+": "+err.Error())
		}
		total += n
	}
	// the table is read directly by the app, so keep a plain copy of it
	if err := gunzipTo(filepath.Join(dir, "screener.json.gz"), filepath.Join(dir, "screener.json")); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return fmt.Sprintf(`{"bytes":%d}`, total)
}

func gunzipTo(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	zr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer zr.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, zr); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	return os.Rename(tmp, dst)
}

type screenerNavFile struct {
	F map[string][]json.RawMessage `json:"f"`
}

// ScreenerNavSeries returns the weekly NAV series for the given scheme codes (comma separated) from the downloaded
// bundle: {"<code>":{"w0":<first week index>,"nav":[...]}}. Week index = days since 1970 divided by 7.
func ScreenerNavSeries(dir string, codesCSV string) string {
	f, err := os.Open(filepath.Join(dir, "nav.json.gz"))
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, "screener data not downloaded yet")
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	defer zr.Close()
	var nf screenerNavFile
	if err := json.NewDecoder(zr).Decode(&nf); err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	out := map[string]map[string]json.RawMessage{}
	for _, c := range strings.Split(codesCSV, ",") {
		c = strings.TrimSpace(c)
		if v, ok := nf.F[c]; ok && len(v) == 2 {
			out[c] = map[string]json.RawMessage{"w0": v[0], "nav": v[1]}
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(b)
}
