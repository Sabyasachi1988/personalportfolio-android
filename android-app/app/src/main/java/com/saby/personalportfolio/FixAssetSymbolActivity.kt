package com.saby.personalportfolio

import android.os.Bundle
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.ledger.bridge.Bridge

class FixAssetSymbolActivity : AppCompatActivity() {

    private val gson = Gson()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_fix_asset_symbol)

        val recyclerView = findViewById<RecyclerView>(R.id.fixAssetSymbolRecyclerView)
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
        recyclerView.adapter = FixAssetSymbolAdapter(assets) { assetId, symbol, type, rowHolder ->
            saveSymbolAndType(assetId, symbol, type, rowHolder)
        }
    }

    private fun saveSymbolAndType(assetId: String, symbol: String, type: String, rowHolder: FixAssetSymbolAdapter.RowHolder) {
        rowHolder.saveButton.isEnabled = false
        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            rowHolder.saveButton.isEnabled = true
            Toast.makeText(this, "Failed to load portfolio: $currentPortfolioJson", Toast.LENGTH_LONG).show()
            return
        }
        val updatedJson = Bridge.setAssetSymbolAndType(currentPortfolioJson, assetId, symbol, type)
        if (isBridgeError(updatedJson)) {
            rowHolder.saveButton.isEnabled = true
            Toast.makeText(this, "Failed to save: $updatedJson", Toast.LENGTH_LONG).show()
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
        rowHolder.saveButton.isEnabled = true
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        Toast.makeText(this, "Saved", Toast.LENGTH_SHORT).show()
    }
}
