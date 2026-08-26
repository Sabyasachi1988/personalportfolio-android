package com.saby.personalportfolio

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Button
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.ledger.bridge.Bridge
import java.util.concurrent.Executors

class UpdateHistoryActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    private lateinit var runButton: Button
    private lateinit var statusText: TextView

    // Fixed lower bound for FX and symbol-based (ETF/stock) price
    // history - comfortably before any realistic transaction date in
    // this portfolio. NAV history has no such bound: UpdateHistoricalNav
    // always fetches a fund's entire available history.
    private val fxHistorySince = "2015-01-01"

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_update_history)

        runButton = findViewById(R.id.runUpdateHistoryButton)
        statusText = findViewById(R.id.updateHistoryStatusText)

        runButton.setOnClickListener { runUpdate() }
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun runUpdate() {
        runButton.isEnabled = false
        statusText.text = "Loading portfolio…"

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                var portfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(portfolioJson)) {
                    mainThread.post { fail("Failed to load portfolio: $portfolioJson") }
                    return@execute
                }

                val snapshot: PortfolioManualEntrySnapshot = try {
                    gson.fromJson(portfolioJson, PortfolioManualEntrySnapshot::class.java)
                } catch (e: Exception) {
                    mainThread.post { fail("Could not read portfolio: ${e.message}") }
                    return@execute
                }

                val indianAssets = snapshot.assets.orEmpty().filter { it.isin.isNotBlank() }
                // Anything without an ISIN falls outside the mutual-fund
                // NAV path entirely (ETFs, stocks - identified by a
                // Yahoo-style symbol instead, whether NSE-listed like
                // NIFTYBEES.NS or a foreign brokerage holding). This used
                // to be silently skipped altogether - no NAV, no price,
                // ever - which is why manually-entered ETFs never showed
                // a real Value in Portfolio Progression.
                val symbolAssets = snapshot.assets.orEmpty().filter { it.isin.isBlank() && it.symbol.isNotBlank() }
                val foreignCurrencies = snapshot.accounts.orEmpty()
                    .map { it.currency }
                    .filter { it.isNotBlank() && it != "INR" }
                    .distinct()

                var navSucceeded = 0
                val navFailures = mutableListOf<String>()
                for ((index, asset) in indianAssets.withIndex()) {
                    mainThread.post {
                        statusText.text = "Fetching NAV history: ${index + 1} of ${indianAssets.size}…"
                    }
                    val result = Bridge.updateHistoricalNav(portfolioJson, asset.id, asset.isin)
                    if (isBridgeError(result)) {
                        navFailures.add("${FundNameFormatter.shorten(asset.name)}: $result")
                    } else {
                        portfolioJson = result
                        navSucceeded++
                    }
                }

                var priceSucceeded = 0
                val priceFailures = mutableListOf<String>()
                for ((index, asset) in symbolAssets.withIndex()) {
                    mainThread.post {
                        statusText.text = "Fetching ETF/stock prices: ${index + 1} of ${symbolAssets.size}…"
                    }
                    val result = Bridge.updateHistoricalPrice(portfolioJson, asset.id, asset.symbol, fxHistorySince)
                    if (isBridgeError(result)) {
                        priceFailures.add("${FundNameFormatter.shorten(asset.name)} (${asset.symbol}): $result")
                    } else {
                        portfolioJson = result
                        priceSucceeded++
                    }
                }

                var fxSucceeded = 0
                val fxFailures = mutableListOf<String>()
                for ((index, currency) in foreignCurrencies.withIndex()) {
                    mainThread.post {
                        statusText.text = "Fetching FX rates: ${index + 1} of ${foreignCurrencies.size}…"
                    }
                    val result = Bridge.updateHistoricalFX(portfolioJson, currency, fxHistorySince)
                    if (isBridgeError(result)) {
                        fxFailures.add("$currency: $result")
                    } else {
                        portfolioJson = result
                        fxSucceeded++
                    }
                }

                val saveResult = Bridge.savePortfolio(portfolioPath, portfolioJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { fail("Failed to save: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    runButton.isEnabled = true
                    val summary = StringBuilder()
                    summary.append("NAV history: $navSucceeded of ${indianAssets.size} fund(s) updated.\n")
                    summary.append("ETF/stock prices: $priceSucceeded of ${symbolAssets.size} updated.\n")
                    summary.append("FX history: $fxSucceeded of ${foreignCurrencies.size} currenc${if (foreignCurrencies.size == 1) "y" else "ies"} updated.")
                    if (navFailures.isNotEmpty() || priceFailures.isNotEmpty() || fxFailures.isNotEmpty()) {
                        summary.append("\n\nFailures:\n")
                        (navFailures + priceFailures + fxFailures).forEach { summary.append("• $it\n") }
                    }
                    statusText.text = summary.toString()
                }
            } catch (e: Exception) {
                mainThread.post { fail("Update failed: ${e.message}") }
            }
        }
    }

    private fun fail(message: String) {
        statusText.text = message
        runButton.isEnabled = true
    }
}
