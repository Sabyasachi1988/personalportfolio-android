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
import android.widget.HorizontalScrollView
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
    private lateinit var tableSectionsRoot: LinearLayout
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
        tableSectionsRoot = findViewById(R.id.comparisonTableSectionsRoot)

        tabGraph.setOnClickListener { switchTab(Tab.GRAPH) }
        tabTable.setOnClickListener { switchTab(Tab.TABLE) }
        findViewById<View>(R.id.comparisonCustomizeButton).setOnClickListener { showMetricPicker() }
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
        if (tab == Tab.TABLE) buildTable() else loadChart()
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

    /**
     * Called whenever the picked set of funds/indices changes (only
     * from the picker's "Done" button today). Confirmed real bug this
     * fixes: previously, Done only refreshed whichever ONE tab
     * (Graph/Table) happened to be active at that moment - the OTHER
     * tab's OverlayChartView/table kept showing the OLD selection
     * until the picker was reopened while THAT tab was active. The
     * picker button's own "N picked" label had the same problem, since
     * it was set inside loadChart() from overlaySeries.size rather
     * than from the actual selection.
     *
     * pickButton.text is set from selectedSeriesIds.size directly -
     * the actual picked count, not derived from whichever tab
     * successfully rendered (which can legitimately be fewer, e.g. a
     * picked fund with no overlapping history yet - that's a separate
     * signal the empty-state message already covers, not something
     * the picker label should silently reflect instead of the real
     * selection).
     */
    private fun onSelectionChanged() {
        pickButton.text = "${selectedSeriesIds.size} picked"
        loadChart()
        buildTable()
        // Only the active tab's container is actually visible - see
        // switchTab - so re-apply that visibility now, since both
        // loadChart()/buildTable() above may have touched
        // emptyState.visibility as a side effect of either one
        // failing independently of the other.
        graphContainer.visibility = if (activeTab == Tab.GRAPH) View.VISIBLE else View.GONE
        tableContainer.visibility = if (activeTab == Tab.TABLE) View.VISIBLE else View.GONE
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
        pickButton.text = "${selectedSeriesIds.size} picked"
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
            .setPositiveButton("Done") { _, _ -> onSelectionChanged() }
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

    // Fixed row heights, NOT wrap_content - this is the actual fix for
    // a confirmed real bug: with every column an independent
    // wrap_content LinearLayout, a fund name or benchmark name that
    // happened to wrap onto a different number of lines than its
    // neighbor threw off every row below it for that column only,
    // producing exactly the "offset, crowded" table seen in a
    // screenshot. Every cell in a given logical row now has the SAME
    // fixed height everywhere it appears - the label column and every
    // fund column alike - so alignment is guaranteed by construction,
    // not by hoping text lengths happen to match.
    private val headerRowHeightDp = 52
    private val dataRowHeightDp = 40

    private fun buildTable() {
        tableSectionsRoot.removeAllViews()
        val selected = rows.filter { selectedSeriesIds.contains(it.seriesId) }
        if (selected.isEmpty()) {
            showEmpty("Pick at least 1 fund/index to compare.")
            return
        }
        emptyState.visibility = View.GONE

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val metricsBySeriesId: Map<String, FundMetricsResult?> = selected.associate { row ->
            row.seriesId to if (row.isBenchmark) {
                null // risk parameters compare a FUND against a benchmark - meaningless for a benchmark comparing against itself
            } else {
                val resultJson = Bridge.computeFundMetrics(portfolioJson, row.seriesId, "")
                if (isBridgeError(resultJson)) null else try {
                    gson.fromJson(resultJson, FundMetricsResult::class.java)
                } catch (e: Exception) {
                    null
                }
            }
        }

        // Which rows to show - the person's own persisted pick (see
        // CompareMetricPreference's own doc comment for why this
        // exists: a confirmed real ask to choose from the FULL
        // available set, including the rolling range rows not shown by
        // default, and have that choice stick across app restarts
        // rather than resetting every time). Order stays canonical
        // (CompareMetricCatalog.ALL's own order) regardless of the
        // order the person happened to check things in.
        val selectedMetricIds = CompareMetricPreference.getSelectedIds(this)
        val returnMetrics = CompareMetricCatalog.ALL.filter { it.section == CompareMetricCatalog.Section.RETURNS && selectedMetricIds.contains(it.id) }
        val riskMetrics = CompareMetricCatalog.ALL.filter { it.section == CompareMetricCatalog.Section.RISK && selectedMetricIds.contains(it.id) }

        if (returnMetrics.isNotEmpty()) {
            tableSectionsRoot.addView(
                buildTableSection(
                    title = "Trailing & Rolling Returns",
                    selected = selected,
                    showBenchmarkSubHeader = false,
                    rowLabels = returnMetrics.map { it.label },
                    cellFor = { row, i -> returnsCell(row, returnMetrics[i].id) }
                )
            )
        }
        if (riskMetrics.isNotEmpty()) {
            tableSectionsRoot.addView(
                buildTableSection(
                    title = "Risk Parameters (3-yr)",
                    selected = selected,
                    showBenchmarkSubHeader = true,
                    rowLabels = riskMetrics.map { it.label },
                    cellFor = { row, i -> riskCell(metricsBySeriesId[row.seriesId], riskMetrics[i].id) },
                    metricsBySeriesId = metricsBySeriesId,
                    portfolioJson = portfolioJson,
                    footnote = if (riskMetrics.any { it.id == "max_drawdown" }) "* Max Drawdown is full-history, not 3-yr" else null
                )
            )
        }
        if (returnMetrics.isEmpty() && riskMetrics.isEmpty()) {
            showEmpty("No metrics selected - tap Customize to pick some.")
        }
    }

    /** @return (display text, color resource id or null for the default neutral color) */
    private fun returnsCell(row: ReturnsTableRow, metricId: String): Pair<String, Int?> {
        val trailing = when (metricId) {
            "day" -> row.day
            "month" -> row.month
            "y1_trailing" -> row.oneYearTrailing
            "y3_trailing" -> row.threeYearTrailing
            "y5_trailing" -> row.fiveYearTrailing
            "y10_trailing" -> row.tenYearTrailing
            else -> null
        }
        if (trailing != null) return fmtPercent(trailing.hasData, trailing.percent) to gainLossColor(trailing.hasData, trailing.percent)
        val rolling = when (metricId) {
            "y1_rolling", "y1_range" -> row.oneYearRolling
            "y3_rolling", "y3_range" -> row.threeYearRolling
            "y5_rolling", "y5_range" -> row.fiveYearRolling
            "y10_rolling", "y10_range" -> row.tenYearRolling
            else -> null
        } ?: return "—" to null
        return if (metricId.endsWith("_range")) {
            fmtRange(rolling.hasData, rolling.min, rolling.max) to null // a range has no single "good/bad" color
        } else {
            fmtPercent(rolling.hasData, rolling.median) to gainLossColor(rolling.hasData, rolling.median)
        }
    }

    /** @return (display text, color resource id or null for the default neutral color) */
    private fun riskCell(metrics: FundMetricsResult?, metricId: String): Pair<String, Int?> {
        if (metrics == null) return "—" to null
        return when (metricId) {
            "beta" -> fmtNumber(metrics.betaHasData, metrics.beta, 2) to null // Beta has no "good/bad" direction
            "alpha" -> fmtPercent(metrics.alphaHasData, metrics.alpha) to gainLossColor(metrics.alphaHasData, metrics.alpha)
            "info_ratio" -> fmtNumber(metrics.infoRatioHasData, metrics.informationRatio, 2) to gainLossColor(metrics.infoRatioHasData, metrics.informationRatio)
            "std_dev" -> fmtPercent(metrics.stdDevHasData, metrics.standardDeviation) to null // Std Dev has no "good/bad" direction
            "up_capture" -> fmtPercent(metrics.upCaptureHasData, metrics.upCapture) to gainLossColor(metrics.upCaptureHasData, metrics.upCapture - 100)
            "down_capture" -> fmtPercent(metrics.downCaptureHasData, metrics.downCapture) to gainLossColor(metrics.downCaptureHasData, 100 - metrics.downCapture) // LOWER is better here, so the sign is inverted
            "max_drawdown" -> fmtPercent(metrics.maxDrawdownHasData, metrics.maxDrawdown) to (if (metrics.maxDrawdownHasData) R.color.colorLoss else null) // always loss-tinted, same convention as ReturnsDetailActivity's own card
            "sharpe" -> fmtNumber(metrics.sharpeHasData, metrics.sharpeRatio, 2) to gainLossColor(metrics.sharpeHasData, metrics.sharpeRatio)
            "sortino" -> fmtNumber(metrics.sortinoHasData, metrics.sortinoRatio, 2) to gainLossColor(metrics.sortinoHasData, metrics.sortinoRatio)
            else -> "—" to null
        }
    }

    private fun gainLossColor(hasData: Boolean, value: Double): Int? =
        if (!hasData) null else if (value >= 0) R.color.colorGain else R.color.colorLoss

    /**
     * Multi-select picker over the full CompareMetricCatalog - a flat
     * checklist in catalog order (Returns metrics first, then Risk),
     * same pattern as the fund/index picker above. Persists
     * immediately on Done and rebuilds the table - no separate
     * "Apply" step.
     */
    private fun showMetricPicker() {
        val currentIds = CompareMetricPreference.getSelectedIds(this).toMutableSet()
        val items = CompareMetricCatalog.ALL
        val labels = items.map { it.label }.toTypedArray()
        val checked = items.map { currentIds.contains(it.id) }.toBooleanArray()
        AlertDialog.Builder(this)
            .setTitle("Choose what to compare")
            .setMultiChoiceItems(labels, checked) { _, which, isChecked ->
                val id = items[which].id
                if (isChecked) currentIds.add(id) else currentIds.remove(id)
            }
            .setPositiveButton("Done") { _, _ ->
                CompareMetricPreference.setSelectedIds(this, currentIds)
                if (activeTab == Tab.TABLE) buildTable()
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    /**
     * One section = one elevated MaterialCardView (matching the
     * dashboard's own donut-card style - cardCornerRadius=16dp,
     * cardElevation=3dp - rather than a flat solid-background
     * LinearLayout, which read as dull against the rest of the app):
     * an colorAmber-accented title, then a frozen label column
     * (outside any scroll view) alongside a HorizontalScrollView
     * holding one column per selected fund/index - the "professional
     * table" pattern (frozen row headers, only the data scrolls).
     * Each fund column's header is colored with the SAME palette
     * color the Graph tab's line for that fund uses, and every
     * value cell is colored green/red by what a favorable reading
     * actually means for THAT metric (see riskCell's own per-row
     * logic) - a flat, uncolored table of numbers was the main
     * source of the "dull" impression; a financial table reads far
     * more alive once gains/losses actually look like gains/losses.
     */
    private fun buildTableSection(
        title: String,
        selected: List<ReturnsTableRow>,
        showBenchmarkSubHeader: Boolean,
        rowLabels: List<String>,
        cellFor: (ReturnsTableRow, Int) -> Pair<String, Int?>,
        metricsBySeriesId: Map<String, FundMetricsResult?> = emptyMap(),
        portfolioJson: String = "",
        footnote: String? = null
    ): View {
        val card = com.google.android.material.card.MaterialCardView(this).apply {
            layoutParams = LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT).apply {
                topMargin = dpToPx(16)
            }
            radius = dpToPx(16).toFloat()
            cardElevation = dpToPx(3).toFloat()
            setCardBackgroundColor(ContextCompat.getColor(this@ComparisonActivity, R.color.colorSurface))
        }
        val cardContent = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            val pad = dpToPx(10)
            setPadding(pad, pad, pad, pad)
        }
        cardContent.addView(TextView(this).apply {
            text = title
            textSize = 15f
            setTypeface(typeface, Typeface.BOLD)
            setTextColor(ContextCompat.getColor(this@ComparisonActivity, R.color.colorAmber))
            setPadding(dpToPx(4), 0, dpToPx(4), if (footnote == null) dpToPx(10) else 0)
        })
        if (footnote != null) {
            cardContent.addView(TextView(this).apply {
                text = footnote
                textSize = 10f
                setTextColor(ContextCompat.getColor(this@ComparisonActivity, R.color.colorNeutral))
                setPadding(dpToPx(4), 0, dpToPx(4), dpToPx(10))
            })
        }

        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            layoutParams = LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT)
        }

        // Frozen label column - fixed width, NOT inside the
        // HorizontalScrollView below, so it stays put while only the
        // fund columns scroll. Given its own solid colorSurfaceVariant
        // background (not per-row zebra like the value columns) so the
        // WHOLE column reads as one distinct panel - color contrast
        // against the value area, rather than the earlier explicit
        // divider line, which read as an abrupt/broken edge rather
        // than an intentional design choice.
        val labelColumnWidthPx = dpToPx(96)
        val labelColumn = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            layoutParams = LinearLayout.LayoutParams(labelColumnWidthPx, LinearLayout.LayoutParams.WRAP_CONTENT)
            setBackgroundColor(ContextCompat.getColor(this@ComparisonActivity, R.color.colorSurfaceVariant))
        }
        labelColumn.addView(fixedHeightCell("", headerRowHeightDp, isHeader = true, zebra = false))
        if (showBenchmarkSubHeader) {
            labelColumn.addView(fixedHeightCell("", dataRowHeightDp, isHeader = true, zebra = false))
        }
        rowLabels.forEach { label ->
            labelColumn.addView(fixedHeightCell(label, dataRowHeightDp, zebra = false))
        }
        row.addView(labelColumn)

        // Adaptive fund-column width: fill the available width evenly
        // across however many funds are picked, UP UNTIL that would
        // squeeze columns below the comfortable minimum - confirmed
        // real complaint: with only 2 funds picked, the fixed-width
        // columns left a visible dead strip of empty space on the
        // right, looking "left-tilted" and unbalanced, while more than
        // 2 funds looked fine simply because they happened to fill the
        // width by coincidence. Below the minimum-width threshold, the
        // usual fixed-width + HorizontalScrollView behavior takes over
        // instead (compressed, scrollable), same as before.
        val minFundColumnWidthPx = dpToPx(92)
        val screenWidthPx = resources.displayMetrics.widthPixels
        val chromeWidthPx = dpToPx(20 + 20 + 10 + 10) + labelColumnWidthPx // root padding + card padding
        val availableForColumnsPx = screenWidthPx - chromeWidthPx
        val fundColumnWidthPx = if (selected.isNotEmpty() && selected.size * minFundColumnWidthPx < availableForColumnsPx) {
            availableForColumnsPx / selected.size
        } else {
            minFundColumnWidthPx
        }

        val scrollValueColumns = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
        }
        selected.forEachIndexed { columnIndex, seriesRow ->
            val fundColor = ContextCompat.getColor(this, paletteColorIds[columnIndex % paletteColorIds.size])
            val column = LinearLayout(this).apply {
                orientation = LinearLayout.VERTICAL
                layoutParams = LinearLayout.LayoutParams(fundColumnWidthPx, LinearLayout.LayoutParams.WRAP_CONTENT).apply {
                    marginStart = dpToPx(2)
                }
            }
            column.addView(
                fixedHeightCell(
                    FundNameFormatter.shorten(seriesRow.name).ifBlank { seriesRow.name },
                    headerRowHeightDp, isHeader = true, bold = true, zebra = false, explicitColor = fundColor,
                    gravity = android.view.Gravity.END or android.view.Gravity.CENTER_VERTICAL
                )
            )
            if (showBenchmarkSubHeader) {
                val metrics = metricsBySeriesId[seriesRow.seriesId]
                val benchmarkCell = fixedHeightCell(
                    if (seriesRow.isBenchmark) "" else (metrics?.benchmarkName?.takeIf { it.isNotBlank() }?.let { "vs $it" } ?: "vs (none)"),
                    dataRowHeightDp, isHeader = true, zebra = false,
                    explicitColor = if (seriesRow.isBenchmark) null else ContextCompat.getColor(this, R.color.colorPrimary),
                    gravity = android.view.Gravity.END or android.view.Gravity.CENTER_VERTICAL
                )
                if (!seriesRow.isBenchmark) {
                    benchmarkCell.setOnClickListener {
                        BenchmarkPicker.show(this@ComparisonActivity, seriesRow.seriesId, portfolioJson, metrics?.benchmarkId.orEmpty()) {
                            buildTable() // rebuild everything - simplest way to reflect the change and pick up the new benchmark's own name
                        }
                    }
                }
                column.addView(benchmarkCell)
            }
            rowLabels.forEachIndexed { i, _ ->
                val (text, colorRes) = cellFor(seriesRow, i)
                column.addView(
                    fixedHeightCell(
                        text, dataRowHeightDp, zebra = i % 2 == 1,
                        gravity = android.view.Gravity.END or android.view.Gravity.CENTER_VERTICAL,
                        explicitColor = colorRes?.let { ContextCompat.getColor(this, it) }
                    )
                )
            }
            scrollValueColumns.addView(column)
        }

        val scrollView = HorizontalScrollView(this).apply {
            scrollBarStyle = View.SCROLLBARS_INSIDE_INSET
            addView(scrollValueColumns)
        }
        row.addView(scrollView)
        cardContent.addView(row)
        card.addView(cardContent)
        return card
    }

    /**
     * A single table cell with a FIXED height (see buildTable's own
     * doc comment on headerRowHeightDp/dataRowHeightDp for why this
     * matters) - ellipsized to 2 lines rather than wrapping
     * indefinitely, so long fund/benchmark names never blow out the
     * fixed height they're constrained to.
     *
     * @param explicitColor overrides the default colorOnSurface text
     *   color - used for gain/loss-tinted value cells and
     *   palette-colored fund-name headers. Null keeps the default.
     */
    private fun fixedHeightCell(
        text: String,
        heightDp: Int,
        isHeader: Boolean = false,
        bold: Boolean = false,
        zebra: Boolean = false,
        gravity: Int = android.view.Gravity.START or android.view.Gravity.CENTER_VERTICAL,
        explicitColor: Int? = null
    ): TextView {
        return TextView(this).apply {
            this.text = text
            textSize = 12f
            setTextColor(explicitColor ?: ContextCompat.getColor(this@ComparisonActivity, R.color.colorOnSurface))
            if (bold) setTypeface(typeface, Typeface.BOLD)
            maxLines = 2
            ellipsize = android.text.TextUtils.TruncateAt.END
            this.gravity = gravity
            setPadding(dpToPx(6), dpToPx(2), dpToPx(6), dpToPx(2))
            layoutParams = LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, dpToPx(heightDp))
            if (zebra) setBackgroundColor(ContextCompat.getColor(this@ComparisonActivity, R.color.colorSurfaceVariant))
        }
    }

    private fun fmtPercent(hasData: Boolean, value: Double): String =
        if (hasData) String.format(Locale.getDefault(), "%+.2f%%", value) else "—"

    private fun fmtNumber(hasData: Boolean, value: Double, decimals: Int): String =
        if (hasData) String.format(Locale.getDefault(), "%.${decimals}f", value) else "—"

    private fun fmtRange(hasData: Boolean, min: Double, max: Double): String =
        if (hasData) String.format(Locale.getDefault(), "%+.1f/%+.1f%%", min, max) else "—"
}
