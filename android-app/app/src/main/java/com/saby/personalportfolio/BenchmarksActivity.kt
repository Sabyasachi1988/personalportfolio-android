package com.saby.personalportfolio

import android.os.Bundle
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.google.gson.Gson
import com.ledger.bridge.Bridge

class BenchmarksActivity : AppCompatActivity() {

    /**
     * Yahoo Finance tickers confirmed directly against live Yahoo
     * Finance quote pages (not guessed, not carried over from an
     * unrelated source like Wikipedia's "trading symbol" field, which
     * for Nifty Next 50 differs from what Yahoo's own page actually
     * uses) - safe to pre-fill as one-tap suggestions.
     */
    private val knownIndices = listOf(
        "Nifty 50" to "^NSEI",
        "Sensex" to "^BSESN",
        "Nifty Next 50" to "^NSMIDCP",
        "Nifty 500" to "^CRSLDX",
        "Nifty Midcap 150" to "NIFTYMIDCAP150.NS",
        "Nifty Smallcap 250" to "NIFTYSMLCAP250.NS"
    )

    private val gson = Gson()
    private lateinit var quickAddGroup: ChipGroup
    private lateinit var recyclerView: RecyclerView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_benchmarks)

        quickAddGroup = findViewById(R.id.benchmarksQuickAddGroup)
        recyclerView = findViewById(R.id.benchmarksRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

        findViewById<android.widget.Button>(R.id.benchmarksAddButton).setOnClickListener {
            val nameInput = findViewById<android.widget.EditText>(R.id.benchmarksNameInput)
            val tickerInput = findViewById<android.widget.EditText>(R.id.benchmarksTickerInput)
            val name = nameInput.text.toString().trim()
            val ticker = tickerInput.text.toString().trim()
            if (name.isEmpty() || ticker.isEmpty()) {
                Toast.makeText(this, "Enter both a name and a ticker", Toast.LENGTH_SHORT).show()
                return@setOnClickListener
            }
            addBenchmark(name, ticker)
            nameInput.text.clear()
            tickerInput.text.clear()
        }

        reload()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun reload() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val snapshot: PortfolioBenchmarksSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioBenchmarksSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioBenchmarksSnapshot(emptyList(), emptyList())
        }
        val benchmarks = snapshot.benchmarks.orEmpty()
        val pricedSeriesIds = snapshot.prices.orEmpty().map { it.seriesId }.toSet()

        bindQuickAddChips(benchmarks)

        recyclerView.adapter = BenchmarksAdapter(
            benchmarks = benchmarks,
            hasHistory = { id -> id in pricedSeriesIds },
            onRefresh = { benchmark, rowHolder -> refreshHistory(benchmark, rowHolder) },
            onDelete = { benchmark -> deleteBenchmark(benchmark) }
        )
    }

    private fun bindQuickAddChips(existing: List<Benchmark>) {
        quickAddGroup.removeAllViews()
        val existingTickers = existing.map { it.yahooTicker }.toSet()
        for ((name, ticker) in knownIndices) {
            if (ticker in existingTickers) continue // already tracked - nothing to suggest
            val chip = Chip(this)
            chip.text = name
            chip.isClickable = true
            chip.setOnClickListener { addBenchmark(name, ticker) }
            quickAddGroup.addView(chip)
        }
    }

    private fun addBenchmark(name: String, ticker: String) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterAdd = Bridge.addBenchmark(portfolioJson, name, ticker)
        if (isBridgeError(afterAdd)) {
            Toast.makeText(this, "Failed to add: $afterAdd", Toast.LENGTH_LONG).show()
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, afterAdd)
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        Toast.makeText(this, "Added $name - tap Refresh to fetch its history", Toast.LENGTH_SHORT).show()
        reload()
    }

    private fun deleteBenchmark(benchmark: Benchmark) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterRemove = Bridge.removeBenchmark(portfolioJson, benchmark.id)
        if (isBridgeError(afterRemove)) {
            Toast.makeText(this, "Failed to remove: $afterRemove", Toast.LENGTH_LONG).show()
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, afterRemove)
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        reload()
    }

    private fun refreshHistory(benchmark: Benchmark, rowHolder: BenchmarksAdapter.RowHolder) {
        rowHolder.refreshButton.isEnabled = false
        rowHolder.status.text = "Fetching…"
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        // "2000-01-01" - deliberately far enough back to capture full
        // available history for any of these indices (the oldest,
        // Sensex, dates to 1986, but Yahoo's own history for it doesn't
        // go back nearly that far in practice) - FetchYahooAdjClose
        // simply returns whatever Yahoo actually has starting from
        // this date, so asking for more than exists is harmless.
        val afterFetch = Bridge.updateBenchmarkHistory(portfolioJson, benchmark.id, "2000-01-01")
        rowHolder.refreshButton.isEnabled = true
        if (isBridgeError(afterFetch)) {
            Toast.makeText(this, "Failed to fetch history: $afterFetch", Toast.LENGTH_LONG).show()
            rowHolder.status.text = benchmark.yahooTicker
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, afterFetch)
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        Toast.makeText(this, "${benchmark.name} history updated", Toast.LENGTH_SHORT).show()
        reload()
    }
}
