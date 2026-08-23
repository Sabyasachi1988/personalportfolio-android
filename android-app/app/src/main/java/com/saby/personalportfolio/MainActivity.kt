package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Spinner
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import com.google.android.material.floatingactionbutton.FloatingActionButton
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.util.Locale

class MainActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var totalValue: TextView
    private lateinit var gainLine: TextView
    private lateinit var xirrLine: TextView
    private lateinit var holdingsCountLine: TextView
    private lateinit var donutMarketCap: DonutChartView
    private lateinit var donutLegendMarketCap: DonutLegendView
    private lateinit var donutOrigin: DonutChartView
    private lateinit var donutLegendOrigin: DonutLegendView
    private lateinit var donutClass: DonutChartView
    private lateinit var donutLegendClass: DonutLegendView
    private lateinit var memberSpinner: Spinner
    private var donutToast: android.widget.Toast? = null

    // Index 0 is always "All (family)" (empty memberID); indices 1.. map
    // 1:1 with memberIds - same convention as HoldingsActivity's spinner.
    private var memberIds: List<String> = emptyList()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        totalValue = findViewById(R.id.statsCardValue)
        gainLine = findViewById(R.id.statsCardGain)
        xirrLine = findViewById(R.id.statsCardXirr)
        holdingsCountLine = findViewById(R.id.statsCardCount)
        // Dashboard's card shows count but not the Invested line (kept
        // terse here since Holdings already covers the detailed
        // breakdown) - Count is otherwise hidden by default in the
        // shared card layout.
        holdingsCountLine.visibility = View.VISIBLE
        donutMarketCap = findViewById(R.id.dashboardDonutMarketCap)
        donutLegendMarketCap = findViewById(R.id.dashboardDonutLegendMarketCap)
        donutOrigin = findViewById(R.id.dashboardDonutOrigin)
        donutLegendOrigin = findViewById(R.id.dashboardDonutLegendOrigin)
        donutClass = findViewById(R.id.dashboardDonutClass)
        donutLegendClass = findViewById(R.id.dashboardDonutLegendClass)
        memberSpinner = findViewById(R.id.dashboardMemberSpinner)
        donutMarketCap.onSliceTapped = { label, percent -> showSliceToast(label, percent) }
        donutOrigin.onSliceTapped = { label, percent -> showSliceToast(label, percent) }
        donutClass.onSliceTapped = { label, percent -> showSliceToast(label, percent) }

        memberSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                loadDashboard()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }

        findViewById<FloatingActionButton>(R.id.importFab).setOnClickListener {
            startActivity(Intent(this, ImportActivity::class.java))
        }
        findViewById<android.widget.Button>(R.id.settingsButton).setOnClickListener {
            startActivity(Intent(this, SettingsActivity::class.java))
        }

        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.DASHBOARD)
    }

    override fun onResume() {
        super.onResume()
        // Re-sync every time this screen resumes, not just once in
        // onCreate - a screen reused via CLEAR_TOP (coming back to it
        // from another tab) never re-runs onCreate, so without this
        // its nav bar could keep showing a stale selection.
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.DASHBOARD)
        // Refresh the member list every time the Dashboard becomes
        // visible (in particular after a new CAS import may have added a
        // new member), then show the dashboard for whichever member is
        // selected - same pattern as HoldingsActivity's loadMemberSpinner.
        loadMemberSpinner()
    }

    private fun showSliceToast(label: String, percent: Float) {
        donutToast?.cancel()
        val toast = android.widget.Toast.makeText(
            this,
            String.format(Locale.getDefault(), "%s: %.1f%%", label, percent),
            android.widget.Toast.LENGTH_SHORT
        )
        donutToast = toast
        toast.show()
    }

    private fun loadMemberSpinner() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val membersJson = Bridge.listMembers(portfolioJson)

        val memberType = object : TypeToken<List<Member>>() {}.type
        val members: List<Member> = try {
            gson.fromJson(membersJson, memberType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        val previousSelection = memberSpinner.selectedItemPosition.takeIf { it >= 0 } ?: 0

        memberIds = listOf("") + members.map { it.id }
        val labels = listOf("All (family)") + members.map { it.name }
        memberSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, labels)

        // Keep the same selection if it's still valid, rather than always
        // resetting to "All" on every refresh.
        memberSpinner.setSelection(previousSelection.coerceAtMost(memberIds.size - 1))

        loadDashboard()
    }

    private fun loadDashboard() {
        val selectedIndex = memberSpinner.selectedItemPosition
        val memberId = memberIds.getOrElse(selectedIndex) { "" }

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val holdingsJson = Bridge.computeHoldingsForMember(portfolioJson, memberId)
        val holdingsType = object : TypeToken<List<Holding>>() {}.type
        val holdings: List<Holding> = try {
            gson.fromJson(holdingsJson, holdingsType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        if (holdings.isEmpty()) {
            totalValue.text = "No holdings yet"
            gainLine.text = "Tap + below to import your first CAS statement"
            gainLine.setTextColor(androidx.core.content.ContextCompat.getColor(this, R.color.colorNeutral))
            xirrLine.text = ""
            holdingsCountLine.text = ""
            donutMarketCap.setSlices(emptyList())
            donutLegendMarketCap.setSlices(emptyList())
            donutOrigin.setSlices(emptyList())
            donutLegendOrigin.setSlices(emptyList())
            donutClass.setSlices(emptyList())
            donutLegendClass.setSlices(emptyList())
            return
        }

        var totalInvested = 0.0
        var totalCurrentValue = 0.0
        var anyPriced = false
        for (h in holdings) {
            if (h.hasPrice) {
                totalInvested += h.netInvested
                totalCurrentValue += h.currentValue
                anyPriced = true
            }
        }

        if (anyPriced) {
            totalValue.text = IndianCurrencyFormatter.format(totalCurrentValue)
            val gain = totalCurrentValue - totalInvested
            val gainPct = if (totalInvested != 0.0) (gain / totalInvested) * 100 else 0.0
            gainLine.text = String.format(
                Locale.getDefault(), "%s (%.1f%%) overall",
                IndianCurrencyFormatter.formatSigned(gain), gainPct
            )
            gainLine.setTextColor(
                androidx.core.content.ContextCompat.getColor(
                    this, if (gain >= 0) R.color.colorGain else R.color.colorLoss
                )
            )
        } else {
            totalValue.text = "Prices not refreshed yet"
            gainLine.text = "Go to Holdings and tap Refresh Prices"
            gainLine.setTextColor(androidx.core.content.ContextCompat.getColor(this, R.color.colorNeutral))
        }

        val xirrJson = Bridge.computePortfolioXIRR(portfolioJson, memberId)
        val xirrResult = try {
            gson.fromJson(xirrJson, PortfolioXirrResult::class.java)
        } catch (e: Exception) {
            null
        }
        xirrLine.text = if (xirrResult?.hasXIRR == true) {
            String.format(Locale.getDefault(), "Portfolio XIRR: %.2f%%", xirrResult.xirr)
        } else {
            ""
        }

        holdingsCountLine.text = "${holdings.size} holding(s)"

        val marketCapJson = Bridge.computeAllocationByMarketCap(portfolioJson, memberId)
        setDonut(donutMarketCap, donutLegendMarketCap, marketCapJson)

        val originJson = Bridge.computeAllocationByEquityOrigin(portfolioJson)
        setDonut(donutOrigin, donutLegendOrigin, originJson)

        val classJson = Bridge.computeAllocationByPortfolioClass(portfolioJson)
        setDonut(donutClass, donutLegendClass, classJson)
    }

    private fun setDonut(chart: DonutChartView, legend: DonutLegendView, allocationJson: String) {
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }
        val chartSlices = slices.map {
            DonutChartView.Slice(it.label, it.percent.toFloat(), CapSegmentColors.forLabel(this, it.label))
        }
        chart.setSlices(chartSlices)
        legend.setSlices(chartSlices)
    }
}
