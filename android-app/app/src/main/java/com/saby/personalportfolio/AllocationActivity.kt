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
    private lateinit var donutChart: DonutChartView
    private lateinit var donutLegend: DonutLegendView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_allocation)

        summary = findViewById(R.id.allocationSummary)
        recyclerView = findViewById(R.id.allocationRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        donutChart = findViewById(R.id.donutChart)
        donutLegend = findViewById(R.id.donutLegend)
        donutChart.onSliceTapped = { label, percent ->
            android.widget.Toast.makeText(
                this,
                String.format(java.util.Locale.getDefault(), "%s: %.1f%%", label, percent),
                android.widget.Toast.LENGTH_SHORT
            ).show()
        }

        findViewById<Button>(R.id.editCompositionButton).setOnClickListener {
            startActivity(Intent(this, CapCompositionActivity::class.java))
        }
        findViewById<Button>(R.id.setTargetButton).setOnClickListener {
            startActivity(Intent(this, TargetAllocationActivity::class.java))
        }

        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.ALLOCATION)
    }

    override fun onResume() {
        super.onResume()
        // Re-sync every time this screen resumes, not just once in
        // onCreate - a screen reused via CLEAR_TOP (coming back to it
        // from another tab) never re-runs onCreate, so without this
        // its nav bar could keep showing a stale selection.
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.ALLOCATION)
        // Recomputes every time this screen becomes visible, so coming
        // back from editing cap composition or the target both reflect
        // immediately.
        loadAndShowAllocation()
    }

    private fun loadAndShowAllocation() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val driftJson = Bridge.computeAllocationDrift(portfolioJson)
        val driftResult: AllocationDriftResult? = try {
            gson.fromJson(driftJson, AllocationDriftResult::class.java)
        } catch (e: Exception) {
            null
        }

        if (driftResult?.hasTarget == true && !driftResult.drift.isNullOrEmpty()) {
            summary.text = "Actual vs. target — bar fill = actual, red line = target"
            recyclerView.adapter = AllocationDriftAdapter(driftResult.drift)
            val chartSlices = driftResult.drift.map {
                DonutChartView.Slice(it.label, it.actual.toFloat(), CapSegmentColors.forLabel(this, it.label))
            }
            donutChart.setSlices(chartSlices)
            donutLegend.setSlices(chartSlices)
            return
        }

        // No target set yet (or it couldn't be read) - fall back to the
        // plain actual-only view.
        val allocationJson = Bridge.computeAllocationByMarketCap(portfolioJson)
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summary.text = "Could not read allocation: ${e.message}"
            recyclerView.adapter = AllocationAdapter(emptyList())
            donutChart.setSlices(emptyList())
            donutLegend.setSlices(emptyList())
            return
        }

        summary.text = if (slices.isEmpty()) {
            "No allocation data yet — this needs holdings with a current price. Refresh prices from the Holdings screen first."
        } else {
            "Allocation by market cap segment (set a target to see drift)"
        }

        val sorted = slices.sortedByDescending { it.percent }
        recyclerView.adapter = AllocationAdapter(sorted)
        val chartSlices = sorted.map {
            DonutChartView.Slice(it.label, it.percent.toFloat(), CapSegmentColors.forLabel(this, it.label))
        }
        donutChart.setSlices(chartSlices)
        donutLegend.setSlices(chartSlices)
    }
}
