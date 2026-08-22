package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Button
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

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_holdings)

        summary = findViewById(R.id.holdingsSummary)
        xirrSummary = findViewById(R.id.xirrSummary)
        recyclerView = findViewById(R.id.holdingsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        refreshButton = findViewById(R.id.refreshPricesButton)

        refreshButton.setOnClickListener { refreshPrices() }
        findViewById<Button>(R.id.viewAllocationButton).setOnClickListener {
            startActivity(Intent(this, AllocationActivity::class.java))
        }

        loadAndShowHoldings()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadAndShowHoldings() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val holdingsJson = Bridge.computeHoldings(portfolioJson)

        val holdingsType = object : TypeToken<List<Holding>>() {}.type
        val holdings: List<Holding> = try {
            gson.fromJson(holdingsJson, holdingsType) ?: emptyList()
        } catch (e: Exception) {
            summary.text = "Could not read holdings: ${e.message}"
            emptyList()
        }

        if (holdings.isEmpty()) {
            summary.text = "No holdings yet. Import a CAS PDF first."
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

                // This is a real network call (AMFI's NAV file, several
                // hundred KB) - can genuinely fail from no connectivity,
                // AMFI being down, or a slow connection timing out.
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
                    loadAndShowHoldings()
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

// Matches RefreshAmfiPrices' {"matchedCount":N,"portfolio":{...}} shape.
// The "portfolio" field is left as a raw JsonObject rather than a typed
// Portfolio data class, since it just needs to be round-tripped straight
// into SavePortfolio, not read field-by-field on the Kotlin side.
private data class RefreshPricesResult(
    val matchedCount: Int,
    val portfolio: com.google.gson.JsonObject
)

private data class PortfolioXirrResult(
    val xirr: Double,
    val hasXIRR: Boolean
)
