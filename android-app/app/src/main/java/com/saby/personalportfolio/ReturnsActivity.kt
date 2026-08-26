package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.view.View
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

class ReturnsActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var recyclerView: RecyclerView
    private lateinit var emptyState: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_returns)

        recyclerView = findViewById(R.id.returnsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        emptyState = findViewById(R.id.returnsEmptyState)

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
            emptyState.visibility = View.VISIBLE
            emptyState.text = "Could not load returns: $resultJson"
            recyclerView.adapter = ReturnsAdapter(emptyList()) {}
            return
        }

        val rowType = object : TypeToken<List<ReturnsTableRow>>() {}.type
        val rows: List<ReturnsTableRow> = try {
            gson.fromJson(resultJson, rowType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        if (rows.isEmpty()) {
            emptyState.visibility = View.VISIBLE
            emptyState.text = "No historical data yet. Mutual funds need their NAV history fetched " +
                "(Settings → a fund's Update History), and benchmarks need adding (tap the gear icon above)."
        } else {
            emptyState.visibility = View.GONE
        }

        recyclerView.adapter = ReturnsAdapter(rows) { row ->
            val intent = Intent(this, ReturnsDetailActivity::class.java)
            intent.putExtra(ReturnsDetailActivity.EXTRA_SERIES_ID, row.seriesId)
            intent.putExtra(ReturnsDetailActivity.EXTRA_NAME, row.name)
            startActivity(intent)
        }
    }
}
