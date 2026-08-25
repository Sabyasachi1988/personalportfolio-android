package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.widget.Button
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import androidx.viewpager2.widget.ViewPager2
import com.google.android.material.tabs.TabLayout
import com.google.android.material.tabs.TabLayoutMediator
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class AllocationActivity : AppCompatActivity() {

    private val gson = Gson()

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

    // Section 4: Tags (see store.Asset.Tags' doc comment) - actual only,
    // no target/drift (not asked for on this axis), and no picker: shows
    // the COMPLETE breakdown across every tag currently in use at once,
    // same convention as Equity Origin, rather than a single
    // caller-chosen tag - see finance.AllocationByTag's doc comment.
    private lateinit var summaryTags: TextView
    private lateinit var recyclerViewTags: RecyclerView
    private lateinit var donutChartTags: DonutChartView

    private var lastMarketCapSlices: List<DonutChartView.Slice> = emptyList()
    private var lastOriginSlices: List<DonutChartView.Slice> = emptyList()
    private var lastClassSlices: List<DonutChartView.Slice> = emptyList()
    private var lastTagSlices: List<DonutChartView.Slice> = emptyList()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_allocation)

        val inflater = LayoutInflater.from(this)
        val marketCapPage = inflater.inflate(R.layout.page_allocation_market_cap, null)
        val equityOriginPage = inflater.inflate(R.layout.page_allocation_equity_origin, null)
        val portfolioClassPage = inflater.inflate(R.layout.page_allocation_portfolio_class, null)
        val tagsPage = inflater.inflate(R.layout.page_allocation_tags, null)

        summary = marketCapPage.findViewById(R.id.allocationSummary)
        subCaption = marketCapPage.findViewById(R.id.allocationSubCaption)
        recyclerView = marketCapPage.findViewById(R.id.allocationRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this, RecyclerView.HORIZONTAL, false)
        donutChart = marketCapPage.findViewById(R.id.donutChart)
        donutChart.onSliceTapped = { _, _ -> openMarketCapExpanded() }
        marketCapPage.findViewById<TextView>(R.id.marketCapTapHint).setOnClickListener { openMarketCapExpanded() }
        marketCapPage.findViewById<Button>(R.id.editCompositionButton).setOnClickListener {
            startActivity(Intent(this, CapCompositionActivity::class.java))
        }
        marketCapPage.findViewById<Button>(R.id.setTargetButton).setOnClickListener {
            startActivity(Intent(this, TargetAllocationActivity::class.java))
        }

        summaryOrigin = equityOriginPage.findViewById(R.id.allocationSummaryOrigin)
        recyclerViewOrigin = equityOriginPage.findViewById(R.id.allocationRecyclerViewOrigin)
        recyclerViewOrigin.layoutManager = LinearLayoutManager(this, RecyclerView.HORIZONTAL, false)
        donutChartOrigin = equityOriginPage.findViewById(R.id.donutChartOrigin)
        donutChartOrigin.onSliceTapped = { _, _ -> openOriginExpanded() }
        equityOriginPage.findViewById<TextView>(R.id.equityOriginTapHint).setOnClickListener { openOriginExpanded() }
        equityOriginPage.findViewById<Button>(R.id.editOriginCompositionButton).setOnClickListener {
            startActivity(Intent(this, EquityOriginCompositionActivity::class.java))
        }
        // Deliberately no jump-to-Holdings on this section's slices even
        // inside the expanded dialog: HoldingsInSegment (Go) only
        // understands cap-size segment labels today. Wiring this the
        // same way would either silently show nothing or require
        // duplicating filter logic here in Kotlin - both worse than
        // just not offering it yet.

        summaryClass = portfolioClassPage.findViewById(R.id.allocationSummaryClass)
        subCaptionClass = portfolioClassPage.findViewById(R.id.allocationSubCaptionClass)
        recyclerViewClass = portfolioClassPage.findViewById(R.id.allocationRecyclerViewClass)
        recyclerViewClass.layoutManager = LinearLayoutManager(this, RecyclerView.HORIZONTAL, false)
        donutChartClass = portfolioClassPage.findViewById(R.id.donutChartClass)
        donutChartClass.onSliceTapped = { _, _ -> openClassExpanded() }
        portfolioClassPage.findViewById<TextView>(R.id.portfolioClassTapHint).setOnClickListener { openClassExpanded() }
        portfolioClassPage.findViewById<Button>(R.id.setClassTargetButton).setOnClickListener {
            startActivity(Intent(this, PortfolioClassTargetActivity::class.java))
        }

        summaryTags = tagsPage.findViewById(R.id.allocationSummaryTags)
        recyclerViewTags = tagsPage.findViewById(R.id.allocationRecyclerViewTags)
        recyclerViewTags.layoutManager = LinearLayoutManager(this, RecyclerView.HORIZONTAL, false)
        donutChartTags = tagsPage.findViewById(R.id.donutChartTags)
        donutChartTags.onSliceTapped = { _, _ -> openTagsExpanded() }
        tagsPage.findViewById<TextView>(R.id.tagsTapHint).setOnClickListener { openTagsExpanded() }
        tagsPage.findViewById<Button>(R.id.manageTagsFromAllocationButton).setOnClickListener {
            startActivity(Intent(this, TagsActivity::class.java))
        }
        // Deliberately no jump-to-Holdings on this section's slices, same
        // reasoning as Equity Origin's comment above - HoldingsInSegment
        // only understands cap-size segment labels today, not tags.

        val viewPager = findViewById<ViewPager2>(R.id.allocationViewPager)
        viewPager.adapter = AllocationPagerAdapter(listOf(marketCapPage, equityOriginPage, portfolioClassPage, tagsPage))
        // offscreenPageLimit keeps all 4 pages' Views alive simultaneously
        // rather than only the current + immediate neighbor - with just
        // 4 total pages this is cheap, and it means data bound to a page
        // that isn't currently visible (e.g. Portfolio Class while
        // showing Market Cap) is never lost or needs re-fetching on swipe.
        viewPager.offscreenPageLimit = 3

        val tabLayout = findViewById<TabLayout>(R.id.allocationTabLayout)
        val tabTitles = listOf("Market Cap", "Equity Origin", "Portfolio Class", "Tags")
        val tabIcons = listOf(R.drawable.ic_tab_pie, R.drawable.ic_tab_globe, R.drawable.ic_tab_layers, R.drawable.ic_tab_tag)
        TabLayoutMediator(tabLayout, viewPager) { tab, position ->
            tab.text = tabTitles[position]
            tab.setIcon(tabIcons[position])
        }.attach()

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
        loadAndShowTagsSection()
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

    private fun openTagsExpanded() {
        DonutExpansionDialog.show(this, "Tags", lastTagSlices)
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

        // Already in a fixed, canonical order from the Go side (see
        // finance.sortSlicesCanonically) - re-sorting by percent here would
        // bring back the exact "reorders itself on every reload" problem
        // that fix was for, just moved from map-iteration randomness to
        // percentages naturally drifting between refreshes.
        val sorted = slices
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

        // Already in a fixed, canonical order from the Go side (see
        // finance.sortSlicesCanonically) - re-sorting by percent here would
        // bring back the exact "reorders itself on every reload" problem
        // that fix was for, just moved from map-iteration randomness to
        // percentages naturally drifting between refreshes.
        val sorted = slices
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

        // Already in a fixed, canonical order from the Go side (see
        // finance.sortSlicesCanonically) - re-sorting by percent here would
        // bring back the exact "reorders itself on every reload" problem
        // that fix was for, just moved from map-iteration randomness to
        // percentages naturally drifting between refreshes.
        val sorted = slices
        recyclerViewClass.adapter = AllocationAdapter(sorted)
        val chartSlices = sorted.map {
            DonutChartView.Slice(it.label, it.percent.toFloat(), CapSegmentColors.forLabel(this, it.label))
        }
        donutChartClass.setSlices(chartSlices)
        lastClassSlices = chartSlices
    }

    private fun loadAndShowTagsSection() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val allocationJson = Bridge.computeAllocationByTag(portfolioJson, "")
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            summaryTags.text = "Could not read tags: ${e.message}"
            recyclerViewTags.adapter = AllocationAdapter(emptyList())
            donutChartTags.setSlices(emptyList())
            lastTagSlices = emptyList()
            return
        }

        summaryTags.text = if (slices.isEmpty()) {
            "No allocation data yet — this needs holdings with a current price. Refresh prices from the Holdings screen first."
        } else if (slices.size == 1 && slices[0].label == "Untagged") {
            "No funds tagged yet — use \"Manage Tags\" below to get started"
        } else {
            "Allocation by tag (funds with no tags fall under \"Untagged\")"
        }

        // Already in a fixed, canonical order from the Go side (see
        // finance.sortSlicesCanonically) - re-sorting by percent here would
        // bring back the exact "reorders itself on every reload" problem
        // that fix was for, just moved from map-iteration randomness to
        // percentages naturally drifting between refreshes.
        val sorted = slices
        recyclerViewTags.adapter = AllocationAdapter(sorted)
        val chartSlices = sorted.map {
            DonutChartView.Slice(it.label, it.percent.toFloat(), CapSegmentColors.forLabel(this, it.label))
        }
        donutChartTags.setSlices(chartSlices)
        lastTagSlices = chartSlices
    }
}
