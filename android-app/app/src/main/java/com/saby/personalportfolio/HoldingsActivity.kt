package com.saby.personalportfolio

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.Editable
import android.text.TextWatcher
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.util.Locale
import java.util.concurrent.Executors

class HoldingsActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    private lateinit var summary: TextView
    private lateinit var xirrSummary: TextView
    private lateinit var recyclerView: RecyclerView
    private lateinit var refreshButton: Button
    private lateinit var memberSpinner: Spinner
    private lateinit var searchInput: EditText
    private lateinit var sortSpinner: Spinner

    // Index 0 is always "All (family)" (empty memberID); indices 1.. map
    // 1:1 with memberIds.
    private var memberIds: List<String> = emptyList()

    // The full member-filtered (but not yet search/sort-filtered) list -
    // totals and XIRR are always computed from this, so they stay stable
    // while searching rather than confusingly changing as you type.
    private var allHoldings: List<Holding> = emptyList()

    private val sortOptions = listOf(
        "Value (high to low)", "Value (low to high)",
        "Gain % (high to low)", "Gain % (low to high)",
        "Name (A-Z)"
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_holdings)

        summary = findViewById(R.id.holdingsSummary)
        xirrSummary = findViewById(R.id.xirrSummary)
        recyclerView = findViewById(R.id.holdingsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        refreshButton = findViewById(R.id.refreshPricesButton)
        memberSpinner = findViewById(R.id.memberFilterSpinner)
        searchInput = findViewById(R.id.holdingsSearchInput)
        sortSpinner = findViewById(R.id.holdingsSortSpinner)

        refreshButton.setOnClickListener { refreshPrices() }
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.HOLDINGS)

        memberSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                showHoldingsForSelectedMember()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }

        sortSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, sortOptions)
        sortSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                applyFilterAndSort()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }

        searchInput.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: Editable?) {
                applyFilterAndSort()
            }
        })
    }

    override fun onResume() {
        super.onResume()
        // Re-sync every time this screen resumes, not just once in
        // onCreate - a screen reused via CLEAR_TOP (coming back to it
        // from another tab) never re-runs onCreate, so without this
        // its nav bar could keep showing a stale selection.
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.HOLDINGS)
        // Refresh the member list every time this screen becomes visible
        // (in particular after a new CAS import may have added a new
        // member), then show holdings for whichever filter is selected.
        loadMemberSpinner()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadMemberSpinner() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val membersJson = Bridge.listMembers(portfolioJson)

        val memberType = object : TypeToken<List<Member>>() {}.type
        val members: List<Member> = try {
            gson.fromJson(membersJson, memberType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        val previousSelection = memberSpinner.selectedItemPosition.takeIf { it >= 0 } ?: 0

        memberIds = listOf("") + members.map { it.id }
        val labels = listOf("All (family)") + members.map { it.name }
        memberSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, labels)

        // Keep the same selection if it's still valid, rather than always
        // resetting to "All" on every refresh.
        memberSpinner.setSelection(previousSelection.coerceAtMost(memberIds.size - 1))

        showHoldingsForSelectedMember()
    }

    private fun showHoldingsForSelectedMember() {
        val selectedIndex = memberSpinner.selectedItemPosition
        val memberId = memberIds.getOrElse(selectedIndex) { "" }
        loadAndShowHoldings(memberId)
    }

    private fun loadAndShowHoldings(memberId: String) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val holdingsJson = Bridge.computeHoldingsForMember(portfolioJson, memberId)

        val holdingsType = object : TypeToken<List<Holding>>() {}.type
        val holdings: List<Holding> = try {
            gson.fromJson(holdingsJson, holdingsType) ?: emptyList()
        } catch (e: Exception) {
            summary.text = "Could not read holdings: ${e.message}"
            emptyList()
        }

        allHoldings = holdings

        if (holdings.isEmpty()) {
            summary.text = "No holdings yet for this filter."
            xirrSummary.text = ""
        } else {
            var totalInvested = 0.0
            var totalValue = 0.0
            var anyPriced = false
            for (h in holdings) {
                if (h.hasPrice) {
                    totalInvested += h.netInvested
                    totalValue += h.currentValue
                    anyPriced = true
                }
            }
            summary.text = if (anyPriced) {
                val totalGain = totalValue - totalInvested
                "${holdings.size} holdings | Invested: ${IndianCurrencyFormatter.format(totalInvested)} | " +
                    "Value: ${IndianCurrencyFormatter.format(totalValue)} | Gain: ${IndianCurrencyFormatter.formatSigned(totalGain)}"
            } else {
                "${holdings.size} holdings (no current prices available yet — tap Refresh Prices)"
            }

            // Portfolio XIRR is computed for whichever holdings are
            // currently shown, so switching the member filter also scopes
            // the XIRR - matches PortfolioXIRR's own filtering behavior.
            xirrSummary.text = if (anyPriced) {
                val xirrJson = Bridge.computePortfolioXIRR(portfolioJson)
                val xirrResult = try {
                    gson.fromJson(xirrJson, PortfolioXirrResult::class.java)
                } catch (e: Exception) {
                    null
                }
                if (xirrResult?.hasXIRR == true) {
                    String.format(Locale.getDefault(), "Portfolio XIRR: %.2f%%", xirrResult.xirr)
                } else {
                    ""
                }
            } else {
                ""
            }
        }

        applyFilterAndSort()
    }

    private fun applyFilterAndSort() {
        val query = searchInput.text?.toString()?.trim().orEmpty()
        var result = if (query.isBlank()) {
            allHoldings
        } else {
            allHoldings.filter { it.assetName.contains(query, ignoreCase = true) }
        }

        result = when (sortSpinner.selectedItemPosition) {
            0 -> result.sortedByDescending { it.currentValue }
            1 -> result.sortedBy { it.currentValue }
            2 -> result.sortedByDescending { it.gainPercent }
            3 -> result.sortedBy { it.gainPercent }
            4 -> result.sortedBy { FundNameFormatter.shorten(it.assetName) }
            else -> result
        }

        recyclerView.adapter = HoldingsAdapter(result)
    }

    private fun refreshPrices() {
        refreshButton.isEnabled = false
        summary.text = "Fetching current AMFI prices…"

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)

                val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { failRefresh("Failed to load portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val refreshResult = Bridge.refreshAmfiPrices(currentPortfolioJson)
                if (isBridgeError(refreshResult)) {
                    mainThread.post { failRefresh("Price refresh failed: $refreshResult") }
                    return@execute
                }

                val parsed = try {
                    gson.fromJson(refreshResult, RefreshPricesResult::class.java)
                } catch (e: Exception) {
                    mainThread.post { failRefresh("Price refresh returned unexpected data: ${e.message}") }
                    return@execute
                }

                val updatedPortfolioJson = gson.toJson(parsed.portfolio)
                val saveResult = Bridge.savePortfolio(portfolioPath, updatedPortfolioJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { failRefresh("Failed to save refreshed prices: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    refreshButton.isEnabled = true
                    Toast.makeText(
                        this,
                        "Matched prices for ${parsed.matchedCount} holding(s)",
                        Toast.LENGTH_LONG
                    ).show()
                    showHoldingsForSelectedMember()
                }
            } catch (e: Exception) {
                mainThread.post { failRefresh("Price refresh failed: ${e.message}") }
            }
        }
    }

    private fun failRefresh(message: String) {
        summary.text = message
        refreshButton.isEnabled = true
    }
}

private data class RefreshPricesResult(
    val matchedCount: Int,
    val portfolio: com.google.gson.JsonObject
)

data class PortfolioXirrResult(
    val xirr: Double,
    val hasXIRR: Boolean
)
