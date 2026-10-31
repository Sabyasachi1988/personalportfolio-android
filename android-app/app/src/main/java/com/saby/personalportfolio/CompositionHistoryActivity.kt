package com.saby.personalportfolio

import android.graphics.Typeface
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.SpannableStringBuilder
import android.text.Spanned
import android.text.style.ForegroundColorSpan
import android.text.style.StyleSpan
import android.widget.Button
import android.widget.PopupMenu
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Locale
import java.util.concurrent.Executors

/**
 * Month-by-month large / mid / small / cash mix (and sector mix) of the portfolio. All maths is in Go
 * (finance.ComputeCompositionHistory); this screen only draws it. Fund data comes from the public fund-data repo
 * and is kept current automatically (see FundDataSync); the button here just forces a check.
 */
class CompositionHistoryActivity : AppCompatActivity() {

    private val gson = Gson()
    private val bg = Executors.newSingleThreadExecutor()
    private val main = Handler(Looper.getMainLooper())

    private lateinit var chart: StackedAreaChartView
    private lateinit var readout: TextView
    private lateinit var coverage: TextView
    private lateinit var fundStatus: TextView
    private lateinit var memberButton: Button
    private lateinit var modeButton: Button
    private lateinit var refreshButton: Button

    private var memberIds: List<String> = listOf("")
    private var memberLabels: List<String> = listOf("All (family)")
    private var memberIndex = 0
    private var sectorMode = false
    private var points: List<CompositionPoint> = emptyList()
    private var series: List<StackedAreaChartView.Series> = emptyList()

