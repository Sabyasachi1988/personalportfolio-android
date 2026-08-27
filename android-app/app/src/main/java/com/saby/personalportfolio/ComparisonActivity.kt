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
    private lateinit var scrubbedView: TextView
    private lateinit var emptyState: TextView

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
        scrubbedView = findViewById(R.id.comparisonScrubbed)
        emptyState = findViewById(R.id.comparisonEmptyState)

        pickButton.setOnClickListener { showPicker() }
        lockSwitch.setOnCheckedChangeListener { _, checked -> chart.setLockBaseDate(checked) }
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
                val cagrText = sv.cagrPercent?.let { String.format(Locale.getDefault(), " (%+.1f%% CAGR)", it) }.orEmpty()
                "$base$cagrText"
            }
            val line = "●  ${FundNameFormatter.shorten(sv.series.name).ifBlank { sv.series.name }}: $valueText\n"
            val start = builder.length
            builder.append(line)
            builder.setSpan(ForegroundColorSpan(sv.series.color), start, start + 1, 0) // colors just the "●"
        }
        return builder
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
        loadChart()
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
            .setPositiveButton("Done") { _, _ -> loadChart() }
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
    }
}
