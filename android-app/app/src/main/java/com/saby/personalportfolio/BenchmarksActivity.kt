package com.saby.personalportfolio

import android.content.ClipData
import android.content.ClipboardManager
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.Toast
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
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

    /**
     * TRI (Total Return, dividends reinvested) versions of the same
     * indices - the genuinely correct benchmark for a fund comparison,
     * since every real fund factsheet benchmarks against the TRI
     * variant, not the plain price index above. See
     * store.Benchmark.NiftyTRIIndexName's Go doc comment for the
     * source and its confirmed canonical spellings - these are NOT
     * Yahoo tickers, they're NSE Indices' own internal index names.
     */
    private val knownTRIIndices = listOf(
        "Nifty 50 TRI" to "NIFTY 50",
        "Nifty 500 TRI" to "NIFTY 500",
        "Nifty Midcap 150 TRI" to "NIFTY MIDCAP 150",
        "Nifty Smallcap 250 TRI" to "NIFTY SMALLCAP 250"
    )

    /**
     * (chip label, stored canonical name, recommended search prefill)
     * for the index-fund-NAV proxy path (Benchmark.ProxyFundISIN's Go
     * doc comment covers why this exists at all). Deliberately NOT
     * hardcoded ISINs - each is only a suggested SEARCH QUERY; the
     * person picks the actual match from live mfapi.in results via
     * showProxyFundPicker, same as any other Additional Fund add. A
     * wrong hardcoded ISIN would silently corrupt every risk stat
     * computed against it, which is worse than not having the
     * shortcut.
     *
     * The STORED name is deliberately the canonical NSE Indices
     * spelling ("NIFTY 500", not "Nifty 500 (index fund proxy)") for
     * the four Nifty-segment ones - finance.DefaultBenchmarkTRIName on
     * the Go side matches a fund's auto-selected benchmark against
     * exactly this string (see ComputeFundMetrics' auto-select in
     * bridge.go), so a proxy benchmark needs the same spelling a
     * TRI-scrape benchmark would have used to be found by that same
     * auto-select logic. The friendlier "(index fund proxy)" wording is
     * only the CHIP's own display text, plus Benchmark.DisplayName()
     * on the Go side already appends "(via <fund name>)" once a proxy
     * is actually added - so the row itself never shows the bare
     * all-caps canonical string.
     *
     * Picks reasoned from Cafemutual's recurring index-fund tracking-
     * error/tracking-difference rankings (checked live, not from
     * memory) - Aug 2026 editions:
     *  - Nifty 50: UTI Nifty 50 Index Fund - India's oldest (2000),
     *    largest AUM, consistently top-3 on BOTH tracking error and
     *    tracking difference across every edition checked, not just a
     *    single-month outlier.
     *  - Nifty 500: Motilal Oswal Nifty 500 Index Fund - the
     *    consistent #1 by tracking error AND tracking difference in
     *    every edition checked, also the longest-running Nifty 500
     *    fund (2009).
     *  - Nifty Midcap 150 / Smallcap 250: picked established,
     *    consistently-ranked, larger-AUM funds (Motilal Oswal, SBI)
     *    over month-to-month "lowest of all" picks like Navi, whose
     *    ultra-low tracking error comes with much smaller AUM and a
     *    shorter, less battle-tested history - the ranking leader here
     *    changes practically every edition, so consistency across
     *    editions weighed more than any single month's #1 spot.
     *  - Sensex: included since Sensex is an existing benchmark entry
     *    and BSE-tracking funds are shown to be fully competitive with
     *    Nifty 50 funds on tracking error (HDFC/UTI BSE Sensex funds
     *    tie for lowest in multiple editions) - directly answers the
     *    "funds that benchmark to a BSE index instead" case. No
     *    DefaultBenchmarkTRIName auto-select match exists for Sensex
     *    (it isn't one of the four fund-segment defaults), so its
     *    stored name is just "Sensex", matching the existing
     *    YahooTicker-based Sensex benchmark's own naming.
     *
     * NOT included: BSE-specific Midcap/Smallcap variants (e.g. S&P
     * BSE 250 SmallCap) - unlike Sensex, these didn't surface in any
     * tracking-error ranking checked, meaning either very low AUM or
     * no fund tracks them closely enough to be reported. If a specific
     * holding benchmarks to one of these, search for it directly in
     * the picker below rather than relying on a recommendation here.
     */
    private data class ProxyFundTarget(val chipLabel: String, val canonicalName: String, val recommendedQuery: String)

    private val proxyFundTargets = listOf(
        ProxyFundTarget("Nifty 50 (index fund proxy)", "NIFTY 50", "UTI Nifty 50 Index Fund"),
        ProxyFundTarget("Nifty 500 (index fund proxy)", "NIFTY 500", "Motilal Oswal Nifty 500 Index Fund"),
        ProxyFundTarget("Nifty Midcap 150 (index fund proxy)", "NIFTY MIDCAP 150", "Motilal Oswal Nifty Midcap 150 Index Fund"),
        ProxyFundTarget("Nifty Smallcap 250 (index fund proxy)", "NIFTY SMALLCAP 250", "SBI Nifty Smallcap 250 Index Fund"),
        ProxyFundTarget("Sensex (index fund proxy)", "Sensex", "UTI BSE Sensex Index Fund")
    )

    private val gson = Gson()
    private val mainHandler = Handler(Looper.getMainLooper())
    private var pendingProxySearchRunnable: Runnable? = null
    private var latestProxySearchQuery: String = ""
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
        val existingTRINames = existing.map { it.niftyTRIIndexName }.toSet()
        for ((name, triIndexName) in knownTRIIndices) {
            if (triIndexName in existingTRINames) continue
            val chip = Chip(this)
            chip.text = name
            chip.isClickable = true
            chip.setOnClickListener { addTRIBenchmark(name, triIndexName) }
            quickAddGroup.addView(chip)
        }
        val existingProxyNames = existing.filter { it.proxyFundISIN.isNotEmpty() }.map { it.name }.toSet()
        for (target in proxyFundTargets) {
            if (target.canonicalName in existingProxyNames) continue
            val chip = Chip(this)
            chip.text = target.chipLabel
            chip.isClickable = true
            chip.setOnClickListener { showProxyFundPicker(target.canonicalName, target.chipLabel, target.recommendedQuery) }
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

    private fun addTRIBenchmark(name: String, niftyTRIIndexName: String) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterAdd = Bridge.addTRIBenchmark(portfolioJson, name, niftyTRIIndexName)
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

    /**
     * Search picker for choosing the ACTUAL fund behind a proxy-fund
     * benchmark - prefilled with a researched recommendation (see
     * proxyFundTargets' doc comment) but the person always confirms the
     * real match from live mfapi.in results, same debounced-search
     * pattern as AdditionalFundsActivity.runSearch (same ANR fix
     * applies here for the same reason - a search box that ever calls
     * Bridge.searchMfapiSchemes must be debounced and backgrounded).
     */
    private fun showProxyFundPicker(canonicalName: String, displayLabel: String, recommendedQuery: String) {
        val density = resources.displayMetrics.density
        val pad = (16 * density).toInt()
        val container = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad, pad, pad)
        }
        val searchInput = EditText(this).apply {
            hint = "Fund name"
            setText(recommendedQuery)
            setSelection(text.length)
        }
        val statusView = android.widget.TextView(this).apply {
            textSize = 12f
            setTextColor(ContextCompat.getColor(this@BenchmarksActivity, R.color.colorNeutral))
            text = "Searching…"
        }
        // A FIXED height, NOT wrap_content - a confirmed real bug: a
        // wrap_content RecyclerView inside an AlertDialog's custom view
        // can end up stuck at zero height once its adapter is swapped
        // in AFTER the dialog is already shown, since the dialog window
        // doesn't reliably re-measure around it - the search results
        // (and the only way to actually add anything, alongside the
        // manual-ISIN fallback below) were invisible with no error and
        // no indication anything had gone wrong. A bounded height has
        // real space to render into from the start, adapter timing or
        // not.
        val resultsView = RecyclerView(this).apply {
            layoutManager = LinearLayoutManager(this@BenchmarksActivity)
        }
        val isinInput = EditText(this).apply {
            hint = "Or paste ISIN directly"
        }
        val isinAddButton = android.widget.Button(this).apply { text = "Add by ISIN" }

        container.addView(searchInput)
        container.addView(statusView, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT).apply {
            topMargin = (4 * density).toInt()
        })
        container.addView(
            resultsView,
            LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, (240 * density).toInt()).apply {
                topMargin = (4 * density).toInt()
            }
        )
        container.addView(isinInput, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT).apply {
            topMargin = (16 * density).toInt()
        })
        container.addView(isinAddButton, LinearLayout.LayoutParams(LinearLayout.LayoutParams.WRAP_CONTENT, LinearLayout.LayoutParams.WRAP_CONTENT).apply {
            topMargin = (6 * density).toInt()
        })

        val titleLabel = displayLabel.removeSuffix(" (index fund proxy)")
        val dialog = AlertDialog.Builder(this)
            .setTitle("Add proxy fund for $titleLabel")
            .setView(container)
            .setNegativeButton("Cancel", null)
            .create()

        fun runProxySearch(query: String) {
            latestProxySearchQuery = query
            statusView.text = "Searching…"
            Thread {
                val resultJson = Bridge.searchMfapiSchemes(query)
                mainHandler.post {
                    if (query != latestProxySearchQuery) return@post // stale - see runSearch's own doc comment in AdditionalFundsActivity
                    if (isBridgeError(resultJson)) {
                        resultsView.adapter = null
                        statusView.text = "Search failed - paste an ISIN below instead"
                        return@post
                    }
                    val matchType = object : TypeToken<List<MfapiSchemeMatch>>() {}.type
                    val matches: List<MfapiSchemeMatch> = try {
                        gson.fromJson(resultJson, matchType) ?: emptyList()
                    } catch (e: Exception) {
                        emptyList()
                    }
                    statusView.text = if (matches.isEmpty()) {
                        "No matches - try a shorter name, or paste an ISIN below"
                    } else {
                        "Tap a match to add it"
                    }
                    resultsView.adapter = MfapiSearchResultsAdapter(matches) { match ->
                        addProxyFundBenchmark(canonicalName, match.isin)
                        dialog.dismiss()
                    }
                }
            }.start()
        }

        searchInput.addTextChangedListener(object : android.text.TextWatcher {
            override fun afterTextChanged(s: android.text.Editable?) {
                val query = s?.toString().orEmpty().trim()
                pendingProxySearchRunnable?.let { mainHandler.removeCallbacks(it) }
                if (query.length < 3) {
                    resultsView.adapter = null
                    statusView.text = "Enter at least 3 characters"
                    return
                }
                val runnable = Runnable { runProxySearch(query) }
                pendingProxySearchRunnable = runnable
                mainHandler.postDelayed(runnable, 400L)
            }
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
        })

        // Manual-ISIN fallback - same reasoning as
        // AdditionalFundsActivity's own ISIN box: search can come up
        // empty (a fund not well-matched by name, or the initial
        // recommendation's search failing outright) and this is the
        // guaranteed way to still finish the add.
        isinAddButton.setOnClickListener {
            val isin = isinInput.text.toString().trim().uppercase()
            if (isin.isEmpty()) {
                isinInput.error = "Enter an ISIN"
                return@setOnClickListener
            }
            addProxyFundBenchmark(canonicalName, isin)
            dialog.dismiss()
        }

        dialog.show()
        // Fire the initial search immediately for the pre-filled
        // recommendation, rather than waiting for the person to type
        // something to retrigger the debounce timer.
        runProxySearch(recommendedQuery)
    }

    private fun addProxyFundBenchmark(targetName: String, isin: String) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterAdd = Bridge.addProxyFundBenchmark(portfolioJson, targetName, isin)
        if (isBridgeError(afterAdd)) {
            showErrorDialog("Failed to add", afterAdd)
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, afterAdd)
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        Toast.makeText(this, "Added $targetName - tap Refresh to fetch its history", Toast.LENGTH_SHORT).show()
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

    /**
     * Shows the full error text in a scrollable, copyable dialog.
     * A plain Toast ellipsizes anything past ~2 lines with no way to
     * read or copy the rest, which made a real diagnosis (e.g. the raw
     * response body FetchNiftyIndicesTRI now includes on parse
     * failure) invisible on-device - see history.go's
     * ParseNiftyIndicesTRI error wrapping.
     */
    private fun showErrorDialog(title: String, message: String) {
        AlertDialog.Builder(this)
            .setTitle(title)
            .setMessage(message)
            .setPositiveButton("OK", null)
            .setNeutralButton("Copy") { _, _ ->
                val clipboard = getSystemService(ClipboardManager::class.java)
                clipboard?.setPrimaryClip(ClipData.newPlainText(title, message))
                Toast.makeText(this, "Copied", Toast.LENGTH_SHORT).show()
            }
            .show()
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
            showErrorDialog("Failed to fetch history", afterFetch)
            rowHolder.status.text = benchmark.proxyFundISIN.ifEmpty { benchmark.niftyTRIIndexName.ifEmpty { benchmark.yahooTicker } }
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
