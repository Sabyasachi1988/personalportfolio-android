package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.widget.Button
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class AllocationActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var summary: TextView
    private lateinit var recyclerView: RecyclerView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_allocation)

        summary = findViewById(R.id.allocationSummary)
        recyclerView = findViewById(R.id.allocationRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)

        findViewById<Button>(R.id.editCompositionButton).setOnClickListener {
            startActivity(Intent(this, CapCompositionActivity::class.java))
        }
    }

    override fun onResume() {
        super.onResume()
        // Recomputes every time this screen becomes visible - in
        // particular, right after coming back from CapCompositionActivity,
        // so an edited composition shows up immediately without needing
        // to force-close and reopen the app.
        loadAndShowAllocation()
    }

    private fun loadAndShowAllocation() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val allocationJson = Bridge.computeAllocationByMarketCap(portfolioJson)

        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summary.text = "Could not read allocation: ${e.message}"
            recyclerView.adapter = AllocationAdapter(emptyList())
            return
        }

        summary.text = if (slices.isEmpty()) {
            "No allocation data yet — this needs holdings with a current price. Refresh prices from the Holdings screen first."
        } else {
            "Allocation by market cap segment"
        }

        // Largest slice first, so the breakdown reads naturally.
        val sorted = slices.sortedByDescending { it.percent }
        recyclerView.adapter = AllocationAdapter(sorted)
    }
}
