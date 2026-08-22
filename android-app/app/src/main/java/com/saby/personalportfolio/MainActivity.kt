package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
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
    private lateinit var donutChart: DonutChartView
    private lateinit var donutLegend: DonutLegendView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        totalValue = findViewById(R.id.dashboardTotalValue)
        gainLine = findViewById(R.id.dashboardGainLine)
        xirrLine = findViewById(R.id.dashboardXirrLine)
        holdingsCountLine = findViewById(R.id.dashboardHoldingsCountLine)
        donutChart = findViewById(R.id.dashboardDonut)
        donutLegend = findViewById(R.id.dashboardDonutLegend)

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
        // Refresh every time the Dashboard becomes visible, so coming
        // back from Import or Settings always reflects the latest state.
        loadDashboard()
    }

    private fun loadDashboard() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val holdingsJson = Bridge.computeHoldingsForMember(portfolioJson, "")
        val holdingsType = object : TypeToken<List<Holding>>() {}.type
        val holdings: List<Holding> = try {
            gson.fromJson(holdingsJson, holdingsType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        if (holdings.isEmpty()) {
            totalValue.text = "No holdings yet"
            gainLine.text = "Tap + below to import your first CAS statement"
            xirrLine.text = ""
            holdingsCountLine.text = ""
            donutChart.setSlices(emptyList())
            donutLegend.setSlices(emptyList())
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
            totalValue.text = String.format(Locale.getDefault(), "₹%,.2f", totalCurrentValue)
            val gain = totalCurrentValue - totalInvested
            val gainPct = if (totalInvested != 0.0) (gain / totalInvested) * 100 else 0.0
            gainLine.text = String.format(
                Locale.getDefault(), "%s₹%,.2f (%.1f%%) overall",
                if (gain >= 0) "+" else "", gain, gainPct
            )
        } else {
            totalValue.text = "Prices not refreshed yet"
            gainLine.text = "Go to Holdings and tap Refresh Prices"
        }

        val xirrJson = Bridge.computePortfolioXIRR(portfolioJson)
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

        val allocationJson = Bridge.computeAllocationByMarketCap(portfolioJson)
        val sliceType = object : TypeToken<List<AllocationSlice>>() {}.type
        val slices: List<AllocationSlice> = try {
            gson.fromJson(allocationJson, sliceType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }
        val chartSlices = slices.map { DonutChartView.Slice(it.label, it.percent.toFloat()) }
        donutChart.setSlices(chartSlices)
        donutLegend.setSlices(chartSlices)
    }
}
