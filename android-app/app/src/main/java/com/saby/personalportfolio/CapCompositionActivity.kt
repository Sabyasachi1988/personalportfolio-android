package com.saby.personalportfolio

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.Executors

class CapCompositionActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())
    private val isoDateFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_cap_composition)

        val recyclerView = findViewById<RecyclerView>(R.id.capCompositionRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

        loadAndBindAdapter(recyclerView)
    }

    private fun loadAndBindAdapter(recyclerView: RecyclerView) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val snapshot: PortfolioAssetsSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioAssetsSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioAssetsSnapshot(emptyList(), emptyList(), emptyList())
        }

        val assets = snapshot.assets.orEmpty()
        val existingByAssetId = snapshot.capCompositions.orEmpty().associateBy { it.assetId }

        recyclerView.adapter = CapCompositionAdapter(
            assets, existingByAssetId,
            onSave = { assetId, large, mid, small, cash, rowHolder ->
                saveComposition(assetId, large, mid, small, cash, rowHolder)
            },
            onFetchFromEtMoney = { assetId, url, rowHolder ->
                fetchFromEtMoney(assetId, url, rowHolder)
            }
        )
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun saveComposition(
        assetId: String,
        large: Double,
        mid: Double,
        small: Double,
        cash: Double,
        rowHolder: CapCompositionAdapter.RowHolder
    ) {
        rowHolder.saveButton.isEnabled = false

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)

                // Reload fresh each time, in case another row was saved
                // earlier in this same screen session - avoids one save
                // clobbering another's earlier write.
                val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { failSave(rowHolder, "Failed to load portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val today = isoDateFormat.format(Date())
                val updatedJson = Bridge.setCapComposition(
                    currentPortfolioJson, assetId, large, mid, small, cash, today, "Manual entry (mobile)"
                )
                if (isBridgeError(updatedJson)) {
                    mainThread.post { failSave(rowHolder, "Failed to set composition: $updatedJson") }
                    return@execute
                }

                val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { failSave(rowHolder, "Failed to save: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    rowHolder.saveButton.isEnabled = true
                    rowHolder.lastEnteredLabel.text = "Last entered: $today (Manual entry (mobile))"
                    Toast.makeText(this, "Saved", Toast.LENGTH_SHORT).show()
                }
            } catch (e: Exception) {
                mainThread.post { failSave(rowHolder, "Failed: ${e.message}") }
            }
        }
    }

    private fun failSave(rowHolder: CapCompositionAdapter.RowHolder, message: String) {
        rowHolder.saveButton.isEnabled = true
        Toast.makeText(this, message, Toast.LENGTH_LONG).show()
    }

    /**
     * Two-step, same pattern as everywhere else that touches the
     * network: fetch + link the URL first, then populate the four
     * input fields for the person to REVIEW (not auto-save) - this
     * fetch is unverified against the live ETMoney site (see
     * priceapi.FetchETMoneyCapComposition's doc comment), so a wrong or
     * partial parse must be something the person visibly catches before
     * it's written anywhere, not something that silently overwrites a
     * correct manual entry.
     */
    private fun fetchFromEtMoney(assetId: String, url: String, rowHolder: CapCompositionAdapter.RowHolder) {
        rowHolder.fetchButton.isEnabled = false
        rowHolder.etMoneyStatusLabel.visibility = android.view.View.VISIBLE
        rowHolder.etMoneyStatusLabel.text = "Fetching…"

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)

                // Link the URL to the asset regardless of whether the
                // fetch below succeeds, so it doesn't need re-typing on
                // a retry.
                val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
                if (!isBridgeError(currentPortfolioJson)) {
                    val linkedJson = Bridge.setAssetETMoneyURL(currentPortfolioJson, assetId, url)
                    if (!isBridgeError(linkedJson)) {
                        Bridge.savePortfolio(portfolioPath, linkedJson)
                    }
                }

                val fetchResultJson = Bridge.fetchCapCompositionFromETMoney(url)
                if (isBridgeError(fetchResultJson)) {
                    val errorMessage = try {
                        gson.fromJson(fetchResultJson, com.google.gson.JsonObject::class.java)
                            .get("error")?.asString ?: fetchResultJson
                    } catch (e: Exception) {
                        fetchResultJson
                    }
                    mainThread.post { failFetch(rowHolder, "Could not fetch: $errorMessage") }
                    return@execute
                }

                val result = try {
                    gson.fromJson(fetchResultJson, ETMoneyFetchResult::class.java)
                } catch (e: Exception) {
                    mainThread.post { failFetch(rowHolder, "Fetch returned unexpected data: ${e.message}") }
                    return@execute
                }

                mainThread.post {
                    rowHolder.fetchButton.isEnabled = true
                    rowHolder.largeInput.setText(formatPercent(result.large))
                    rowHolder.midInput.setText(formatPercent(result.mid))
                    rowHolder.smallInput.setText(formatPercent(result.small))
                    rowHolder.cashInput.setText(formatPercent(result.cash))

                    val sumWarning = if (result.matchedSum < 90.0 || result.matchedSum > 110.0) {
                        " ⚠ these sum to ${String.format(Locale.getDefault(), "%.1f", result.matchedSum)}%, check before saving"
                    } else {
                        ""
                    }
                    rowHolder.etMoneyStatusLabel.visibility = android.view.View.VISIBLE
                    rowHolder.etMoneyStatusLabel.text = "Fetched Large/Mid/Small — Cash set to 100 − their sum. Review below, then tap Save.$sumWarning"
                }
            } catch (e: Exception) {
                mainThread.post { failFetch(rowHolder, "Failed: ${e.message}") }
            }
        }
    }

    private fun formatPercent(value: Double): String =
        if (value == 0.0) "" else String.format(Locale.getDefault(), "%.2f", value)

    private fun failFetch(rowHolder: CapCompositionAdapter.RowHolder, message: String) {
        rowHolder.fetchButton.isEnabled = true
        rowHolder.etMoneyStatusLabel.visibility = android.view.View.VISIBLE
        rowHolder.etMoneyStatusLabel.text = message
    }
}
