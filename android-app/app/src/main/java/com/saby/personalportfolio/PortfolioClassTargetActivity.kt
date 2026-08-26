package com.saby.personalportfolio

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.ledger.bridge.Bridge

class PortfolioClassTargetActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var statusText: TextView
    private lateinit var equityInput: EditText
    private lateinit var debtInput: EditText
    private lateinit var commodityInput: EditText
    private lateinit var othersInput: EditText

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_portfolio_class_target)

        statusText = findViewById(R.id.classTargetStatusText)
        equityInput = findViewById(R.id.classTargetEquityInput)
        debtInput = findViewById(R.id.classTargetDebtInput)
        commodityInput = findViewById(R.id.classTargetCommodityInput)
        othersInput = findViewById(R.id.classTargetOthersInput)

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val snapshot: PortfolioClassTargetSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioClassTargetSnapshot::class.java)
        } catch (e: Exception) {
            PortfolioClassTargetSnapshot(null)
        }
        snapshot.portfolioClassTarget?.let { t ->
            equityInput.setText(t.equity.takeIf { it != 0.0 }?.toString() ?: "")
            debtInput.setText(t.debt.takeIf { it != 0.0 }?.toString() ?: "")
            commodityInput.setText(t.commodity.takeIf { it != 0.0 }?.toString() ?: "")
            othersInput.setText(t.others.takeIf { it != 0.0 }?.toString() ?: "")
        }

        findViewById<Button>(R.id.saveClassTargetButton).setOnClickListener { save() }
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun save() {
        val equity = equityInput.text.toString().toDoubleOrNull() ?: 0.0
        val debt = debtInput.text.toString().toDoubleOrNull() ?: 0.0
        val commodity = commodityInput.text.toString().toDoubleOrNull() ?: 0.0
        val others = othersInput.text.toString().toDoubleOrNull() ?: 0.0

        if (equity == 0.0 && debt == 0.0 && commodity == 0.0 && others == 0.0) {
            statusText.text = "Enter at least one nonzero percentage."
            return
        }

        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            statusText.text = "Failed to load portfolio: $currentPortfolioJson"
            return
        }

        val updatedJson = Bridge.setPortfolioClassTarget(currentPortfolioJson, equity, debt, commodity, others)
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
