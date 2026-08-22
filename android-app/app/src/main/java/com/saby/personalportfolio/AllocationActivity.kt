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

    // Section 1: Market cap (Large/Mid/Small/Cash) - unchanged from before.
    private lateinit var summary: TextView
    private lateinit var recyclerView: RecyclerView
    private lateinit var donutChart: DonutChartView
    private lateinit var donutLegend: DonutLegendView

    // Section 2: Equity origin (Indian vs. International) - actual only,
    // no target/drift (not asked for on this axis).
    private lateinit var summaryOrigin: TextView
    private lateinit var recyclerViewOrigin: RecyclerView
    private lateinit var donutChartOrigin: DonutChartView
    private lateinit var donutLegendOrigin: DonutLegendView

    // Section 3: Portfolio class (Equity/Debt/Commodity/Others) - full
    // target/drift, same treatment as the market-cap section.
    private lateinit var summaryClass: TextView
    private lateinit var recyclerViewClass: RecyclerView
    private lateinit var donutChartClass: DonutChartView
    private lateinit var donutLegendClass: DonutLegendView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_allocation)

        summary = findViewById(R.id.allocationSummary)
        recyclerView = findViewById(R.id.allocationRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        donutChart = findViewById(R.id.donutChart)
        donutLegend = findViewById(R.id.donutLegend)
        donutChart.onSliceTapped = { label, _ ->
            val intent = Intent(this, HoldingsActivity::class.java)
            intent.putExtra(HoldingsActivity.EXTRA_SEGMENT_FILTER, label)
            intent.addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP)
            startActivity(intent)
        }

        summaryOrigin = findViewById(R.id.allocationSummaryOrigin)
        recyclerViewOrigin = findViewById(R.id.allocationRecyclerViewOrigin)
        recyclerViewOrigin.layoutManager = LinearLayoutManager(this)
        donutChartOrigin = findViewById(R.id.donutChartOrigin)
        donutLegendOrigin = findViewById(R.id.donutLegendOrigin)
        // Deliberately not wired to tap-to-filter yet: HoldingsInSegment
        // (Go) only understands cap-size segment labels today. Wiring
        // this donut the same way would either silently show nothing or
        // require duplicating filter logic here in Kotlin - both worse
        // than just not offering the tap yet.

        summaryClass = findViewById(R.id.allocationSummaryClass)
        recyclerViewClass = findViewById(R.id.allocationRecyclerViewClass)
        recyclerViewClass.layoutManager = LinearLayoutManager(this)
        donutChartClass = findViewById(R.id.donutChartClass)
        donutLegendClass = findViewById(R.id.donutLegendClass)

        findViewById<Button>(R.id.editCompositionButton).setOnClickListener {
            startActivity(Intent(this, CapCompositionActivity::class.java))
        }
        findViewById<Button>(R.id.setTargetButton).setOnClickListener {
            startActivity(Intent(this, TargetAllocationActivity::class.java))
        }
        findViewById<Button>(R.id.editOriginCompositionButton).setOnClickListener {
            startActivity(Intent(this, EquityOriginCompositionActivity::class.java))
        }
        findViewById<Button>(R.id.setClassTargetButton).setOnClickListener {
            startActivity(Intent(this, PortfolioClassTargetActivity::class.java))
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
        // back from editing any composition or any target all reflect
        // immediately.
        loadAndShowMarketCapSection()
        loadAndShowEquityOriginSection()
        loadAndShowPortfolioClassSection()
    }

    private fun loadAndShowMarketCapSection() {
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

    private fun loadAndShowEquityOriginSection() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val allocationJson = Bridge.computeAllocationByEquityOrigin(portfolioJson)
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summaryOrigin.text = "Could not read equity origin: ${e.message}"
            recyclerViewOrigin.adapter = AllocationAdapter(emptyList())
            donutChartOrigin.setSlices(emptyList())
            donutLegendOrigin.setSlices(emptyList())
            return
        }

        summaryOrigin.text = if (slices.isEmpty()) {
            "No equity holdings with a current price yet."
        } else {
            "Of your equity holdings, Indian vs. International (defaults to Indian until entered)"
        }

        val sorted = slices.sortedByDescending { it.percent }
        recyclerViewOrigin.adapter = AllocationAdapter(sorted)
        val chartSlices = sorted.map {
            DonutChartView.Slice(it.label, it.percent.toFloat(), CapSegmentColors.forLabel(this, it.label))
        }
        donutChartOrigin.setSlices(chartSlices)
        donutLegendOrigin.setSlices(chartSlices)
    }

    private fun loadAndShowPortfolioClassSection() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val driftJson = Bridge.computePortfolioClassDrift(portfolioJson)
        val driftResult: PortfolioClassDriftResult? = try {
            gson.fromJson(driftJson, PortfolioClassDriftResult::class.java)
        } catch (e: Exception) {
            null
        }

        if (driftResult?.hasTarget == true && !driftResult.drift.isNullOrEmpty()) {
            summaryClass.text = "Actual vs. target — bar fill = actual, red line = target"
            recyclerViewClass.adapter = AllocationDriftAdapter(driftResult.drift)
            val chartSlices = driftResult.drift.map {
                DonutChartView.Slice(it.label, it.actual.toFloat(), CapSegmentColors.forLabel(this, it.label))
            }
            donutChartClass.setSlices(chartSlices)
            donutLegendClass.setSlices(chartSlices)
            return
        }

        val allocationJson = Bridge.computeAllocationByPortfolioClass(portfolioJson)
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summaryClass.text = "Could not read portfolio class: ${e.message}"
            recyclerViewClass.adapter = AllocationAdapter(emptyList())
            donutChartClass.setSlices(emptyList())
            donutLegendClass.setSlices(emptyList())
            return
        }

        summaryClass.text = if (slices.isEmpty()) {
            "No holdings with a current price yet."
        } else {
            "Whole portfolio by class (set a target to see drift)"
        }

        val sorted = slices.sortedByDescending { it.percent }
        recyclerViewClass.adapter = AllocationAdapter(sorted)
        val chartSlices = sorted.map {
            DonutChartView.Slice(it.label, it.percent.toFloat(), CapSegmentColors.forLabel(this, it.label))
        }
        donutChartClass.setSlices(chartSlices)
        donutLegendClass.setSlices(chartSlices)
    }
}
