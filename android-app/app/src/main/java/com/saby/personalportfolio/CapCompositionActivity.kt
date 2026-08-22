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

        recyclerView.adapter = CapCompositionAdapter(assets, existingByAssetId) { assetId, large, mid, small, cash, rowHolder ->
            saveComposition(assetId, large, mid, small, cash, rowHolder)
        }
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
}
