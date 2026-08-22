package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"ledger/internal/finance"
	"ledger/internal/store"
)

type chartPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

// generateHTMLReport writes a self-contained HTML file (Chart.js loaded
// from CDN) with allocation charts, a gain/loss chart, and a sortable
// table, and returns its path. classByAsset maps AssetID -> AMFI/SEBI
// asset class (may be sparse; missing entries render as "Unclassified").
func generateHTMLReport(holdings []finance.Holding, invested, value float64, classByAsset map[string]string, compositionByAsset map[string]store.CapComposition, xirr float64, hasXIRR bool) (string, error) {
	var allocationByFund []chartPoint
	var gains []chartPoint
	for _, h := range holdings {
		if !h.HasPrice {
			continue
		}
		allocationByFund = append(allocationByFund, chartPoint{Label: h.AssetName, Value: h.CurrentValue})
		gains = append(gains, chartPoint{Label: h.AssetName, Value: h.Gain})
	}

	classSlices := finance.AllocationByAssetClass(holdings, classByAsset)
	segmentSlices := finance.AllocationByMarketCapSegment(holdings, compositionByAsset)

	fundJSON, err := marshalPoints(allocationByFund)
	if err != nil {
		return "", err
	}
	gainsJSON, err := marshalPoints(gains)
	if err != nil {
		return "", err
	}
	classJSON, err := json.Marshal(classSlices)
	if err != nil {
		return "", err
	}
	segmentJSON, err := json.Marshal(segmentSlices)
	if err != nil {
		return "", err
	}

	var rows strings.Builder
	for _, h := range holdings {
		priceCell, valueCell, gainCell, gainPctCell, xirrCell := "-", "-", "-", "-", "-"
		if h.HasPrice {
			priceCell = fmt.Sprintf("%.4f", h.CurrentPrice)
			valueCell = fmt.Sprintf("%.2f", h.CurrentValue)
			gainCell = fmt.Sprintf("%.2f", h.Gain)
			gainPctCell = fmt.Sprintf("%.2f%%", h.GainPercent)
		}
		if h.HasXIRR {
			xirrCell = fmt.Sprintf("%.2f%%", h.XIRR)
		}
		gainClass := "neutral"
		if h.HasPrice {
			if h.Gain > 0 {
				gainClass = "pos"
			} else if h.Gain < 0 {
				gainClass = "neg"
			}
		}
		fmt.Fprintf(&rows, `<tr><td>%s</td><td>%s</td><td class="num">%.4f</td><td class="num">%.2f</td><td class="num">%s</td><td class="num">%s</td><td class="num %s">%s</td><td class="num %s">%s</td><td class="num">%s</td></tr>`+"\n",
			esc(h.AssetName), esc(h.AccountName), h.UnitsHeld, h.NetInvested, priceCell, valueCell, gainClass, gainCell, gainClass, gainPctCell, xirrCell)
	}

	gainTotal := value - invested
	gainPct := 0.0
	if invested != 0 {
		gainPct = gainTotal / invested * 100
	}
	gainSummaryClass := "neutral"
	if gainTotal > 0 {
		gainSummaryClass = "pos"
	} else if gainTotal < 0 {
		gainSummaryClass = "neg"
	}

	xirrStat := "-"
	if hasXIRR {
		xirrStat = fmt.Sprintf("%.2f%%", xirr)
	}

	full := fmt.Sprintf(reportTemplate,
		invested, value, gainSummaryClass, gainTotal, gainPct, xirrStat,
		allocationBarHTML(classSlices),
		legendHTML(classSlices), legendHTML(segmentSlices),
		rows.String(),
		fundJSON, gainsJSON, classJSON, segmentJSON,
	)

	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	reportDir := filepath.Join(dir, "PersonalPortfolio")
	_ = os.MkdirAll(reportDir, 0755)
	path := filepath.Join(reportDir, "portfolio-report.html")

	if err := os.WriteFile(path, []byte(full), 0644); err != nil {
		return "", err
	}
	return path, nil
}

func marshalPoints(points []chartPoint) (string, error) {
	if points == nil {
		points = []chartPoint{}
	}
	b, err := json.Marshal(points)
	return string(b), err
}

// paletteColors is a fixed, readable qualitative palette used consistently
// across every chart and the HTML legends, so a given slice's color means
// the same thing everywhere in the report.
var paletteColors = []string{
	"#2B2E6B", "#5B6FE0", "#157A50", "#2FA98C", "#C77D2E",
	"#B23B2E", "#8E5BC7", "#3D8FBF", "#B08A2E", "#6B6F80",
}

func colorFor(i int) string {
	return paletteColors[i%len(paletteColors)]
}

