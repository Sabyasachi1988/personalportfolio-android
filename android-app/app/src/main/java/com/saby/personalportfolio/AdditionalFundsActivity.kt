package com.saby.personalportfolio

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

/**
 * Manages "Additional Funds" - funds tracked purely for comparison, not
 * actually owned (see store.Asset.AccountID's Go doc comment). A
 * deliberately SEPARATE screen from BenchmarksActivity even though the
 * two look structurally similar (a quick-add area + a list with
 * Refresh/Delete per row) - a tracked FUND and a market INDEX are a
 * different mental category to the person using this, not just a
 * different data source under the hood.
 */
class AdditionalFundsActivity : AppCompatActivity() {

    private val gson = Gson()
    private val mainHandler = Handler(Looper.getMainLooper())
    private lateinit var searchInput: android.widget.EditText
    private lateinit var searchResultsView: RecyclerView
    private lateinit var isinInput: android.widget.EditText
    private lateinit var isinAddButton: android.widget.Button
    private lateinit var recyclerView: RecyclerView

    // Debounce + staleness-guard state for the live name search - see
    // this class's own note on runSearch for the CONFIRMED REAL BUG
    // this fixes (a hang/ANR on the 3rd keystroke): the very first
    // search of a session triggers mfapi.in's full ~38,000-scheme list
    // download, which is multi-MB and multi-second - running that on
    // the main thread (the original implementation) froze the entire
    // app for however long that download took. pendingSearchRunnable
    // lets a fresh keystroke cancel a not-yet-fired debounce timer;
    // latestSearchQuery lets a slow, now-STALE background search's
    // result be silently discarded if a newer query has since been
    // typed, rather than flashing outdated results onto the screen.
    private var pendingSearchRunnable: Runnable? = null
    private var latestSearchQuery: String = ""

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_additional_funds)

        searchInput = findViewById(R.id.additionalFundsSearchInput)
        searchResultsView = findViewById(R.id.additionalFundsSearchResults)
        searchResultsView.layoutManager = LinearLayoutManager(this)
        isinInput = findViewById(R.id.additionalFundsIsinInput)
        isinAddButton = findViewById(R.id.additionalFundsIsinAddButton)
        recyclerView = findViewById(R.id.additionalFundsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

        // Debounced (400ms after typing stops), not on every keystroke -
        // see this class's own doc comment on pendingSearchRunnable for
        // why a naive every-keystroke version was a confirmed real bug.
        searchInput.addTextChangedListener(object : android.text.TextWatcher {
            override fun afterTextChanged(s: android.text.Editable?) {
                val query = s?.toString().orEmpty()
                pendingSearchRunnable?.let { mainHandler.removeCallbacks(it) }
                if (query.trim().length < 3) {
                    searchResultsView.adapter = null
                    return
                }
                val runnable = Runnable { runSearch(query.trim()) }
                pendingSearchRunnable = runnable
                mainHandler.postDelayed(runnable, 400L)
            }
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
        })

        isinAddButton.setOnClickListener {
            val isin = isinInput.text.toString().trim().uppercase()
            if (isin.isEmpty()) {
                isinInput.error = "Enter an ISIN"
                return@setOnClickListener
            }
            addTrackedFundByISIN(isin)
        }

        reload()
    }

    override fun onResume() {
        super.onResume()
        // A resume can follow a promotion (the person imported a
        // statement for a fund that WAS tracked here) - reload so a
        // now-owned fund disappears from this list without needing a
        // manual pull-to-refresh.
        reload()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    /**
     * Looks up funds by name against mfapi.in's scheme list, run on a
     * BACKGROUND thread - this is the fix for a confirmed real bug: the
     * original implementation called this directly on the main thread,
     * and since the first search of a session has to download mfapi.in's
     * entire ~38,000-scheme list first (multi-MB, can take several
     * seconds on a slow connection), that froze the whole app long
     * enough to trigger Android's ANR ("app not responding") dialog on
     * roughly the 3rd keystroke, once the earlier debounce-free version
     * had already queued several blocking calls back to back. This
     * codebase has no existing coroutine convention (see other Activity
     * classes) - a plain background Thread + posting the result back to
     * the main thread via mainHandler is the minimal fix, not a new
     * framework-wide pattern.
     */
    private fun runSearch(query: String) {
        latestSearchQuery = query
        Thread {
            val resultJson = Bridge.searchMfapiSchemes(query)
            mainHandler.post {
                // Discard a stale result - see latestSearchQuery's own
                // doc comment above.
                if (query != latestSearchQuery) return@post
                if (isBridgeError(resultJson)) {
                    searchResultsView.adapter = null
                    return@post
                }
                val matchType = object : TypeToken<List<MfapiSchemeMatch>>() {}.type
                val matches: List<MfapiSchemeMatch> = try {
                    gson.fromJson(resultJson, matchType) ?: emptyList()
                } catch (e: Exception) {
                    emptyList()
                }
                searchResultsView.adapter = MfapiSearchResultsAdapter(matches) { match ->
                    addTrackedFund(match.name, match.isin)
                    searchInput.text.clear()
                    searchResultsView.adapter = null
                }
            }
        }.start()
    }

    private fun reload() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val snapshot: AdditionalFundsSnapshot = try {
            gson.fromJson(portfolioJson, AdditionalFundsSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            AdditionalFundsSnapshot(emptyList(), emptyList())
        }
        // Tracked (not owned) is exactly AccountID == "" - see
        // store.Asset.AccountID's own Go doc comment.
        val trackedFunds = snapshot.assets.orEmpty().filter { it.accountId.isEmpty() }
        val pricedSeriesIds = snapshot.prices.orEmpty().map { it.seriesId }.toSet()

        recyclerView.adapter = AdditionalFundsAdapter(
            funds = trackedFunds,
            hasHistory = { id -> id in pricedSeriesIds },
            onRefresh = { fund, rowHolder -> refreshHistory(fund, rowHolder) },
            onDelete = { fund -> deleteTrackedFund(fund) }
        )
    }

    /**
     * Adding by a bare ISIN first tries to resolve the fund's REAL name
     * from mfapi.in (also on a background thread - same ANR reasoning
     * as runSearch) before adding it - a confirmed real complaint with
     * the old behavior, which stored the ISIN itself as the fund's
     * "name", making it genuinely hard to recognize in any list. Falls
     * back to the bare ISIN only if resolution itself fails (e.g. no
     * network, or a genuinely unrecognized ISIN) - the fund is still
     * added either way, never blocked on this lookup succeeding.
     */
    private fun addTrackedFundByISIN(isin: String) {
        isinAddButton.isEnabled = false
        Thread {
            val resultJson = Bridge.resolveFundNameByISIN(isin)
            val resolvedName = if (!isBridgeError(resultJson)) {
                try {
                    gson.fromJson(resultJson, IsinNameResolution::class.java)?.name
                } catch (e: Exception) {
                    null
                }
            } else {
                null
            }
            mainHandler.post {
                isinAddButton.isEnabled = true
                addTrackedFund(resolvedName?.takeIf { it.isNotBlank() } ?: isin, isin)
                isinInput.text.clear()
            }
        }.start()
    }

    private fun addTrackedFund(name: String, isin: String) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterAdd = Bridge.addTrackedFund(portfolioJson, name, isin)
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

    private fun deleteTrackedFund(fund: AssetSummary) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterRemove = Bridge.removeTrackedFund(portfolioJson, fund.id)
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

    private fun refreshHistory(fund: AssetSummary, rowHolder: AdditionalFundsAdapter.RowHolder) {
        rowHolder.refreshButton.isEnabled = false
        rowHolder.status.text = "Fetching…"
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterFetch = Bridge.updateHistoricalNav(portfolioJson, fund.id, fund.isin)
        rowHolder.refreshButton.isEnabled = true
        if (isBridgeError(afterFetch)) {
            Toast.makeText(this, "Failed to fetch history: $afterFetch", Toast.LENGTH_LONG).show()
            rowHolder.status.text = fund.isin
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, afterFetch)
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        Toast.makeText(this, "${fund.name} history updated", Toast.LENGTH_SHORT).show()
        reload()
    }
}
