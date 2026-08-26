package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.widget.FrameLayout
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.widget.PopupMenu
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class ReturnsActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var pickerTab: TextView
    private lateinit var cardContainer: FrameLayout
    private lateinit var emptyState: TextView

    private var rows: List<ReturnsTableRow> = emptyList()
    private var selectedSeriesId: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_returns)

        pickerTab = findViewById(R.id.returnsPickerTab)
        cardContainer = findViewById(R.id.returnsCardContainer)
        emptyState = findViewById(R.id.returnsEmptyState)

        pickerTab.setOnClickListener { showPicker() }

        findViewById<View>(R.id.returnsManageBenchmarksButton).setOnClickListener {
            startActivity(Intent(this, BenchmarksActivity::class.java))
        }
    }

    override fun onResume() {
        super.onResume()
        loadReturnsTable()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadReturnsTable() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val resultJson = Bridge.computeReturnsTable(portfolioJson)
        if (isBridgeError(resultJson)) {
            showEmpty("Could not load returns: $resultJson")
            return
        }

        val rowType = object : TypeToken<List<ReturnsTableRow>>() {}.type
        rows = try {
            gson.fromJson(resultJson, rowType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        if (rows.isEmpty()) {
            showEmpty(
                "No historical data yet. Mutual funds need their NAV history fetched " +
                    "(Settings → a fund's Update History), and benchmarks need adding (tap the gear icon above)."
            )
            return
        }

        emptyState.visibility = View.GONE
        pickerTab.visibility = View.VISIBLE
        cardContainer.visibility = View.VISIBLE

        // Keep the existing selection across a reload (e.g. returning
        // from the drill-down chart) if it's still present; otherwise
        // default to the first fund (funds are what you hold, so more
        // likely to be what you want to see first) or, failing that,
        // the first row of any kind.
        val stillPresent = rows.any { it.seriesId == selectedSeriesId }
        if (!stillPresent) {
            selectedSeriesId = rows.firstOrNull { !it.isBenchmark }?.seriesId ?: rows.first().seriesId
        }
        showSelectedCard()
    }

    private fun showEmpty(message: String) {
        pickerTab.visibility = View.GONE
        cardContainer.visibility = View.GONE
        emptyState.visibility = View.VISIBLE
        emptyState.text = message
    }

    private fun showSelectedCard() {
        val row = rows.firstOrNull { it.seriesId == selectedSeriesId } ?: return
        pickerTab.text = FundNameFormatter.shorten(row.name).ifBlank { row.name }

        val cardView = LayoutInflater.from(this).inflate(R.layout.item_returns_row, cardContainer, false)
        ReturnsCardBinder.bind(cardView, row)
        cardView.setOnClickListener {
            val intent = Intent(this, ReturnsDetailActivity::class.java)
            intent.putExtra(ReturnsDetailActivity.EXTRA_SERIES_ID, row.seriesId)
            intent.putExtra(ReturnsDetailActivity.EXTRA_NAME, row.name)
            startActivity(intent)
        }
        cardContainer.removeAllViews()
        cardContainer.addView(cardView)
    }

    /**
     * Two-stage picker: Fund vs Index category first, then the specific
     * item within it - same reasoning as Progression's picker (see its
     * own doc comment): with 15+ funds and benchmarks combined, one flat
     * list would be exactly the unmanageable-length problem this screen
     * was just redesigned to avoid.
     */
    private fun showPicker() {
        val funds = rows.filter { !it.isBenchmark }
        val benchmarks = rows.filter { it.isBenchmark }

        val popup = PopupMenu(this, pickerTab)
        val fundCategoryId = 0
        val benchmarkCategoryId = 1
        if (funds.isNotEmpty()) {
            popup.menu.add(0, fundCategoryId, fundCategoryId, "Fund (${funds.size}) ▸")
        }
        if (benchmarks.isNotEmpty()) {
            popup.menu.add(0, benchmarkCategoryId, benchmarkCategoryId, "Index (${benchmarks.size}) ▸")
        }
        popup.setOnMenuItemClickListener { item ->
            when (item.itemId) {
                fundCategoryId -> showCategoryPicker(funds)
                benchmarkCategoryId -> showCategoryPicker(benchmarks)
            }
            true
        }
        popup.show()
    }

    private fun showCategoryPicker(items: List<ReturnsTableRow>) {
        val popup = PopupMenu(this, pickerTab)
        items.forEachIndexed { index, row ->
            popup.menu.add(0, index, index, FundNameFormatter.shorten(row.name).ifBlank { row.name })
        }
        popup.setOnMenuItemClickListener { item ->
            selectedSeriesId = items[item.itemId].seriesId
            showSelectedCard()
            true
        }
        popup.show()
    }
}
