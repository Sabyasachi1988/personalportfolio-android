package com.saby.personalportfolio

import android.app.AlertDialog
import android.os.Bundle
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Locale

class ReturnsDetailActivity : AppCompatActivity() {

    companion object {
        const val EXTRA_SERIES_ID = "series_id"
        const val EXTRA_NAME = "name"
        const val EXTRA_IS_BENCHMARK = "is_benchmark"
    }

    private val gson = Gson()
    private lateinit var seriesId: String
    private lateinit var portfolioJson: String
    private var selectedBenchmarkId: String = "" // empty = let the bridge auto-pick

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_returns_detail)

        seriesId = intent.getStringExtra(EXTRA_SERIES_ID) ?: return
        val name = intent.getStringExtra(EXTRA_NAME) ?: seriesId
        val isBenchmark = intent.getBooleanExtra(EXTRA_IS_BENCHMARK, false)

        val nameView = findViewById<TextView>(R.id.returnsDetailName)
        val scrubbedView = findViewById<TextView>(R.id.returnsDetailScrubbed)
        val chart = findViewById<PriceHistoryChartView>(R.id.returnsDetailChart)
        val chartScrubber = findViewById<ChartRangeScrubberView>(R.id.returnsDetailChartScrubber)
        val emptyState = findViewById<TextView>(R.id.returnsDetailEmptyState)

        nameView.text = name

        val portfolioPath = PortfolioStorage.filePath(this)
        portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val resultJson = Bridge.computePriceHistory(portfolioJson, seriesId)
        val pointType = object : TypeToken<List<PricePoint>>() {}.type
        val points: List<PricePoint> = try {
            gson.fromJson(resultJson, pointType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        if (points.size < 2) {
            chart.visibility = View.GONE
            emptyState.visibility = View.VISIBLE
            emptyState.text = "Not enough history yet to chart. Use Refresh (funds: Update History; " +
                "benchmarks: Manage Benchmarks) to fetch it."
            return
        }

        val displayFormat = SimpleDateFormat("d MMM yyyy", Locale.getDefault())
        val storedFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
        fun formatDisplayDate(stored: String): String = try {
            displayFormat.format(storedFormat.parse(stored) ?: stored)
        } catch (e: Exception) {
            stored
        }
        chart.onPointScrubbed = { windowStartPoint, currentPoint ->
            val displayDate = formatDisplayDate(currentPoint.date)
            val priceText = "$displayDate: ${PricePerUnitFormatter.format(currentPoint.price, decimals = 2)}"
            val cagr = CagrCalculator.compute(windowStartPoint.price, windowStartPoint.date, currentPoint.price, currentPoint.date)
            scrubbedView.text = if (cagr != null && windowStartPoint.date != currentPoint.date) {
                val cagrText = String.format(Locale.getDefault(), "%+.2f%% CAGR", cagr)
                "$priceText\nFrom ${formatDisplayDate(windowStartPoint.date)}: $cagrText"
            } else {
                priceText
            }
        }
        chart.setPoints(points)
        chart.onWindowChanged = { total, start, end -> chartScrubber.setRange(total, start, end) }
        chartScrubber.onRangeDragged = { start, end -> chart.setWindowByIndex(start, end) }

        setUpRangePresets(points, chart)

        findViewById<View>(R.id.returnsDetailSetDateRange).setOnClickListener {
            showDateRangeDialog(points) { start, end -> chart.setWindowByDates(start, end) }
        }

        // Transaction markers (buy/sell dots on the chart) only make
        // sense for a real holding, not a Benchmark - see Benchmark's
        // own Go doc comment (never a portfolio holding, has no
        // transactions at all), so this is skipped entirely for a
        // benchmark row rather than making a call that would just come
        // back empty every time.
        if (!isBenchmark) {
            loadTransactionMarkers(chart)
        }

        // Risk & relative performance metrics only make sense for a FUND
        // (Beta/Info Ratio/Capture need a fund-vs-benchmark comparison; a
        // benchmark compared against itself is meaningless) - Max
        // Drawdown alone would be computable for a benchmark too, but
        // showing a lone metric in an otherwise-empty section reads as
        // broken rather than intentionally partial, so the whole section
        // stays hidden for benchmark rows.
        if (!isBenchmark) {
            findViewById<View>(R.id.returnsDetailMetricsSection).visibility = View.VISIBLE
            findViewById<View>(R.id.returnsDetailChangeBenchmark).setOnClickListener { showBenchmarkPicker() }
            loadMetrics()
        }
    }

    private fun showDateRangeDialog(points: List<PricePoint>, onPicked: (start: String, end: String) -> Unit) {
        if (points.size < 2) return
        DateRangePicker.show(this, points.first().date, points.last().date, onPicked)
    }

    // 3M/6M/1Y/2Y/3Y/Max quick-jump shortcuts alongside the chart's own
    // free pinch-zoom/pan and the ChartRangeScrubberView - built
    // programmatically (all 6 are structurally identical: an ActionChip
    // that computes a start date some fixed offset before the series'
    // LAST point and sets the chart's window to [that date, last date])
    // rather than 6 near-duplicate XML blocks. "Max" is the one
    // non-offset case - it just resets to the whole series, same as the
    // chart's own double-tap-to-reset gesture.
    private fun setUpRangePresets(points: List<PricePoint>, chart: PriceHistoryChartView) {
        if (points.size < 2) return
        val container = findViewById<LinearLayout>(R.id.returnsDetailRangePresets)
        val lastDate = points.last().date
        val firstDate = points.first().date
        val presets = listOf(
            "3M" to 3, "6M" to 6, "1Y" to 12, "2Y" to 24, "3Y" to 36
        )
        presets.forEach { (label, monthsBack) ->
            container.addView(buildPresetChip(label) {
                chart.setWindowByDates(dateMonthsBefore(lastDate, monthsBack), lastDate)
            })
        }
        container.addView(buildPresetChip("Max") {
            chart.setWindowByDates(firstDate, lastDate)
        })
    }

    private fun buildPresetChip(label: String, onClick: () -> Unit): TextView {
        val chip = TextView(this, null, 0, R.style.ActionChip)
        chip.text = label
        chip.layoutParams = LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT
        ).apply { marginEnd = dpToPx(8) }
        chip.setOnClickListener { onClick() }
        return chip
    }

    private fun dpToPx(dp: Int): Int = (dp * resources.displayMetrics.density).toInt()

    // storedDate is "yyyy-MM-dd" (this app's stored-date convention
    // throughout - see PriceHistoryChartView's own dateStoredFormat).
    // Falls back to storedDate itself if parsing ever fails, so a
    // preset tap degrades to "show everything from the start" rather
    // than crashing.
    private fun dateMonthsBefore(storedDate: String, months: Int): String {
        val fmt = SimpleDateFormat("yyyy-MM-dd", Locale.US)
        return try {
            val cal = Calendar.getInstance()
            cal.time = fmt.parse(storedDate) ?: return storedDate
            cal.add(Calendar.MONTH, -months)
            fmt.format(cal.time)
        } catch (e: Exception) {
            storedDate
        }
    }

    private fun loadTransactionMarkers(chart: PriceHistoryChartView) {
        val resultJson = Bridge.computeAssetTransactionMarkers(portfolioJson, seriesId)
        if (isBridgeError(resultJson)) return
        val markerType = object : TypeToken<List<TransactionMarker>>() {}.type
        val markers: List<TransactionMarker> = try {
            gson.fromJson(resultJson, markerType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }
        if (markers.isEmpty()) return
        chart.setMarkers(markers)
        val popup = findViewById<AutoDismissPopupView>(R.id.returnsDetailMarkerPopup)
        // Quick tap: shows near the touched point, lingers briefly on
        // its own. Press-and-hold: shows near the point with NO timer
        // while held, then lingers the same short duration once the
        // finger lifts - see AutoDismissPopupView's own doc comment for
        // why two entry points exist.
        chart.onMarkerTapped = { marker, x, y -> showMarkerPopup(popup, marker, x, y, persistent = false) }
        chart.onMarkerHoldStart = { marker, x, y -> showMarkerPopup(popup, marker, x, y, persistent = true) }
        chart.onMarkerHoldEnd = { popup.dismissAfterLinger() }
    }

    // Only Date/NAV/Units/Amount are shown - a transaction's raw
    // Description (e.g. "Sys. Investment ISIP (11/28)") was deliberately
    // dropped after explicit feedback that it wasn't meaningful
    // information for this popup, unlike the other 4 fields.
    private fun showMarkerPopup(popup: AutoDismissPopupView, marker: TransactionMarker, x: Float, y: Float, persistent: Boolean) {
        val displayFormat = SimpleDateFormat("d MMM yyyy", Locale.getDefault())
        val storedFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
        val displayDate = try {
            displayFormat.format(storedFormat.parse(marker.date) ?: marker.date)
        } catch (e: Exception) {
            marker.date
        }
        val title = if (marker.isBuy) "Buy" else "Sell"
        val accentColorRes = if (marker.isBuy) R.color.colorAmber else R.color.colorLoss
        val message = "Date: $displayDate\n" +
            "NAV: ${PricePerUnitFormatter.format(marker.price, decimals = 3)}\n" +
            "Units: ${String.format(Locale.getDefault(), "%.3f", marker.units)}\n" +
            "Amount: ${IndianCurrencyFormatter.format(marker.amount)}"
        if (persistent) {
            popup.showPersistent(title, message, accentColorRes, x, y)
        } else {
            popup.show(title, message, accentColorRes, x, y)
        }
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadMetrics() {
        val resultJson = Bridge.computeFundMetrics(portfolioJson, seriesId, selectedBenchmarkId)
        if (isBridgeError(resultJson)) return
        val metrics: FundMetricsResult = try {
            gson.fromJson(resultJson, FundMetricsResult::class.java)
        } catch (e: Exception) {
            return
        }
        bindMetrics(metrics)
    }

    private fun bindMetrics(m: FundMetricsResult) {
        val benchmarkLabel = findViewById<TextView>(R.id.returnsDetailBenchmarkLabel)
        if (m.benchmarkId.isNullOrEmpty()) {
            benchmarkLabel.text = "No benchmark to compare against yet — tap Change to pick one"
        } else {
            val suffix = if (m.autoSelected) " (auto)" else ""
            benchmarkLabel.text = "Compared against: ${m.benchmarkName}$suffix"
        }

        // Beta has no inherent "good/bad" direction (it's a market-
        // sensitivity measure, not a performance one) so it stays
        // neutral - every other card is colored by what a favorable
        // reading actually means for THAT metric, not just "positive =
        // green": Down Capture and Max Drawdown are both metrics where
        // a LOWER number is the good outcome, the opposite of the other
        // 5, so each card's threshold is spelled out at its own call
        // site rather than one blanket "value >= 0" rule that would
        // color Down Capture backwards.
        val cards = listOf(
            MetricCardSpec("Beta", m.beta, m.betaHasData, decimals = 2, suffix = "", colorRes = R.color.colorOnSurface),
            MetricCardSpec("Alpha", m.alpha, m.alphaHasData, decimals = 2, suffix = "%", colorRes = if (m.alpha >= 0) R.color.colorGain else R.color.colorLoss),
            MetricCardSpec("Information Ratio", m.informationRatio, m.infoRatioHasData, decimals = 2, suffix = "", colorRes = if (m.informationRatio >= 0) R.color.colorGain else R.color.colorLoss),
            MetricCardSpec("Std. Deviation", m.standardDeviation, m.stdDevHasData, decimals = 2, suffix = "%", colorRes = R.color.colorOnSurface),
            MetricCardSpec("Up Capture", m.upCapture, m.upCaptureHasData, decimals = 2, suffix = "%", colorRes = if (m.upCapture >= 100) R.color.colorGain else R.color.colorLoss),
            MetricCardSpec("Down Capture", m.downCapture, m.downCaptureHasData, decimals = 2, suffix = "%", colorRes = if (m.downCapture <= 100) R.color.colorGain else R.color.colorLoss),
            MetricCardSpec("Max Drawdown", m.maxDrawdown, m.maxDrawdownHasData, decimals = 2, suffix = "%", colorRes = R.color.colorLoss),
            MetricCardSpec("Sharpe Ratio", m.sharpeRatio, m.sharpeHasData, decimals = 2, suffix = "", colorRes = if (m.sharpeRatio >= 0) R.color.colorGain else R.color.colorLoss),
            MetricCardSpec("Sortino Ratio", m.sortinoRatio, m.sortinoHasData, decimals = 2, suffix = "", colorRes = if (m.sortinoRatio >= 0) R.color.colorGain else R.color.colorLoss)
        )
        buildMetricCardGrid(cards)
    }

    private data class MetricCardSpec(
        val label: String, val value: Double, val hasData: Boolean, val decimals: Int, val suffix: String, val colorRes: Int
    )

    // 2-per-row grid of small cards, replacing the old flat "Label:
    // value" list - see the XML container's own doc comment for why
    // this is built programmatically rather than as static XML blocks.
    private fun buildMetricCardGrid(cards: List<MetricCardSpec>) {
        val grid = findViewById<LinearLayout>(R.id.returnsDetailMetricsGrid)
        grid.removeAllViews()
        cards.chunked(2).forEach { rowCards ->
            val row = LinearLayout(this).apply {
                orientation = LinearLayout.HORIZONTAL
                layoutParams = LinearLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT
                ).apply { topMargin = dpToPx(8) }
            }
            rowCards.forEachIndexed { i, spec ->
                row.addView(buildMetricCard(spec).apply {
                    layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f).apply {
                        if (i > 0) marginStart = dpToPx(8)
                    }
                })
            }
            // An odd final row (7 cards = 3 rows of 2 + 1) gets an
            // invisible spacer in the second slot, so the lone card
            // stays HALF width like every other card rather than
            // stretching to fill the row - a stretched final card would
            // look like a mistake, not a deliberate layout.
            if (rowCards.size == 1) {
                row.addView(View(this).apply {
                    layoutParams = LinearLayout.LayoutParams(0, 0, 1f).apply { marginStart = dpToPx(8) }
                })
            }
            grid.addView(row)
        }
    }

    private fun buildMetricCard(spec: MetricCardSpec): LinearLayout {
        val card = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            background = ContextCompat.getDrawable(this@ReturnsDetailActivity, R.drawable.dropdown_tab_background)
            val pad = dpToPx(12)
            setPadding(pad, pad, pad, pad)
        }
        val labelView = TextView(this).apply {
            text = spec.label
            textSize = 12f
            setTextColor(ContextCompat.getColor(this@ReturnsDetailActivity, R.color.colorNeutral))
        }
        val valueView = TextView(this).apply {
            textSize = 18f
            setTypeface(typeface, android.graphics.Typeface.BOLD)
            layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply {
                topMargin = dpToPx(2)
            }
            if (spec.hasData) {
                // Same UnknownFormatConversionException landmine as
                // before - see the original bindMetricRow's own note:
                // a bare "%" inside a format STRING is invalid, so the
                // number is formatted alone and the suffix concatenated
                // as plain text afterward.
                text = String.format(Locale.getDefault(), "%.${spec.decimals}f", spec.value) + spec.suffix
                setTextColor(ContextCompat.getColor(this@ReturnsDetailActivity, spec.colorRes))
            } else {
                text = "—"
                setTextColor(ContextCompat.getColor(this@ReturnsDetailActivity, R.color.colorNeutral))
            }
        }
        card.addView(labelView)
        card.addView(valueView)
        return card
    }

    /**
     * Lets the person override the auto-picked benchmark - reads the
     * same PortfolioBenchmarksSnapshot slice BenchmarksActivity uses, so
     * this only ever offers benchmarks the person has actually added
     * (see Settings → Manage Benchmarks), never a hypothetical index
     * they haven't tracked yet.
     *
     * Uses AlertDialog's own single-choice list (setSingleChoiceItems)
     * rather than PopupMenu - PopupMenu's own item-index-based click
     * handler was the ORIGINAL implementation here and reportedly
     * crashed the Activity on tap (a confirmed but not fully root-
     * caused bug; the "Auto-pick" entry used a NEGATIVE menu item ID,
     * -1, which every other PopupMenu in this codebase avoids -
     * ProgressionActivity/ReturnsActivity's pickers always start IDs at
     * 0). AlertDialog.setSingleChoiceItems is the same proven pattern
     * already used successfully elsewhere in this codebase (Settings,
     * Holdings, the Compare picker) and sidesteps the question entirely
     * by using plain array indices, never a custom/negative ID.
     */
    private fun showBenchmarkPicker() {
        val snapshot: PortfolioBenchmarksSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioBenchmarksSnapshot::class.java)
        } catch (e: Exception) {
            null
        } ?: PortfolioBenchmarksSnapshot(emptyList(), emptyList())
        val benchmarks = snapshot.benchmarks ?: emptyList()
        if (benchmarks.isEmpty()) return

        val labels = (listOf("Auto-pick (recommended)") + benchmarks.map { it.name }).toTypedArray()
        val currentIndex = if (selectedBenchmarkId.isEmpty()) {
            0
        } else {
            benchmarks.indexOfFirst { it.id == selectedBenchmarkId }.let { if (it < 0) 0 else it + 1 }
        }
        AlertDialog.Builder(this)
            .setTitle("Compare against")
            .setSingleChoiceItems(labels, currentIndex) { dialog, which ->
                selectedBenchmarkId = if (which == 0) "" else benchmarks[which - 1].id
                loadMetrics()
                dialog.dismiss()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }
}
