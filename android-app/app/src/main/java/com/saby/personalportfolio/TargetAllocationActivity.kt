package com.saby.personalportfolio

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.ledger.bridge.Bridge

class TargetAllocationActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var statusText: TextView
    private lateinit var largeInput: EditText
    private lateinit var midInput: EditText
    private lateinit var smallInput: EditText
    private lateinit var cashInput: EditText

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_target_allocation)

        statusText = findViewById(R.id.targetStatusText)
        largeInput = findViewById(R.id.targetLargeInput)
        midInput = findViewById(R.id.targetMidInput)
        smallInput = findViewById(R.id.targetSmallInput)
        cashInput = findViewById(R.id.targetCashInput)

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val snapshot: PortfolioTargetSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioTargetSnapshot::class.java)
        } catch (e: Exception) {
            PortfolioTargetSnapshot(null)
        }
        snapshot.targetAllocation?.let { t ->
            largeInput.setText(t.large.takeIf { it != 0.0 }?.toString() ?: "")
            midInput.setText(t.mid.takeIf { it != 0.0 }?.toString() ?: "")
            smallInput.setText(t.small.takeIf { it != 0.0 }?.toString() ?: "")
            cashInput.setText(t.cash.takeIf { it != 0.0 }?.toString() ?: "")
        }

        findViewById<Button>(R.id.saveTargetButton).setOnClickListener { save() }
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun save() {
        val large = largeInput.text.toString().toDoubleOrNull() ?: 0.0
        val mid = midInput.text.toString().toDoubleOrNull() ?: 0.0
        val small = smallInput.text.toString().toDoubleOrNull() ?: 0.0
        val cash = cashInput.text.toString().toDoubleOrNull() ?: 0.0

        if (large == 0.0 && mid == 0.0 && small == 0.0 && cash == 0.0) {
            statusText.text = "Enter at least one nonzero percentage."
            return
        }

        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            statusText.text = "Failed to load portfolio: $currentPortfolioJson"
            return
        }

        val updatedJson = Bridge.setTargetAllocation(currentPortfolioJson, large, mid, small, cash)
        if (isBridgeError(updatedJson)) {
            statusText.text = "Failed to set target: $updatedJson"
            return
        }

        val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
        if (isBridgeError(saveResult)) {
            statusText.text = "Failed to save: $saveResult"
            return
        }

        statusText.text = "Target saved."
        Toast.makeText(this, "Saved. Check Allocation to see drift.", Toast.LENGTH_LONG).show()
    }
}
