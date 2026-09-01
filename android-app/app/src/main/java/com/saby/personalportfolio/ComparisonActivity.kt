package com.saby.personalportfolio

import android.app.AlertDialog
import android.graphics.Typeface
import android.os.Bundle
import android.text.Editable
import android.text.SpannableStringBuilder
import android.text.TextWatcher
import android.text.style.ForegroundColorSpan
import android.text.style.StyleSpan
import android.view.View
import android.widget.ArrayAdapter
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.util.Locale

class ComparisonActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var pickButton: TextView
    private lateinit var lockSwitch: com.google.android.material.switchmaterial.SwitchMaterial
    private lateinit var chart: OverlayChartView
    private lateinit var chartScrubber: ChartRangeScrubberView
    private lateinit var scrubbedView: TextView
    private lateinit var emptyState: TextView
    private lateinit var tabGraph: TextView
    private lateinit var tabTable: TextView
    private lateinit var graphContainer: View
    private lateinit var tableContainer: View
    private lateinit var tableRoot: LinearLayout
    private var activeTab: Tab = Tab.GRAPH

    private enum class Tab { GRAPH, TABLE }

    private var rows: List<ReturnsTableRow> = emptyList()
    private val selectedSeriesIds = linkedSetOf<String>()
    private var currentUnionDateBounds: Pair<String, String>? = null // (min, max) across the currently charted series

    // Same 14-color palette already used for pie-chart slices (see
    // colors.xml) - cycled if more than 14 series are selected, which
    // in practice never happens (the picker has far fewer items than
    // that), but the cycle avoids an out-of-bounds crash regardless.
    private val paletteColorIds = listOf(
        R.color.chartSlice1, R.color.chartSlice2, R.color.chartSlice3, R.color.chartSlice4,
        R.color.chartSlice5, R.color.chartSlice6, R.color.chartSlice7, R.color.chartSlice8,
        R.color.chartSlice9, R.color.chartSlice10, R.color.chartSlice11, R.color.chartSlice12,
        R.color.chartSlice13, R.color.chartSlice14
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_comparison)

        pickButton = findViewById(R.id.comparisonPickButton)
        lockSwitch = findViewById(R.id.comparisonLockSwitch)
        chart = findViewById(R.id.comparisonChart)
        chartScrubber = findViewById(R.id.comparisonChartScrubber)
        scrubbedView = findViewById(R.id.comparisonScrubbed)
        emptyState = findViewById(R.id.comparisonEmptyState)
        tabGraph = findViewById(R.id.comparisonTabGraph)
        tabTable = findViewById(R.id.comparisonTabTable)
        graphContainer = findViewById(R.id.comparisonGraphContainer)
        tableContainer = findViewById(R.id.comparisonTableContainer)
        tableRoot = findViewById(R.id.comparisonTableRoot)

        tabGraph.setOnClickListener { switchTab(Tab.GRAPH) }
        tabTable.setOnClickListener { switchTab(Tab.TABLE) }
        updateTabStyling()

        pickButton.setOnClickListener { showPicker() }
        lockSwitch.setOnCheckedChangeListener { _, checked -> chart.setLockBaseDate(checked) }
        chart.onWindowChanged = { total, start, end -> chartScrubber.setRange(total, start, end) }
        chartScrubber.onRangeDragged = { start, end -> chart.setWindowByIndex(start, end) }
        findViewById<View>(R.id.comparisonSetDateRange).setOnClickListener {
            val bounds = currentUnionDateBounds ?: return@setOnClickListener
            DateRangePicker.show(this, bounds.first, bounds.second) { start, end -> chart.setWindowByDates(start, end) }
        }

        chart.onScrubbed = { date, values ->
            scrubbedView.text = buildLegendText(date, values)
        }

        loadRows()
    }

    /**
     * One line per series, each prefixed with a colored "●" matching its
     * line color on the chart, so which line is which reads at a glance
     * instead of a single long inline sentence that ran fund names into
     * each other on longer names / more series - the reported "text at
     * the top... running into each other" issue. Shows both the
     * normalized (base=100) value and the point-to-point CAGR from the
     * base date to here.
     */
    private fun buildLegendText(date: String, values: List<OverlayScrubValue>): CharSequence {
        val builder = SpannableStringBuilder()
        val dateLine = formatDisplayDate(date) + "\n"
        builder.append(dateLine, StyleSpan(Typeface.BOLD), 0)
        values.forEach { sv ->
            val valueText = if (sv.normalizedValue == null) {
                "no data yet"
            } else {
                val base = String.format(Locale.getDefault(), "%.1f", sv.normalizedValue)
                val cagrText = sv.cagrPercent?.let { String.format(Locale.getDefault(), " (%+.2f%% CAGR)", it) }.orEmpty()
                "$base$cagrText"
            }
            val line = "●  ${FundNameFormatter.shorten(sv.series.name).ifBlank { sv.series.name }}: $valueText\n"
            val start = builder.length
            builder.append(line)
            builder.setSpan(ForegroundColorSpan(sv.series.color), start, start + 1, 0) // colors just the "●"
        }
        return builder
    }

    private fun switchTab(tab: Tab) {
        activeTab = tab
        updateTabStyling()
        graphContainer.visibility = if (tab == Tab.GRAPH) View.VISIBLE else View.GONE
        tableContainer.visibility = if (tab == Tab.TABLE) View.VISIBLE else View.GONE
        if (tab == Tab.TABLE) buildTable()
    }

    // Simple alpha-based active/inactive styling rather than a new
    // style resource - ActionChip has no built-in "selected" variant,
    // and a full TabLayout (like ProgressionActivity's own daily/
    // weekly toggle) would need restructuring this screen's layout for
    // a cosmetic difference only.
    private fun updateTabStyling() {
        tabGraph.alpha = if (activeTab == Tab.GRAPH) 1.0f else 0.5f
        tabTable.alpha = if (activeTab == Tab.TABLE) 1.0f else 0.5f
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun formatDisplayDate(stored: String): String = try {
        val storedFormat = java.text.SimpleDateFormat("yyyy-MM-dd", Locale.US)
        val displayFormat = java.text.SimpleDateFormat("d MMM yyyy", Locale.getDefault())
        displayFormat.format(storedFormat.parse(stored) ?: stored)
    } catch (e: Exception) {
        stored
    }

    private fun loadRows() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val resultJson = Bridge.computeReturnsTable(portfolioJson)
        if (isBridgeError(resultJson)) {
            showEmpty("Could not load funds/indices: $resultJson")
            return
        }
        val rowType = object : TypeToken<List<ReturnsTableRow>>() {}.type
        rows = try {
            gson.fromJson(resultJson, rowType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }
        if (rows.isEmpty()) {
            showEmpty(
                "No historical data yet. Mutual funds need their NAV history fetched " +
                    "(Settings → a fund's Update History), and benchmarks need adding (Returns → gear icon)."
            )
            return
        }
        emptyState.visibility = View.GONE
        // Default to the first two funds (or fund + first benchmark) if
        // nothing's been picked yet, so the screen isn't blank on first
        // visit - the person can immediately change the selection.
        if (selectedSeriesIds.isEmpty()) {
            rows.take(2).forEach { selectedSeriesIds.add(it.seriesId) }
        }
        if (activeTab == Tab.TABLE) buildTable() else loadChart()
    }

    private fun showEmpty(message: String) {
        emptyState.visibility = View.VISIBLE
        emptyState.text = message
    }

    private fun showPicker() {
        val dialogPadding = (20 * resources.displayMetrics.density).toInt()
        val container = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dialogPadding, dialogPadding / 2, dialogPadding, 0)
        }
        val searchBox = EditText(this).apply {
            hint = "Search funds/indices"
        }
        val listView = ListView(this).apply {
            layoutParams = LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT,
                (360 * resources.displayMetrics.density).toInt()
            )
        }
        container.addView(searchBox)
        container.addView(listView)

        var visibleRows = rows
        val adapter = object : ArrayAdapter<ReturnsTableRow>(this, android.R.layout.simple_list_item_multiple_choice, visibleRows) {
            override fun getView(position: Int, convertView: View?, parent: android.view.ViewGroup): View {
                val view = super.getView(position, convertView, parent) as android.widget.CheckedTextView
                val row = getItem(position) ?: return view
                val prefix = if (row.isBenchmark) "[Index] " else "[Fund] "
                view.text = prefix + FundNameFormatter.shorten(row.name).ifBlank { row.name }
                view.isChecked = selectedSeriesIds.contains(row.seriesId)
                return view
            }
        }
        listView.adapter = adapter
        // Checked state lives in selectedSeriesIds (keyed by seriesId),
        // NOT in ListView's own position-based checked-state tracking -
        // positions shift as the search box filters the list, so
        // anything keyed by position would silently lose or misapply
        // selections across a filter change.
        listView.setOnItemClickListener { _, itemView, position, _ ->
            val row = adapter.getItem(position) ?: return@setOnItemClickListener
            if (selectedSeriesIds.contains(row.seriesId)) selectedSeriesIds.remove(row.seriesId) else selectedSeriesIds.add(row.seriesId)
            (itemView as? android.widget.CheckedTextView)?.isChecked = selectedSeriesIds.contains(row.seriesId)
        }
        searchBox.addTextChangedListener(object : TextWatcher {
            override fun afterTextChanged(s: Editable?) {
                val query = s?.toString()?.trim()?.lowercase(Locale.getDefault()).orEmpty()
                visibleRows = if (query.isEmpty()) rows else rows.filter { it.name.lowercase(Locale.getDefault()).contains(query) }
                adapter.clear()
                adapter.addAll(visibleRows)
            }
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
        })

        AlertDialog.Builder(this)
            .setTitle("Pick funds/indices to compare")
            .setView(container)
            .setPositiveButton("Done") { _, _ -> if (activeTab == Tab.TABLE) buildTable() else loadChart() }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun loadChart() {
        if (selectedSeriesIds.size < 2) {
            showEmpty("Pick at least 2 funds/indices to compare.")
            return
        }
        emptyState.visibility = View.GONE

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val idsJson = gson.toJson(selectedSeriesIds.toList())
        val resultJson = Bridge.computeMultiSeriesHistory(portfolioJson, idsJson)
        if (isBridgeError(resultJson)) {
            showEmpty("Could not load history: $resultJson")
            return
        }
        val itemType = object : TypeToken<List<MultiSeriesHistoryItem>>() {}.type
        val items: List<MultiSeriesHistoryItem> = try {
            gson.fromJson(resultJson, itemType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        val overlaySeries = items.mapIndexedNotNull { index, item ->
            val points = item.points ?: emptyList()
            if (points.size < 2) return@mapIndexedNotNull null
            OverlaySeries(
                seriesId = item.seriesId,
                name = item.name,
                isBenchmark = item.isBenchmark,
                points = points,
                color = ContextCompat.getColor(this, paletteColorIds[index % paletteColorIds.size])
            )
        }

        if (overlaySeries.size < 2) {
            showEmpty("Not enough overlapping history yet for at least 2 of the picked items. Use Refresh to fetch more.")
            return
        }

        currentUnionDateBounds = overlaySeries.flatMap { it.points }.let { pts ->
            if (pts.isEmpty()) null else pts.minOf { it.date } to pts.maxOf { it.date }
        }
        pickButton.text = "${overlaySeries.size} picked"
        chart.setSeries(overlaySeries)
        chart.setLockBaseDate(lockSwitch.isChecked)
        setUpRangePresets()
    }

    // 3M/6M/1Y/2Y/3Y/Max quick-jump shortcuts, same as
    // ReturnsDetailActivity's own chart - see that class's own doc
    // comment for the reasoning (identical here: computes a start date
    // some fixed offset before the union's LAST date and sets the
    // chart's window to [that date, last date]). Rebuilt on every
    // loadChart() call since the union date bounds can change when the
    // selected series change.
    private fun setUpRangePresets() {
        val bounds = currentUnionDateBounds ?: return
        val (firstDate, lastDate) = bounds
        val container = findViewById<LinearLayout>(R.id.comparisonRangePresets)
        container.removeAllViews()
        val presets = listOf("3M" to 3, "6M" to 6, "1Y" to 12, "2Y" to 24, "3Y" to 36)
        presets.forEach { (label, monthsBack) ->
            container.addView(buildPresetChip(label) {
                chart.setWindowByDates(dateMonthsBefore(lastDate, monthsBack), lastDate)
            })
        }
        container.addView(buildPresetChip("Max") {
            chart.setWindowByDates(firstDate, lastDate)
        })

        // Free-typed custom window, in years (including fractional) -
        // same as ReturnsDetailActivity's own chart.
        val yearsInput = findViewById<android.widget.EditText>(R.id.comparisonChartWindowYears)
        findViewById<View>(R.id.comparisonChartWindowYearsShow).setOnClickListener {
            val years = yearsInput.text?.toString()?.trim()?.toDoubleOrNull()
            if (years == null || years <= 0) {
                yearsInput.error = "Enter a window in years, e.g. 4.5"
                return@setOnClickListener
            }
            yearsInput.error = null
            chart.setWindowByDates(dateYearsBefore(lastDate, years), lastDate)
        }
    }

    private fun buildPresetChip(label: String, onClick: () -> Unit): TextView {
        val chip = TextView(this, null, 0, R.style.ActionChip)
        chip.text = label
        chip.layoutParams = LinearLayout.LayoutParams(
            LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT
        ).apply { marginEnd = dpToPx(8) }
        chip.setOnClickListener { onClick() }
        return chip
    }

    private fun dpToPx(dp: Int): Int = (dp * resources.displayMetrics.density).toInt()

    // storedDate is "yyyy-MM-dd" - same stored-date convention used
    // throughout this app. Falls back to storedDate itself if parsing
    // ever fails, so a preset tap degrades to "show everything from the
    // start" rather than crashing.
    private fun dateMonthsBefore(storedDate: String, months: Int): String {
        val fmt = java.text.SimpleDateFormat("yyyy-MM-dd", Locale.US)
        return try {
            val cal = java.util.Calendar.getInstance()
            cal.time = fmt.parse(storedDate) ?: return storedDate
            cal.add(java.util.Calendar.MONTH, -months)
            fmt.format(cal.time)
        } catch (e: Exception) {
            storedDate
        }
    }

    // Same as ReturnsDetailActivity's own dateYearsBefore - see its doc
    // comment for why fractional-year arithmetic goes via milliseconds
    // rather than whole-month Calendar math.
    private fun dateYearsBefore(storedDate: String, years: Double): String {
        val fmt = java.text.SimpleDateFormat("yyyy-MM-dd", Locale.US)
        return try {
            val date = fmt.parse(storedDate) ?: return storedDate
            val millisPerYear = 365.25 * 24 * 60 * 60 * 1000
            val shifted = java.util.Date(date.time - (years * millisPerYear).toLong())
            fmt.format(shifted)
        } catch (e: Exception) {
            storedDate
        }
    }

    /**
     * Row labels for the quantitative table, in display order. Shared
     * between the fixed label column and every series column, so the
     * two stay in lockstep by construction rather than by convention -
     * a mismatch here would silently misalign every value in the
     * table.
     *
     * "(3-yr)" is appended to exactly the metrics that are actually
     * windowed to trailing 3 years - NOT Max Drawdown (deliberately
     * full-history, no confirmed 3-year convention exists for that
     * one - see finance.WindowToTrailingYears' own doc comment), same
     * labeling ReturnsDetailActivity's own metric cards use.
     */
    private val riskLabels = listOf(
        "Beta (3-yr)", "Alpha (3-yr)", "Information Ratio (3-yr)", "Std. Deviation (3-yr)",
        "Up Capture (3-yr)", "Down Capture (3-yr)", "Max Drawdown (full history)",
        "Sharpe Ratio (3-yr)", "Sortino Ratio (3-yr)"
    )
    private val returnLabels = listOf(
        "Day", "1 Month", "1Y Trailing", "1Y Rolling (median)", "1Y Rolling (min-max)",
        "3Y Trailing", "3Y Rolling (median)", "3Y Rolling (min-max)",
        "5Y Trailing", "5Y Rolling (median)", "5Y Rolling (min-max)",
        "10Y Trailing", "10Y Rolling (median)", "10Y Rolling (min-max)"
    )

    /**
     * Builds the whole quantitative comparison table fresh - called on
     * switching to the Table tab, after the fund picker, and after a
     * per-fund benchmark change (which needs the WHOLE table rebuilt
     * since Bridge.computeFundMetrics is per-series, called once per
     * column here).
     */
    private fun buildTable() {
        tableRoot.removeAllViews()
        val selected = rows.filter { selectedSeriesIds.contains(it.seriesId) }
        if (selected.size < 1) {
            showEmpty("Pick at least 1 fund/index to compare.")
            return
        }
        emptyState.visibility = View.GONE

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)

        tableRoot.addView(buildLabelColumn())
        selected.forEach { row ->
            val metrics: FundMetricsResult? = if (row.isBenchmark) {
                null // risk parameters compare a FUND against a benchmark - meaningless for a benchmark comparing against itself
            } else {
                val resultJson = Bridge.computeFundMetrics(portfolioJson, row.seriesId, "")
                if (isBridgeError(resultJson)) null else try {
                    gson.fromJson(resultJson, FundMetricsResult::class.java)
                } catch (e: Exception) {
                    null
                }
            }
            tableRoot.addView(buildSeriesColumn(row, metrics, portfolioJson))
        }
    }

    private fun buildLabelColumn(): LinearLayout {
        val column = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(dpToPx(150), LinearLayout.LayoutParams.WRAP_CONTENT)
        }
        column.addView(tableCell("", isHeader = true, bold = true)) // aligns with each column's name header
        column.addView(tableCell("", isHeader = true)) // aligns with each column's "Benchmark: X" sub-header
        column.addView(tableSectionLabel("Trailing & Rolling Returns"))
        returnLabels.forEach { column.addView(tableCell(it)) }
        column.addView(tableSectionLabel("Risk Parameters"))
        riskLabels.forEach { column.addView(tableCell(it)) }
        return column
    }

    private fun buildSeriesColumn(row: ReturnsTableRow, metrics: FundMetricsResult?, portfolioJson: String): LinearLayout {
        val column = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(dpToPx(110), LinearLayout.LayoutParams.WRAP_CONTENT).apply {
                marginStart = dpToPx(4)
            }
        }
        column.addView(tableCell(FundNameFormatter.shorten(row.name).ifBlank { row.name }, isHeader = true, bold = true))

        val benchmarkLine = tableCell(
            if (row.isBenchmark) "" else (metrics?.benchmarkName?.takeIf { it.isNotBlank() }?.let { "vs $it" } ?: "vs (none)"),
            isHeader = true
        )
        if (!row.isBenchmark) {
            benchmarkLine.setTextColor(ContextCompat.getColor(this, R.color.colorPrimary))
            benchmarkLine.setOnClickListener {
                BenchmarkPicker.show(this@ComparisonActivity, row.seriesId, portfolioJson, metrics?.benchmarkId.orEmpty()) {
                    buildTable() // rebuild the whole table, not just this column - simplest way to reflect the change and pick up the new benchmark's own name
                }
            }
        }
        column.addView(benchmarkLine)

        column.addView(tableSectionLabel("")) // aligns with the label column's "Trailing & Rolling Returns" section header
        column.addView(tableCell(fmtPercent(row.day.hasData, row.day.percent)))
        column.addView(tableCell(fmtPercent(row.month.hasData, row.month.percent)))
        column.addView(tableCell(fmtPercent(row.oneYearTrailing.hasData, row.oneYearTrailing.percent)))
        column.addView(tableCell(fmtPercent(row.oneYearRolling.hasData, row.oneYearRolling.median)))
        column.addView(tableCell(fmtRange(row.oneYearRolling.hasData, row.oneYearRolling.min, row.oneYearRolling.max)))
        column.addView(tableCell(fmtPercent(row.threeYearTrailing.hasData, row.threeYearTrailing.percent)))
        column.addView(tableCell(fmtPercent(row.threeYearRolling.hasData, row.threeYearRolling.median)))
        column.addView(tableCell(fmtRange(row.threeYearRolling.hasData, row.threeYearRolling.min, row.threeYearRolling.max)))
        column.addView(tableCell(fmtPercent(row.fiveYearTrailing.hasData, row.fiveYearTrailing.percent)))
        column.addView(tableCell(fmtPercent(row.fiveYearRolling.hasData, row.fiveYearRolling.median)))
        column.addView(tableCell(fmtRange(row.fiveYearRolling.hasData, row.fiveYearRolling.min, row.fiveYearRolling.max)))
        column.addView(tableCell(fmtPercent(row.tenYearTrailing.hasData, row.tenYearTrailing.percent)))
        column.addView(tableCell(fmtPercent(row.tenYearRolling.hasData, row.tenYearRolling.median)))
        column.addView(tableCell(fmtRange(row.tenYearRolling.hasData, row.tenYearRolling.min, row.tenYearRolling.max)))

        column.addView(tableSectionLabel("")) // aligns with the label column's "Risk Parameters" section header
        if (metrics == null) {
            // A benchmark column, or a fund whose risk metrics failed to
            // load - fill every risk row with "-" so the column still
            // has exactly as many rows as every other column (buildTable's
            // own doc comment: alignment depends on this).
            repeat(riskLabels.size) { column.addView(tableCell("—")) }
        } else {
            column.addView(tableCell(fmtNumber(metrics.betaHasData, metrics.beta, 2)))
            column.addView(tableCell(fmtPercent(metrics.alphaHasData, metrics.alpha)))
            column.addView(tableCell(fmtNumber(metrics.infoRatioHasData, metrics.informationRatio, 2)))
            column.addView(tableCell(fmtPercent(metrics.stdDevHasData, metrics.standardDeviation)))
            column.addView(tableCell(fmtPercent(metrics.upCaptureHasData, metrics.upCapture)))
            column.addView(tableCell(fmtPercent(metrics.downCaptureHasData, metrics.downCapture)))
            column.addView(tableCell(fmtPercent(metrics.maxDrawdownHasData, metrics.maxDrawdown)))
            column.addView(tableCell(fmtNumber(metrics.sharpeHasData, metrics.sharpeRatio, 2)))
            column.addView(tableCell(fmtNumber(metrics.sortinoHasData, metrics.sortinoRatio, 2)))
        }
        return column
    }

    private fun tableCell(text: String, isHeader: Boolean = false, bold: Boolean = false): TextView {
        return TextView(this).apply {
            this.text = text
            textSize = if (isHeader) 12f else 12f
            setTextColor(ContextCompat.getColor(this@ComparisonActivity, R.color.colorOnSurface))
            if (bold) setTypeface(typeface, Typeface.BOLD)
            maxLines = 2
            val vPad = dpToPx(6)
            setPadding(dpToPx(4), vPad, dpToPx(4), vPad)
        }
    }

    private fun tableSectionLabel(text: String): TextView {
        return TextView(this).apply {
            this.text = text
            textSize = 11f
            setTypeface(typeface, Typeface.BOLD)
            setTextColor(ContextCompat.getColor(this@ComparisonActivity, R.color.colorNeutral))
            setPadding(dpToPx(4), dpToPx(10), dpToPx(4), dpToPx(2))
        }
    }

    private fun fmtPercent(hasData: Boolean, value: Double): String =
        if (hasData) String.format(Locale.getDefault(), "%+.2f%%", value) else "—"

    private fun fmtNumber(hasData: Boolean, value: Double, decimals: Int): String =
        if (hasData) String.format(Locale.getDefault(), "%.${decimals}f", value) else "—"

    private fun fmtRange(hasData: Boolean, min: Double, max: Double): String =
        if (hasData) String.format(Locale.getDefault(), "%+.1f to %+.1f%%", min, max) else "—"
}
