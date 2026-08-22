package com.saby.personalportfolio

import android.os.Bundle
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class AllocationActivity : AppCompatActivity() {

    private val gson = Gson()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_allocation)

        val summary = findViewById<TextView>(R.id.allocationSummary)
        val recyclerView = findViewById<RecyclerView>(R.id.allocationRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val allocationJson = Bridge.computeAllocationByMarketCap(portfolioJson)

        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summary.text = "Could not read allocation: ${e.message}"
            emptyList()
        }

        if (slices.isEmpty()) {
            summary.text = "No allocation data yet — this needs holdings with a current price. Refresh prices from the Holdings screen first."
        } else {
            summary.text = "Allocation by market cap segment"
        }

        // Largest slice first, so the breakdown reads naturally.
        val sorted = slices.sortedByDescending { it.percent }
        recyclerView.adapter = AllocationAdapter(sorted)
    }
}
