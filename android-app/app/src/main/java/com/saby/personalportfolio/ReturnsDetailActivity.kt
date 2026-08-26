package com.saby.personalportfolio

import android.os.Bundle
import android.view.View
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Locale

class ReturnsDetailActivity : AppCompatActivity() {

    companion object {
        const val EXTRA_SERIES_ID = "series_id"
        const val EXTRA_NAME = "name"
    }

    private val gson = Gson()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_returns_detail)

        val seriesId = intent.getStringExtra(EXTRA_SERIES_ID) ?: return
        val name = intent.getStringExtra(EXTRA_NAME) ?: seriesId

        val nameView = findViewById<TextView>(R.id.returnsDetailName)
        val scrubbedView = findViewById<TextView>(R.id.returnsDetailScrubbed)
        val chart = findViewById<PriceHistoryChartView>(R.id.returnsDetailChart)
        val emptyState = findViewById<TextView>(R.id.returnsDetailEmptyState)

        nameView.text = name

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val resultJson = Bridge.computePriceHistory(portfolioJson, seriesId)
        val pointType = object : TypeToken<List<PricePoint>>() {}.type
        val points: List<PricePoint> = try {
            gson.fromJson(resultJson, pointType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        if (points.size < 2) {
            chart.visibility = View.GONE
            emptyState.visibility = View.VISIBLE
            emptyState.text = "Not enough history yet to chart. Use Refresh (funds: Update History; " +
                "benchmarks: Manage Benchmarks) to fetch it."
            return
        }

        val displayFormat = SimpleDateFormat("d MMM yyyy", Locale.getDefault())
        val storedFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
        chart.onPointScrubbed = { point ->
            val displayDate = try {
                displayFormat.format(storedFormat.parse(point.date) ?: point.date)
            } catch (e: Exception) {
                point.date
            }
            scrubbedView.text = "$displayDate: ${IndianCurrencyFormatter.format(point.price, decimals = 2)}"
        }
        chart.setPoints(points)
    }
}
