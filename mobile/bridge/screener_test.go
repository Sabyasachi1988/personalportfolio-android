package bridge

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScreenerNavSeries(t *testing.T) {
	dir := t.TempDir()
	f, _ := os.Create(filepath.Join(dir, "nav.json.gz"))
	zw := gzip.NewWriter(f)
	zw.Write([]byte(`{"asof":"x","f":{"1":[2900,[10,11,12]],"2":[2901,[5,6]]}}`))
	zw.Close()
	f.Close()
	got := ScreenerNavSeries(dir, "2, 9")
	if !strings.Contains(got, `"2":{"nav":[5,6],"w0":2901}`) || strings.Contains(got, `"1"`) || strings.Contains(got, `"9"`) {
		t.Fatalf("unexpected: %s", got)
	}
	if !strings.Contains(ScreenerNavSeries(t.TempDir(), "1"), "error") {
		t.Fatal("missing bundle should be an error")
	}
}
