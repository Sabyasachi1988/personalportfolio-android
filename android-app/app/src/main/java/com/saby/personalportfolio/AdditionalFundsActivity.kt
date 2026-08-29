package com.saby.personalportfolio

import android.os.Bundle
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
    private lateinit var searchInput: android.widget.EditText
    private lateinit var searchResultsView: RecyclerView
    private lateinit var isinInput: android.widget.EditText
    private lateinit var recyclerView: RecyclerView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_additional_funds)

        searchInput = findViewById(R.id.additionalFundsSearchInput)
        searchResultsView = findViewById(R.id.additionalFundsSearchResults)
        searchResultsView.layoutManager = LinearLayoutManager(this)
        isinInput = findViewById(R.id.additionalFundsIsinInput)
        recyclerView = findViewById(R.id.additionalFundsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

        // Searches on every keystroke past the 3-character floor, same
        // as SearchMfapiSchemes' own minimum (see its Go doc comment) -
        // simple debounce-free live search since this codebase has no
        // existing coroutine/threading pattern for bridge calls (see
        // this class's own note on that below) and mfapi.in's scheme
        // list is cached after the first fetch, so repeat searches are
        // fast local filtering, not repeat downloads.
        searchInput.addTextChangedListener(object : android.text.TextWatcher {
            override fun afterTextChanged(s: android.text.Editable?) {
                runSearch(s?.toString().orEmpty())
            }
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
        })

        findViewById<android.view.View>(R.id.additionalFundsIsinAddButton).setOnClickListener {
            val isin = isinInput.text.toString().trim().uppercase()
            if (isin.isEmpty()) {
                isinInput.error = "Enter an ISIN"
                return@setOnClickListener
            }
            addTrackedFund(isin, isin) // name defaults to the ISIN itself when added this way - refreshed to the real fund name isn't available from a bare ISIN, only from a search hit or an eventual NAV fetch
            isinInput.text.clear()
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

    // SearchMfapiSchemes is a synchronous, network-bound Go call (see
    // its own doc comment) - called directly on the main thread here,
    // matching every other network-bound bridge call in this codebase
    // (e.g. BenchmarksActivity.refreshHistory), which has no existing
    // coroutine/background-thread convention to plug into. The first
    // search in a session may pause briefly while mfapi.in's full
    // scheme list downloads; every search after that is fast (cached).
    private fun runSearch(query: String) {
        if (query.trim().length < 3) {
            searchResultsView.adapter = null
            return
        }
        val resultJson = Bridge.searchMfapiSchemes(query.trim())
        if (isBridgeError(resultJson)) {
            searchResultsView.adapter = null
            return
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

    private fun reload() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val snapshot: PortfolioAssetsSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioAssetsSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioAssetsSnapshot(emptyList(), emptyList())
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

    private fun deleteTrackedFund(fund: TrackedFundAsset) {
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

    private fun refreshHistory(fund: TrackedFundAsset, rowHolder: AdditionalFundsAdapter.RowHolder) {
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