    private val monthIn = SimpleDateFormat("yyyy-MM", Locale.US)
    private val monthOut = SimpleDateFormat("MMM yy", Locale.US)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_composition_history)
        chart = findViewById(R.id.compositionChart)
        readout = findViewById(R.id.compositionReadout)
        coverage = findViewById(R.id.compositionCoverage)
        fundStatus = findViewById(R.id.compositionFundStatus)
        memberButton = findViewById(R.id.compositionMemberButton)
        modeButton = findViewById(R.id.compositionModeButton)
        refreshButton = findViewById(R.id.compositionRefreshButton)

        chart.onScrub = { showReadout(it) }
        modeButton.setOnClickListener {
            sectorMode = !sectorMode
            modeButton.text = if (sectorMode) "Sectors" else "Market cap"
            rebuildSeries()
        }
        memberButton.setOnClickListener {
            val menu = PopupMenu(this, memberButton)
            memberLabels.forEachIndexed { i, l -> menu.menu.add(0, i, i, l) }
            menu.setOnMenuItemClickListener {
                memberIndex = it.itemId
                memberButton.text = memberLabels[memberIndex]
                load()
                true
            }
            menu.show()
        }
        refreshButton.setOnClickListener { forceRefresh() }
        loadMembers()
        load()
    }

    private fun loadMembers() {
        try {
            val json = PortfolioLoadCache.load(PortfolioStorage.filePath(this))
            val members: List<Member> = gson.fromJson(Bridge.listMembers(json), object : TypeToken<List<Member>>() {}.type) ?: emptyList()
            memberIds = listOf("") + members.map { it.id }
            memberLabels = listOf("All (family)") + members.map { it.name }
        } catch (e: Exception) {
            // keep the "All" default
        }
    }

    private fun isErr(s: String) = s.trimStart().startsWith("{\"error\"")

    private fun load() {
        readout.text = "Loading…"
        bg.execute {
            val json = PortfolioLoadCache.load(PortfolioStorage.filePath(this))
            val res = Bridge.computeCompositionHistory(json, memberIds.getOrElse(memberIndex) { "" })
            val pts: List<CompositionPoint> = if (isErr(res)) emptyList() else try {
                gson.fromJson(res, object : TypeToken<List<CompositionPoint>>() {}.type) ?: emptyList()
            } catch (e: Exception) { emptyList() }
            val status = fundStatusText(json)
            main.post {
                if (isErr(res)) Toast.makeText(this, res, Toast.LENGTH_LONG).show()
                points = pts
                fundStatus.text = status
                rebuildSeries()
            }
        }
    }

    private fun fundStatusText(portfolioJson: String): String = try {
        val snap = gson.fromJson(portfolioJson, AdditionalFundsSnapshot::class.java)
        val names = snap.assets.orEmpty().associateBy { it.id }
        snap.fundHistories.orEmpty().mapNotNull { h ->
            val a = names[h.assetId] ?: return@mapNotNull null
            val last = h.months?.lastOrNull()?.month ?: "no data"
            "${NicknameResolver.resolve(a.name, a.nickname)}: through $last. ${h.note.orEmpty()}"
        }.joinToString("\n")
    } catch (e: Exception) { "" }

    private fun label(m: String) = try { monthOut.format(monthIn.parse(m)!!) } catch (e: Exception) { m }

    private fun rebuildSeries() {
        if (points.isEmpty()) {
            chart.setData(emptyList(), emptyList())
            readout.text = "No holdings with published fund data yet. Open Additional Funds or wait for the automatic sync."
            coverage.text = ""
            return
        }
        val n = points.size
        val list = ArrayList<StackedAreaChartView.Series>()
        fun add(name: String, color: Int, f: (CompositionPoint) -> Double) {
            val v = DoubleArray(n) { f(points[it]) }
            if (v.any { it > 0.05 }) list.add(StackedAreaChartView.Series(name, color, v))
        }
        if (!sectorMode) {
            add("Large cap", 0xFF1E88E5.toInt()) { it.large }
            add("Mid cap", 0xFFFFA000.toInt()) { it.mid }
            add("Small cap", 0xFFD81B60.toInt()) { it.small }
            add("Cash", 0xFF00897B.toInt()) { it.cash }
            add("Debt", 0xFF6D4C41.toInt()) { it.debt }
            add("Foreign", 0xFF8E24AA.toInt()) { it.foreign }
            add("Other / unclassified", 0xFF78909C.toInt()) { it.other }
            add("No data", 0x55808080) { it.noData }
        } else {
            val avg = HashMap<String, Double>()
            points.forEach { p -> p.sectors.orEmpty().forEach { (k, v) -> avg[k] = (avg[k] ?: 0.0) + v / n } }
            val top = avg.entries.sortedByDescending { it.value }.take(9).map { it.key }
            val pal = ChartColors.palette(this)
            top.forEachIndexed { i, k -> add(k, pal[i % pal.size]) { it.sectors?.get(k) ?: 0.0 } }
            add("Other sectors", 0xFF90A4AE.toInt()) { p ->
                p.sectors.orEmpty().filterKeys { it !in top }.values.sum()
            }
            add("Non-equity / no data", 0x55808080) { p -> (100.0 - p.sectors.orEmpty().values.sum()).coerceAtLeast(0.0) }
        }
        series = list
        chart.setData(points.map { label(it.month) }, list)
        showReadout(points.size - 1)
        val last = points.last()
        coverage.text = "Covered by published fund data: ${"%.0f".format(100 - last.noData)}% of the latest value. " +
            "Months estimated or carried over from a neighbouring month: ${"%.0f".format(last.estimated)}% of the latest value."
    }

    private fun showReadout(idx: Int) {
        if (idx !in points.indices) return
        val p = points[idx]
        val sb = SpannableStringBuilder()
        val head = "${label(p.month)}  ·  ${IndianCurrencyFormatter.format(p.value)}\n"
        sb.append(head)
        sb.setSpan(StyleSpan(Typeface.BOLD), 0, head.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
        series.map { it to it.values[idx] }.filter { it.second >= 0.05 }.sortedByDescending { it.second }.forEach { (s, v) ->
            val start = sb.length
            sb.append("● ")
            sb.setSpan(ForegroundColorSpan(s.color or 0xFF000000.toInt()), start, sb.length, Spanned.SPAN_EXCLUSIVE_EXCLUSIVE)
            sb.append("${s.name}  ${"%.1f".format(v)}%\n")
        }
        readout.text = sb
    }

    private fun forceRefresh() {
        refreshButton.isEnabled = false
        refreshButton.text = "Checking…"
        bg.execute {
            val path = PortfolioStorage.filePath(this)
            val json = PortfolioLoadCache.load(path)
            val snap = try { gson.fromJson(json, AdditionalFundsSnapshot::class.java) } catch (e: Exception) { null }
            var failed = 0
            snap?.assets.orEmpty().filter { it.isin.isNotBlank() && it.type == "MutualFund" }.forEach {
                if (isErr(Bridge.pullFundAtPath(path, it.id, false))) failed++
            }
            main.post {
                refreshButton.isEnabled = true
                refreshButton.text = "Refresh fund data now"
                if (failed > 0) Toast.makeText(this, "$failed fund(s) could not be refreshed", Toast.LENGTH_LONG).show()
                load()
            }
        }
    }
}
