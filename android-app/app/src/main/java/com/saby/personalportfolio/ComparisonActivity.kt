package com.saby.personalportfolio

import android.app.AlertDialog
import android.os.Bundle
import android.view.View
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

        chart.onScrubbed = { date, values ->
            val displayDate = formatDisplayDate(date)
            val parts = values.mapNotNull { (series, v) ->
                if (v == null) null else "${FundNameFormatter.shorten(series.name).ifBlank { series.name }}: " +
                    String.format(Locale.getDefault(), "%.1f", v)
            }
            scrubbedView.text = if (parts.isEmpty()) displayDate else "$displayDate — ${parts.joinToString("   ")}"
        }

        loadRows()
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
        val labels = rows.map {
            val prefix = if (it.isBenchmark) "[Index] " else "[Fund] "
            prefix + FundNameFormatter.shorten(it.name).ifBlank { it.name }
        }.toTypedArray()
        val checked = rows.map { selectedSeriesIds.contains(it.seriesId) }.toBooleanArray()

        AlertDialog.Builder(this)
            .setTitle("Pick funds/indices to compare")
            .setMultiChoiceItems(labels, checked) { _, which, isChecked ->
                if (isChecked) selectedSeriesIds.add(rows[which].seriesId) else selectedSeriesIds.remove(rows[which].seriesId)
            }
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

        pickButton.text = "${overlaySeries.size} picked"
        chart.setSeries(overlaySeries)
        chart.setLockBaseDate(lockSwitch.isChecked)
    }
}
