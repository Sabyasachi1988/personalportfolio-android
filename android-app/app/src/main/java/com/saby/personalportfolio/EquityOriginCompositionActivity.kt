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

class EquityOriginCompositionActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())
    private val isoDateFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_equity_origin_composition)

        val recyclerView = findViewById<RecyclerView>(R.id.equityOriginRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val snapshot: PortfolioEquityOriginSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioEquityOriginSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioEquityOriginSnapshot(emptyList(), emptyList())
        }

        val assets = snapshot.assets.orEmpty()
        val existingByAssetId = snapshot.equityOriginCompositions.orEmpty().associateBy { it.assetId }

        recyclerView.adapter = EquityOriginCompositionAdapter(assets, existingByAssetId) { assetId, indian, international, rowHolder ->
            saveComposition(assetId, indian, international, rowHolder)
        }
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun saveComposition(
        assetId: String,
        indian: Double,
        international: Double,
        rowHolder: EquityOriginCompositionAdapter.RowHolder
    ) {
        rowHolder.saveButton.isEnabled = false

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)

                // Reload fresh each time, same reasoning as
                // CapCompositionActivity - avoids one save clobbering
                // another's earlier write within the same screen session.
                val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { failSave(rowHolder, "Failed to load portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val today = isoDateFormat.format(Date())
                val updatedJson = Bridge.setEquityOriginComposition(
                    currentPortfolioJson, assetId, indian, international, today, "Manual entry (mobile)"
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

    private fun failSave(rowHolder: EquityOriginCompositionAdapter.RowHolder, message: String) {
        rowHolder.saveButton.isEnabled = true
        Toast.makeText(this, message, Toast.LENGTH_LONG).show()
    }
}
