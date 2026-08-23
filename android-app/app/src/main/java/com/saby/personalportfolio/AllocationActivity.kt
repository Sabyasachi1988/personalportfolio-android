package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.android.material.tabs.TabLayout
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class AllocationActivity : AppCompatActivity() {

    private val gson = Gson()

    private lateinit var tabLayout: TabLayout
    private lateinit var marketCapSection: View
    private lateinit var equityOriginSection: View
    private lateinit var portfolioClassSection: View

    // Section 1: Market cap (Large/Mid/Small/Cash)
    private lateinit var summary: TextView
    private lateinit var subCaption: TextView
    private lateinit var recyclerView: RecyclerView
    private lateinit var donutChart: DonutChartView

    // Section 2: Equity origin (Indian vs. International) - actual only,
    // no target/drift (not asked for on this axis).
    private lateinit var summaryOrigin: TextView
    private lateinit var recyclerViewOrigin: RecyclerView
    private lateinit var donutChartOrigin: DonutChartView

    // Section 3: Portfolio class (Equity/Debt/Commodity/Others) - full
    // target/drift, same treatment as the market-cap section.
    private lateinit var summaryClass: TextView
    private lateinit var subCaptionClass: TextView
    private lateinit var recyclerViewClass: RecyclerView
    private lateinit var donutChartClass: DonutChartView

    private var lastMarketCapSlices: List<DonutChartView.Slice> = emptyList()
    private var lastOriginSlices: List<DonutChartView.Slice> = emptyList()
    private var lastClassSlices: List<DonutChartView.Slice> = emptyList()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_allocation)

        tabLayout = findViewById(R.id.allocationTabLayout)
        marketCapSection = findViewById(R.id.marketCapSection)
        equityOriginSection = findViewById(R.id.equityOriginSection)
        portfolioClassSection = findViewById(R.id.portfolioClassSection)
        tabLayout.addTab(tabLayout.newTab().setText("Market Cap"))
        tabLayout.addTab(tabLayout.newTab().setText("Equity Origin"))
        tabLayout.addTab(tabLayout.newTab().setText("Portfolio Class"))
        tabLayout.addOnTabSelectedListener(object : TabLayout.OnTabSelectedListener {
            override fun onTabSelected(tab: TabLayout.Tab) = showSection(tab.position)
            override fun onTabUnselected(tab: TabLayout.Tab) {}
            override fun onTabReselected(tab: TabLayout.Tab) {}
        })

        summary = findViewById(R.id.allocationSummary)
        subCaption = findViewById(R.id.allocationSubCaption)
        recyclerView = findViewById(R.id.allocationRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this, RecyclerView.HORIZONTAL, false)
        donutChart = findViewById(R.id.donutChart)
        donutChart.onSliceTapped = { _, _ -> openMarketCapExpanded() }
        findViewById<TextView>(R.id.marketCapTapHint).setOnClickListener { openMarketCapExpanded() }

        summaryOrigin = findViewById(R.id.allocationSummaryOrigin)
        recyclerViewOrigin = findViewById(R.id.allocationRecyclerViewOrigin)
        recyclerViewOrigin.layoutManager = LinearLayoutManager(this, RecyclerView.HORIZONTAL, false)
        donutChartOrigin = findViewById(R.id.donutChartOrigin)
        donutChartOrigin.onSliceTapped = { _, _ -> openOriginExpanded() }
        findViewById<TextView>(R.id.equityOriginTapHint).setOnClickListener { openOriginExpanded() }
        // Deliberately no jump-to-Holdings on this section's slices even
        // inside the expanded dialog: HoldingsInSegment (Go) only
        // understands cap-size segment labels today. Wiring this the
        // same way would either silently show nothing or require
        // duplicating filter logic here in Kotlin - both worse than
        // just not offering it yet.

        summaryClass = findViewById(R.id.allocationSummaryClass)
        subCaptionClass = findViewById(R.id.allocationSubCaptionClass)
        recyclerViewClass = findViewById(R.id.allocationRecyclerViewClass)
        recyclerViewClass.layoutManager = LinearLayoutManager(this, RecyclerView.HORIZONTAL, false)
        donutChartClass = findViewById(R.id.donutChartClass)
        donutChartClass.onSliceTapped = { _, _ -> openClassExpanded() }
        findViewById<TextView>(R.id.portfolioClassTapHint).setOnClickListener { openClassExpanded() }

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

    private fun showSection(tabPosition: Int) {
        marketCapSection.visibility = if (tabPosition == 0) View.VISIBLE else View.GONE
        equityOriginSection.visibility = if (tabPosition == 1) View.VISIBLE else View.GONE
        portfolioClassSection.visibility = if (tabPosition == 2) View.VISIBLE else View.GONE
    }

    private fun openMarketCapExpanded() {
        DonutExpansionDialog.show(
            this, "Market Cap", lastMarketCapSlices,
            navigationHint = "Tap a segment to view its holdings"
        ) { label -> navigateToHoldingsSegment(label) }
    }

    private fun openOriginExpanded() {
        DonutExpansionDialog.show(this, "Equity Origin", lastOriginSlices)
    }

    private fun openClassExpanded() {
        DonutExpansionDialog.show(
            this, "Portfolio Class", lastClassSlices,
            navigationHint = "Tap a segment to view its holdings"
        ) { label -> navigateToHoldingsSegment(label) }
    }

    private fun navigateToHoldingsSegment(label: String) {
        val intent = Intent(this, HoldingsActivity::class.java)
        intent.putExtra(HoldingsActivity.EXTRA_SEGMENT_FILTER, label)
        intent.addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP)
        startActivity(intent)
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
            summary.text = "Actual vs. target"
            subCaption.visibility = View.VISIBLE
            subCaption.text = "Bar = actual · red line = target"
            recyclerView.adapter = AllocationDriftAdapter(driftResult.drift)
            val chartSlices = driftResult.drift.map {
                DonutChartView.Slice(it.label, it.actual.toFloat(), CapSegmentColors.forLabel(this, it.label))
            }
            donutChart.setSlices(chartSlices)
            lastMarketCapSlices = chartSlices
            return
        }

        // No target set yet (or it couldn't be read) - fall back to the
        // plain actual-only view.
        val allocationJson = Bridge.computeAllocationByMarketCap(portfolioJson, "")
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summary.text = "Could not read allocation: ${e.message}"
            subCaption.visibility = View.GONE
            recyclerView.adapter = AllocationAdapter(emptyList())
            donutChart.setSlices(emptyList())
            lastMarketCapSlices = emptyList()
            return
        }

        subCaption.visibility = View.GONE
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
        lastMarketCapSlices = chartSlices
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
            lastOriginSlices = emptyList()
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
        lastOriginSlices = chartSlices
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
            summaryClass.text = "Actual vs. target"
            subCaptionClass.visibility = View.VISIBLE
            subCaptionClass.text = "Bar = actual · red line = target"
            recyclerViewClass.adapter = AllocationDriftAdapter(driftResult.drift)
            val chartSlices = driftResult.drift.map {
                DonutChartView.Slice(it.label, it.actual.toFloat(), CapSegmentColors.forLabel(this, it.label))
            }
            donutChartClass.setSlices(chartSlices)
            lastClassSlices = chartSlices
            return
        }

        val allocationJson = Bridge.computeAllocationByPortfolioClass(portfolioJson)
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summaryClass.text = "Could not read portfolio class: ${e.message}"
            subCaptionClass.visibility = View.GONE
            recyclerViewClass.adapter = AllocationAdapter(emptyList())
            donutChartClass.setSlices(emptyList())
            lastClassSlices = emptyList()
            return
        }

        subCaptionClass.visibility = View.GONE
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
        lastClassSlices = chartSlices
    }
}
