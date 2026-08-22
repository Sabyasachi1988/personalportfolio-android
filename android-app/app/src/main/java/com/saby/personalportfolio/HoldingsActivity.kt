package com.saby.personalportfolio

import android.os.Bundle
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.util.Locale

class HoldingsActivity : AppCompatActivity() {

    private val gson = Gson()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_holdings)

        val summary = findViewById<TextView>(R.id.holdingsSummary)
        val recyclerView = findViewById<RecyclerView>(R.id.holdingsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

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
                "${holdings.size} holdings (no current prices available yet)"
            }
        }

        recyclerView.adapter = HoldingsAdapter(holdings)
    }
}
