package com.saby.personalportfolio

import android.os.Bundle
import android.view.View
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.widget.PopupMenu
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
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
        chart.onPointScrubbed = { point ->
            val displayDate = try {
                displayFormat.format(storedFormat.parse(point.date) ?: point.date)
            } catch (e: Exception) {
                point.date
            }
            scrubbedView.text = "$displayDate: ${IndianCurrencyFormatter.format(point.price, decimals = 2)}"
        }
        chart.setPoints(points)

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

        bindMetricRow(R.id.returnsDetailMetricBeta, "Beta", m.beta, m.betaHasData, decimals = 2, suffix = "")
        bindMetricRow(R.id.returnsDetailMetricInfoRatio, "Information Ratio", m.informationRatio, m.infoRatioHasData, decimals = 2, suffix = "")
        bindMetricRow(R.id.returnsDetailMetricUpCapture, "Up Capture", m.upCapture, m.upCaptureHasData, decimals = 0, suffix = "%")
        bindMetricRow(R.id.returnsDetailMetricDownCapture, "Down Capture", m.downCapture, m.downCaptureHasData, decimals = 0, suffix = "%")
        bindMetricRow(R.id.returnsDetailMetricMaxDrawdown, "Max Drawdown", m.maxDrawdown, m.maxDrawdownHasData, decimals = 1, suffix = "%")
    }

    private fun bindMetricRow(viewId: Int, label: String, value: Double, hasData: Boolean, decimals: Int, suffix: String) {
        val view = findViewById<TextView>(viewId)
        view.text = if (hasData) {
            "$label: " + String.format(Locale.getDefault(), "%.${decimals}f$suffix", value)
        } else {
            "$label: — (not enough overlapping history)"
        }
    }

    /**
     * Lets the person override the auto-picked benchmark - reads the
     * same PortfolioBenchmarksSnapshot slice BenchmarksActivity uses, so
     * this only ever offers benchmarks the person has actually added
     * (see Settings → Manage Benchmarks), never a hypothetical index
     * they haven't tracked yet.
     */
    private fun showBenchmarkPicker() {
        val snapshot: PortfolioBenchmarksSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioBenchmarksSnapshot::class.java)
        } catch (e: Exception) {
            null
        } ?: PortfolioBenchmarksSnapshot(emptyList(), emptyList())
        val benchmarks = snapshot.benchmarks ?: emptyList()
        if (benchmarks.isEmpty()) return

        val anchor = findViewById<View>(R.id.returnsDetailChangeBenchmark)
        val popup = PopupMenu(this, anchor)
        popup.menu.add(0, -1, -1, "Auto-pick (recommended)")
        benchmarks.forEachIndexed { index, b ->
            popup.menu.add(0, index, index, b.name)
        }
        popup.setOnMenuItemClickListener { item ->
            selectedBenchmarkId = if (item.itemId == -1) "" else benchmarks[item.itemId].id
            loadMetrics()
            true
        }
        popup.show()
    }
}
