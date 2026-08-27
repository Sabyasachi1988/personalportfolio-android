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

/**
 * Fetches NAV/ETF-stock/FX/index history for the whole portfolio in one
 * combined, concurrent bridge call (Bridge.updateAllHistory) instead of
 * looping over 4 separate per-item calls one at a time. Each individual
 * fetch is already incremental (see bridge.UpdateHistoricalNav's doc
 * comment), but a portfolio with many funds/indices still has many
 * independent network round trips to make - doing them one at a time
 * meant their LATENCY summed up even once each fetch itself returned
 * almost nothing, a confirmed real report ("still 12-13 seconds even
 * when nothing changed"). Running them concurrently (see
 * bridge.UpdateAllHistory's own doc comment for the concurrency-safety
 * reasoning) turns that into roughly the single slowest round trip
 * instead of the sum of all of them.
 */
class UpdateHistoryActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    private lateinit var runButton: Button
    private lateinit var statusText: TextView

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
        statusText.text = "Fetching everything at once…"

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                val portfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(portfolioJson)) {
                    mainThread.post { fail("Failed to load portfolio: $portfolioJson") }
                    return@execute
                }

                val resultJson = Bridge.updateAllHistory(portfolioJson)
                if (isBridgeError(resultJson)) {
                    mainThread.post { fail("Update failed: $resultJson") }
                    return@execute
                }

                val result: UpdateAllHistoryResult = try {
                    gson.fromJson(resultJson, UpdateAllHistoryResult::class.java)
                } catch (e: Exception) {
                    mainThread.post { fail("Update returned unexpected data: ${e.message}") }
                    return@execute
                }

                val saveResult = Bridge.savePortfolio(portfolioPath, result.portfolioJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { fail("Failed to save: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    runButton.isEnabled = true
                    val summary = StringBuilder()
                    summary.append("NAV history: ${result.navSucceeded} of ${result.navTotal} fund(s) updated.\n")
                    summary.append("ETF/stock prices: ${result.priceSucceeded} of ${result.priceTotal} updated.\n")
                    summary.append("FX history: ${result.fxSucceeded} of ${result.fxTotal} currenc${if (result.fxTotal == 1) "y" else "ies"} updated.\n")
                    summary.append("Index history: ${result.benchmarkSucceeded} of ${result.benchmarkTotal} updated.")
                    val allFailures = (result.navFailures.orEmpty() + result.priceFailures.orEmpty() +
                        result.fxFailures.orEmpty() + result.benchmarkFailures.orEmpty())
                    if (allFailures.isNotEmpty()) {
                        summary.append("\n\nFailures:\n")
                        allFailures.forEach { summary.append("• $it\n") }
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
