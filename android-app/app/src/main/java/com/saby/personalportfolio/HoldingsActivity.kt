package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Button
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

    // Index 0 is always "All (family)" (empty memberID); indices 1.. map
    // 1:1 with memberIds.
    private var memberIds: List<String> = emptyList()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_holdings)

        summary = findViewById(R.id.holdingsSummary)
        xirrSummary = findViewById(R.id.xirrSummary)
        recyclerView = findViewById(R.id.holdingsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        refreshButton = findViewById(R.id.refreshPricesButton)
        memberSpinner = findViewById(R.id.memberFilterSpinner)

        refreshButton.setOnClickListener { refreshPrices() }
        findViewById<Button>(R.id.viewAllocationButton).setOnClickListener {
            startActivity(Intent(this, AllocationActivity::class.java))
        }
        findViewById<Button>(R.id.manageTransactionsButton).setOnClickListener {
            startActivity(Intent(this, TransactionsActivity::class.java))
        }

        memberSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                showHoldingsForSelectedMember()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }
    }

    override fun onResume() {
        super.onResume()
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
                String.format(
                    Locale.getDefault(),
                    "%d holdings | Invested: ₹%.2f | Value: ₹%.2f | Gain: ₹%.2f",
                    holdings.size, totalInvested, totalValue, totalGain
                )
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

        recyclerView.adapter = HoldingsAdapter(holdings)
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

private data class PortfolioXirrResult(
    val xirr: Double,
    val hasXIRR: Boolean
)