// allocationBarHTML renders the signature horizontal stacked-segment bar:
// one bar, segmented proportionally by asset class, each segment showing
// its own percentage directly (rather than relying on a separate legend
// to convey composition).
func allocationBarHTML(slices []finance.AllocationSlice) string {
	if len(slices) == 0 {
		return `<div class="alloc-empty">No priced holdings yet.</div>`
	}
	var b strings.Builder
	b.WriteString(`<div class="alloc-bar">`)
	for i, s := range slices {
		if s.Percent < 0.5 {
			continue // too thin to render a readable label; still counted in the legend below
		}
		fmt.Fprintf(&b, `<div class="alloc-seg" style="width:%.4f%%;background:%s" title="%s: %.2f%%"><span>%s %.0f%%</span></div>`,
			s.Percent, colorFor(i), esc(s.Label), s.Percent, esc(s.Label), s.Percent)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// legendHTML renders a plain HTML/CSS legend (not Chart.js's canvas-drawn
// legend) so fund/category names never get clipped or hidden at
// different zoom levels - text in the DOM reflows normally instead of
// being laid out inside a fixed canvas.
func legendHTML(slices []finance.AllocationSlice) string {
	var b strings.Builder
	for i, s := range slices {
		fmt.Fprintf(&b, `<div class="legend-row"><span class="swatch" style="background:%s"></span><span class="legend-label">%s</span><span class="legend-pct">%.2f%%</span></div>`,
			colorFor(i), esc(s.Label), s.Percent)
	}
	return b.String()
}

func esc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

func openInBrowser(path string) error {
	cmd := exec.Command("cmd", "/c", "start", "", path)
	return cmd.Start()
}

const reportTemplate = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Portfolio Report</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=IBM+Plex+Mono:wght@400;500;600&display=swap" rel="stylesheet">
<script src="https://cdnjs.cloudflare.com/ajax/libs/Chart.js/4.4.0/chart.umd.min.js"></script>
<style>
  :root {
    --bg: #F7F8FA;
    --surface: #FFFFFF;
    --ink: #12172B;
    --muted: #5B6178;
    --line: #E4E7EE;
    --accent: #2B2E6B;
    --pos: #157A50;
    --neg: #B23B2E;
  }
  * { box-sizing: border-box; }
  body {
    font-family: 'Inter', -apple-system, Segoe UI, sans-serif;
    margin: 0; padding: 32px 40px 56px;
    color: var(--ink); background: var(--bg);
  }
  h1 { font-size: 22px; font-weight: 700; margin: 0 0 4px; letter-spacing: -0.01em; }
  .subtitle { color: var(--muted); font-size: 13px; margin-bottom: 24px; }
  .num, .stat-val, td.num, .legend-pct { font-family: 'IBM Plex Mono', ui-monospace, monospace; font-variant-numeric: tabular-nums; }

  .stat-strip { display: flex; gap: 16px; margin-bottom: 20px; flex-wrap: wrap; }
  .stat-card {
    background: var(--surface); border: 1px solid var(--line); border-radius: 10px;
    padding: 14px 20px; min-width: 150px;
  }
  .stat-label { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.04em; margin-bottom: 4px; }
  .stat-val { font-size: 21px; font-weight: 600; }

  .alloc-bar {
    display: flex; width: 100%%; height: 34px; border-radius: 8px; overflow: hidden;
    margin-bottom: 24px; border: 1px solid var(--line);
  }
  .alloc-seg {
    display: flex; align-items: center; justify-content: center;
    color: white; font-size: 11px; font-weight: 600; white-space: nowrap; overflow: hidden;
    border-right: 1px solid rgba(255,255,255,0.25);
  }
  .alloc-seg span { padding: 0 6px; text-overflow: ellipsis; overflow: hidden; }
  .alloc-empty { color: var(--muted); font-size: 13px; margin-bottom: 24px; }

  .charts { display: flex; gap: 20px; margin-bottom: 24px; flex-wrap: wrap; }
  .chart-box {
    background: var(--surface); border: 1px solid var(--line); border-radius: 10px;
    padding: 18px; flex: 1; min-width: 320px;
  }
  .chart-title { font-size: 13px; font-weight: 600; margin-bottom: 4px; }
  .chart-note { font-size: 11px; color: var(--muted); margin-bottom: 12px; }
  .chart-canvas-wrap { position: relative; height: 220px; margin-bottom: 12px; }

  .legend-row { display: flex; align-items: center; gap: 8px; padding: 3px 0; font-size: 12px; }
  .swatch { width: 10px; height: 10px; border-radius: 2px; flex-shrink: 0; }
  .legend-label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .legend-pct { color: var(--muted); }

  table { border-collapse: collapse; width: 100%%; background: var(--surface); border: 1px solid var(--line); border-radius: 10px; overflow: hidden; }
  th, td { padding: 9px 12px; border-bottom: 1px solid var(--line); text-align: left; font-size: 13px; }
  th { background: #FAFAFC; font-weight: 600; font-size: 11px; text-transform: uppercase; letter-spacing: 0.03em; color: var(--muted); cursor: pointer; user-select: none; }
  td.num { text-align: right; }
  .pos { color: var(--pos); }
  .neg { color: var(--neg); }
</style>
</head>
<body>
<h1>Portfolio Report</h1>
<div class="subtitle">Generated from your local portfolio data. Nothing here is sent anywhere.</div>

<div class="stat-strip">
  <div class="stat-card"><div class="stat-label">Invested</div><div class="stat-val">%.2f</div></div>
  <div class="stat-card"><div class="stat-label">Current Value</div><div class="stat-val">%.2f</div></div>
  <div class="stat-card"><div class="stat-label">Gain</div><div class="stat-val %s">%.2f (%.2f%%)</div></div>
  <div class="stat-card"><div class="stat-label">Cumulative XIRR</div><div class="stat-val">%s</div></div>
</div>

%s

<div class="charts">
  <div class="chart-box">
    <div class="chart-title">Allocation by fund</div>
    <div class="chart-note">Share of current value</div>
    <div class="chart-canvas-wrap"><canvas id="fundChart"></canvas></div>
  </div>
  <div class="chart-box">
    <div class="chart-title">Gain / loss per fund</div>
    <div class="chart-note">Absolute gain, current currency</div>
    <div class="chart-canvas-wrap"><canvas id="gainChart"></canvas></div>
  </div>
</div>

<div class="charts">
  <div class="chart-box">
    <div class="chart-title">Allocation by asset class</div>
    <div class="chart-note">Official AMFI/SEBI category. Index funds and ETFs all fall under "Other" here regardless of what they track.</div>
    <div class="chart-canvas-wrap"><canvas id="classChart"></canvas></div>
    <div class="legend-list">%s</div>
  </div>
  <div class="chart-box">
    <div class="chart-title">Allocation by market-cap segment</div>
    <div class="chart-note">Inferred from fund names (not an official AMFI category) - built to answer large/mid/small-cap mix for index-heavy portfolios.</div>
    <div class="chart-canvas-wrap"><canvas id="segmentChart"></canvas></div>
    <div class="legend-list">%s</div>
  </div>
</div>

<table id="holdingsTable">
<thead><tr>
<th onclick="sortTable(0)">Fund</th><th onclick="sortTable(1)">Account</th>
<th onclick="sortTable(2)">Units</th><th onclick="sortTable(3)">Invested</th>
<th onclick="sortTable(4)">Price</th><th onclick="sortTable(5)">Value</th>
<th onclick="sortTable(6)">Gain</th><th onclick="sortTable(7)">Gain %%</th><th onclick="sortTable(8)">XIRR</th>
</tr></thead>
<tbody>
%s
</tbody>
</table>

<script>
const palette = ['#2B2E6B','#5B6FE0','#157A50','#2FA98C','#C77D2E','#B23B2E','#8E5BC7','#3D8FBF','#B08A2E','#6B6F80'];
const fundData = %s;
const gainData = %s;
const classData = %s;
const segmentData = %s;

function pieOptions(values) {
  const total = values.reduce((s, v) => s + Math.abs(v), 0);
  return {
    maintainAspectRatio: false,
    plugins: {
      legend: { display: false },
      tooltip: {
        callbacks: {
          label: (ctx) => {
            const v = ctx.parsed;
            const pct = total ? (v / total * 100).toFixed(2) : '0.00';
            return ctx.label + ': ' + v.toLocaleString(undefined, {maximumFractionDigits: 2}) + ' (' + pct + '%%)';
          }
        }
      }
    }
  };
}

function makePie(id, data, labelKey, valueKey) {
  const values = data.map(d => d[valueKey]);
  new Chart(document.getElementById(id), {
    type: 'doughnut',
    data: {
      labels: data.map(d => d[labelKey]),
      datasets: [{ data: values, backgroundColor: data.map((_, i) => palette[i %% palette.length]) }]
    },
    options: pieOptions(values)
  });
}

makePie('fundChart', fundData, 'label', 'value');
makePie('classChart', classData, 'Label', 'Value');
makePie('segmentChart', segmentData, 'Label', 'Value');

new Chart(document.getElementById('gainChart'), {
  type: 'bar',
  data: {
    labels: gainData.map(g => g.label),
    datasets: [{
      data: gainData.map(g => g.value),
      backgroundColor: gainData.map(g => g.value >= 0 ? '#157A50' : '#B23B2E')
    }]
  },
  options: {
    maintainAspectRatio: false,
    indexAxis: 'y',
    plugins: { legend: { display: false } }
  }
});

function sortTable(col) {
  const table = document.getElementById('holdingsTable');
  const tbody = table.tBodies[0];
  const rows = Array.from(tbody.rows);
  const numeric = col >= 2;
  const dir = table.dataset.sortCol == col && table.dataset.sortDir === 'asc' ? 'desc' : 'asc';
  rows.sort((a, b) => {
    let av = a.cells[col].innerText, bv = b.cells[col].innerText;
    if (numeric) { av = parseFloat(av) || 0; bv = parseFloat(bv) || 0; }
    if (av < bv) return dir === 'asc' ? -1 : 1;
    if (av > bv) return dir === 'asc' ? 1 : -1;
    return 0;
  });
  rows.forEach(r => tbody.appendChild(r));
  table.dataset.sortCol = col;
  table.dataset.sortDir = dir;
}
</script>
</body>
</html>`
