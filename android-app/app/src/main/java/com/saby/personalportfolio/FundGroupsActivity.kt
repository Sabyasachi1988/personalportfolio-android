package com.saby.personalportfolio

import android.os.Bundle
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.ledger.bridge.Bridge

class FundGroupsActivity : AppCompatActivity() {

    private val gson = Gson()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_fund_groups)

        val recyclerView = findViewById<RecyclerView>(R.id.fundGroupsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        loadAndBindAdapter(recyclerView)
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadAndBindAdapter(recyclerView: RecyclerView) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)

        val snapshot: PortfolioAssetsSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioAssetsSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioAssetsSnapshot(emptyList(), emptyList(), emptyList())
        }

        val assets = snapshot.assets.orEmpty().sortedBy { FundNameFormatter.shorten(it.name) }
        recyclerView.adapter = FundGroupsAdapter(assets) { assetId, label, rowHolder ->
            saveLabel(assetId, label, rowHolder)
        }
    }

    private fun saveLabel(assetId: String, label: String, rowHolder: FundGroupsAdapter.RowHolder) {
        rowHolder.saveButton.isEnabled = false
        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            rowHolder.saveButton.isEnabled = true
            Toast.makeText(this, "Failed to load portfolio: $currentPortfolioJson", Toast.LENGTH_LONG).show()
            return
        }
        val updatedJson = Bridge.setAssetGroupLabel(currentPortfolioJson, assetId, label)
        if (isBridgeError(updatedJson)) {
            rowHolder.saveButton.isEnabled = true
            Toast.makeText(this, "Failed to set group label: $updatedJson", Toast.LENGTH_LONG).show()
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
        rowHolder.saveButton.isEnabled = true
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        Toast.makeText(this, if (label.isBlank()) "Ungrouped" else "Saved: $label", Toast.LENGTH_SHORT).show()
    }
}
