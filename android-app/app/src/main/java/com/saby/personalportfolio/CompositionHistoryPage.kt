package com.saby.personalportfolio

import android.content.res.ColorStateList
import android.graphics.drawable.GradientDrawable
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.LinearLayout
import android.widget.PopupMenu
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.view.ContextThemeWrapper
import androidx.core.content.ContextCompat
import com.google.android.material.button.MaterialButtonToggleGroup
import com.google.android.material.card.MaterialCardView
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Locale
import java.util.concurrent.Executors

/**
 * The "Over time" page of the Allocation screen: month-by-month market-cap and sector mix of the portfolio.
 * All maths is in Go (finance.ComputeCompositionHistory); this class only draws it. Values are shown as a share of the
 * holdings that have published fund data (the no-data slice is reported in the coverage line instead of being drawn),
 * so the stack always totals 100%. Fund data is kept current by FundDataSync; "Refresh data" forces a check.
 */
class CompositionHistoryPage(private val activity: AppCompatActivity) {

    val root: View = LayoutInflater.from(activity).inflate(R.layout.page_allocation_over_time, null)

    private val gson = Gson()
    private val bg = Executors.newSingleThreadExecutor()
    private var lastQuickSync = 0L
    private val main = Handler(Looper.getMainLooper())
    private val chart: StackedAreaChartView = root.findViewById(R.id.overTimeChart)
    private val legend: ChipGroup = root.findViewById(R.id.overTimeLegend)
    private val kpis: LinearLayout = root.findViewById(R.id.overTimeKpis)
    private val rows: LinearLayout = root.findViewById(R.id.overTimeRows)
    private val memberButton: Button = root.findViewById(R.id.overTimeMember)
    private val refreshButton: Button = root.findViewById(R.id.overTimeRefresh)
    private val modeGroup: MaterialButtonToggleGroup = root.findViewById(R.id.overTimeModeGroup)

    private var memberIds = listOf("")
    private var memberLabels = listOf("All")
    private var memberIndex = 0
    private var sectorMode = false
    private var points: List<CompositionPoint> = emptyList()
    private var series: List<StackedAreaChartView.Series> = emptyList()
    private var loadedOnce = false
    private var fundId = ""
    private var fundName = ""
    private var fundChoices: List<Pair<String, String>> = emptyList() // assetId -> display name
    private val fundButton: Button = root.findViewById(R.id.overTimeFund)
    /** Sectors drawn in the chart (max 6 - the palette has six validated hues), each keeping its colour slot while selected. */
    private val picked = LinkedHashMap<String, Int>()
    private val slotColors = intArrayOf(R.color.viz_s1, R.color.viz_s2, R.color.viz_s3, R.color.viz_s4, R.color.viz_s5, R.color.viz_s6)

    private val monthIn = SimpleDateFormat("yyyy-MM", Locale.US)
    private val monthOut = SimpleDateFormat("MMM yy", Locale.US)

    init {
        modeGroup.check(R.id.overTimeModeCap)
        modeGroup.addOnButtonCheckedListener { _, id, checked ->
            if (checked) { sectorMode = id == R.id.overTimeModeSector; rebuild(true) }
        }
        chart.onScrub = { showSelection(it) }
        memberButton.setOnClickListener {
            val menu = PopupMenu(activity, memberButton)
            memberLabels.forEachIndexed { i, l -> menu.menu.add(0, i, i, l) }
            menu.setOnMenuItemClickListener {
                memberIndex = it.itemId
                memberButton.text = memberLabels[memberIndex] + " ▾"
                load(); true
            }
            menu.show()
        }
        refreshButton.setOnClickListener { forceRefresh() }
        fundButton.setOnClickListener {
            val menu = PopupMenu(activity, fundButton)
            menu.menu.add(0, 0, 0, "Whole portfolio")
            fundChoices.forEachIndexed { i, f -> menu.menu.add(0, i + 1, i + 1, f.second) }
            menu.setOnMenuItemClickListener {
                if (it.itemId == 0) { fundId = ""; fundName = "" } else { fundId = fundChoices[it.itemId - 1].first; fundName = fundChoices[it.itemId - 1].second }
                fundButton.text = (if (fundId.isEmpty()) "Whole portfolio" else fundName) + " ▾"
                memberButton.visibility = if (fundId.isEmpty() && memberIds.size > 2) View.VISIBLE else View.GONE
                load(); true
            }
            menu.show()
        }
        root.findViewById<View>(R.id.overTimeAddFund).setOnClickListener {
            activity.startActivity(android.content.Intent(activity, AdditionalFundsActivity::class.java))
        }
        val sources = root.findViewById<TextView>(R.id.overTimeSources)
        root.findViewById<TextView>(R.id.overTimeSourcesToggle).setOnClickListener {
            val show = sources.visibility != View.VISIBLE
            sources.visibility = if (show) View.VISIBLE else View.GONE
            (it as TextView).text = if (show) "Data sources ▾" else "Data sources ▸"
        }
    }

    /** Called whenever the Allocation screen resumes. */
    fun refresh() {
        loadMembers()
        load()
        // Cheap manifest check (one tiny request); downloads only if the published data is newer than what is stored.
        val now = System.currentTimeMillis()
        if (now - lastQuickSync < 10 * 60 * 1000L) return
        lastQuickSync = now
        bg.execute {
            val res = try { Bridge.syncFundHistoriesAtPath(PortfolioStorage.filePath(activity)) } catch (e: Exception) { "" }
            if (res.contains("\"synced\"")) main.post { load() }
        }
    }

    private fun loadMembers() {
        try {
            val json = PortfolioLoadCache.load(PortfolioStorage.filePath(activity))
            val members: List<Member> = gson.fromJson(Bridge.listMembers(json), object : TypeToken<List<Member>>() {}.type) ?: emptyList()
            memberIds = listOf("") + members.map { it.id }
            memberLabels = listOf("All") + members.map { it.name }
            memberIndex = memberIndex.coerceAtMost(memberIds.size - 1)
            memberButton.text = memberLabels[memberIndex] + " ▾"
            memberButton.visibility = if (members.size > 1) View.VISIBLE else View.GONE
        } catch (e: Exception) { /* keep default */ }
    }

    private fun isErr(s: String) = s.trimStart().startsWith("{\"error\"")
    private fun label(m: String) = try { monthOut.format(monthIn.parse(m)!!) } catch (e: Exception) { m }
    private fun color(id: Int) = ContextCompat.getColor(activity, id)

    private fun load() {
        bg.execute {
            val json = PortfolioLoadCache.load(PortfolioStorage.filePath(activity))
            val res = if (fundId.isEmpty()) Bridge.computeCompositionHistory(json, memberIds.getOrElse(memberIndex) { "" })
            else Bridge.computeFundCompositionHistory(json, fundId)
            fundChoices = try {
                val snap = gson.fromJson(json, AdditionalFundsSnapshot::class.java)
                val withData = snap.fundHistories.orEmpty().filter { !it.months.isNullOrEmpty() }.map { it.assetId }.toSet()
                snap.assets.orEmpty().filter { it.id in withData }
                    .map { it.id to NicknameResolver.resolve(it.name, it.nickname) }.distinctBy { it.second }.sortedBy { it.second.lowercase() }
            } catch (e: Exception) { fundChoices }
            val pts: List<CompositionPoint> = if (isErr(res)) emptyList() else try {
                gson.fromJson(res, object : TypeToken<List<CompositionPoint>>() {}.type) ?: emptyList()
            } catch (e: Exception) { emptyList() }
            val status = sourcesText(json)
            main.post {
                points = pts
                root.findViewById<TextView>(R.id.overTimeSources).text = status
                rebuild(!loadedOnce)
                loadedOnce = true
            }
        }
    }

    private fun sourcesText(portfolioJson: String): String = try {
        val snap = gson.fromJson(portfolioJson, AdditionalFundsSnapshot::class.java)
        val names = snap.assets.orEmpty().associateBy { it.id }
        snap.fundHistories.orEmpty().mapNotNull { h ->
            val a = names[h.assetId] ?: return@mapNotNull null
            "${NicknameResolver.resolve(a.name, a.nickname)} - through ${h.months?.lastOrNull()?.month ?: "no data"}. ${h.note.orEmpty()}"
        }.joinToString("\n\n")
    } catch (e: Exception) { "" }

    /** value as a share of covered holdings (no-data slice removed), so the stack totals 100. */
    private fun cov(p: CompositionPoint) = (100.0 - p.noData).coerceAtLeast(1.0)

    private fun rebuild(animate: Boolean) {
        val title = root.findViewById<TextView>(R.id.overTimeChartTitle)
        val sub = root.findViewById<TextView>(R.id.overTimeChartSub)
        title.text = (if (sectorMode) "Sector mix by month" else "Market cap mix by month") + (if (fundId.isNotEmpty()) " · fund" else "")
        sub.text = if (fundId.isNotEmpty() && !sectorMode) "This fund's own holdings by market cap · drag to explore"
        else if (sectorMode) "Share of your equity holdings · tap sectors below to chart them · drag to explore"
        else "Share of your holdings with published fund data · drag to explore"
        if (points.isEmpty()) {
            chart.setData(emptyList(), emptyList(), false)
            legend.removeAllViews(); kpis.removeAllViews(); rows.removeAllViews()
            root.findViewById<TextView>(R.id.overTimeBreakdownTitle).text = "No data yet"
            root.findViewById<TextView>(R.id.overTimeBreakdownSub).text = "Needs holdings with published fund data. It syncs automatically; tap Refresh data to check now."
            root.findViewById<TextView>(R.id.overTimeCoverage).text = ""
            return
        }
        val n = points.size
        val list = ArrayList<StackedAreaChartView.Series>()
        fun add(name: String, col: Int, f: (CompositionPoint) -> Double) {
            val v = DoubleArray(n) { f(points[it]) / cov(points[it]) * 100.0 }
            if (v.any { it > 0.05 }) list.add(StackedAreaChartView.Series(name, col, v))
        }
        if (!sectorMode) {
            add("Large cap", color(R.color.viz_s1)) { it.large }
            add("Mid cap", color(R.color.viz_s2)) { it.mid }
            add("Small cap", color(R.color.viz_s3)) { it.small }
            add("Cash", color(R.color.viz_s4)) { it.cash }
            add("Debt", color(R.color.viz_s5)) { it.debt }
            add("Foreign", color(R.color.viz_s7)) { it.foreign }
            add("Other", color(R.color.viz_neutral)) { it.other }
            add("Unclassified equity", color(R.color.viz_neutral_soft)) { it.unclassified }
        } else {
            picked.keys.retainAll { k -> points.any { it.sectors?.containsKey(k) == true } }
            if (picked.isEmpty()) {
                val avg = HashMap<String, Double>()
                points.forEach { p -> p.sectors.orEmpty().forEach { (k, v) -> avg[k] = (avg[k] ?: 0.0) + v / n } }
                avg.entries.sortedByDescending { it.value }.take(5).forEachIndexed { i, e -> picked[e.key] = i }
            }
            picked.forEach { (k, slot) -> addSector(list, k, slot) }
        }
        series = list
        chart.setData(points.map { label(it.month) }, list, animate,
            if (sectorMode) niceMax(list) else 100.0)
        buildLegend()
        buildKpis()
        showSelection(n - 1)
        val last = points.last()
        root.findViewById<TextView>(R.id.overTimeCoverage).text =
            if (fundId.isNotEmpty()) "Source: the fund house's published monthly portfolio, classified with AMFI's half-yearly cap list. Months marked estimated are look-throughs from a same-index fund." else
            "Published fund data covers ${"%.0f".format(100 - last.noData)}% of your latest value" +
                (if (last.estimated >= 1) ", of which ${"%.0f".format(last.estimated)}% uses estimated months" else "") +
                ". The newest month reuses the previous month's portfolio until fund houses publish it, around the middle of the following month."
    }

    /** equity base for sector percentages: the sum of the sector split, so the full list always totals 100. */
    private fun sectorBase(p: CompositionPoint) = p.sectors.orEmpty().values.sum().coerceAtLeast(0.0001)

    private fun addSector(list: ArrayList<StackedAreaChartView.Series>, name: String, slot: Int) {
        val v = DoubleArray(points.size) { i -> (points[i].sectors?.get(name) ?: 0.0) / sectorBase(points[i]) * 100.0 }
        list.add(StackedAreaChartView.Series(name, color(slotColors[slot]), v))
    }

    private fun niceMax(list: List<StackedAreaChartView.Series>): Double {
        var m = 0.0
        for (i in points.indices) m = maxOf(m, list.sumOf { it.values[i] })
        val steps = doubleArrayOf(4.0, 8.0, 12.0, 20.0, 40.0, 60.0, 80.0, 100.0)
        return steps.firstOrNull { it >= m * 1.05 } ?: 100.0
    }

    private fun toggleSector(name: String) {
        if (picked.containsKey(name)) {
            if (picked.size > 1) picked.remove(name)
        } else {
            if (picked.size >= 6) { // the palette holds six hues: the oldest pick makes room
                val old = picked.keys.first(); picked.remove(old)
            }
            val used = picked.values.toSet()
            picked[name] = (0..5).first { it !in used }
        }
        rebuild(false)
    }

    private fun dot(col: Int, sizeDp: Int = 10): View = View(activity).apply {
        val s = (sizeDp * activity.resources.displayMetrics.density).toInt()
        layoutParams = LinearLayout.LayoutParams(s, s).apply { marginEnd = (8 * activity.resources.displayMetrics.density).toInt() }
        background = GradientDrawable().apply { shape = GradientDrawable.OVAL; setColor(col) }
    }

    private fun buildLegend() {
        legend.removeAllViews()
        val ctx = ContextThemeWrapper(activity, com.google.android.material.R.style.Widget_Material3_Chip_Assist)
        series.forEach { s ->
            legend.addView(Chip(ctx).apply {
                text = s.name
                textSize = 12f
                isClickable = false; isCheckable = false; isFocusable = false
                chipIcon = GradientDrawable().apply { shape = GradientDrawable.OVAL; setColor(s.color); setSize(28, 28) }
                isChipIconVisible = true
                chipIconTint = null
                chipIconSize = 11f * activity.resources.displayMetrics.density
                chipStartPadding = 6f
                chipBackgroundColor = ColorStateList.valueOf(0)
                chipStrokeWidth = 0f
            })
        }
    }

    private fun buildKpis() {
        kpis.removeAllViews()
        val first = 0
        val last = points.size - 1
        val picks = if (!sectorMode) series.filter { it.name in listOf("Large cap", "Mid cap", "Small cap") }
        else series.take(3)
        picks.forEachIndexed { i, s ->
            val card = MaterialCardView(activity).apply {
                radius = 18f * activity.resources.displayMetrics.density
                cardElevation = 0f
                strokeWidth = (1 * activity.resources.displayMetrics.density).toInt()
                strokeColor = color(R.color.viz_grid)
                layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f).apply {
                    if (i > 0) marginStart = (8 * activity.resources.displayMetrics.density).toInt()
                }
            }
            val pad = (12 * activity.resources.displayMetrics.density).toInt()
            val col = LinearLayout(activity).apply { orientation = LinearLayout.VERTICAL; setPadding(pad, pad, pad, pad) }
            val head = LinearLayout(activity).apply { gravity = Gravity.CENTER_VERTICAL }
            head.addView(dot(s.color, 8))
            head.addView(TextView(activity).apply {
                text = s.name; textSize = 11f; setTextColor(color(R.color.viz_text_secondary)); maxLines = 1; ellipsize = android.text.TextUtils.TruncateAt.END
            })
            col.addView(head)
            col.addView(TextView(activity).apply {
                text = "%.1f%%".format(s.values[last]); textSize = 22f; setTypeface(typeface, android.graphics.Typeface.BOLD)
            })
            val d = s.values[last] - s.values[first]
            col.addView(TextView(activity).apply {
                text = (if (d >= 0) "▲ " else "▼ ") + "%.1f pts since %s".format(Math.abs(d), label(points[first].month))
                textSize = 10.5f; setTextColor(color(R.color.viz_text_muted))
            })
            card.addView(col)
            kpis.addView(card)
        }
    }

    private fun showSelection(idx: Int) {
        if (idx !in points.indices) return
        val p = points[idx]
        root.findViewById<TextView>(R.id.overTimeBreakdownTitle).text = label(p.month)
        root.findViewById<TextView>(R.id.overTimeBreakdownSub).text =
            if (fundId.isNotEmpty()) "${fundName.ifBlank { "Fund" }} · from the fund house's monthly disclosure" + (if (p.estimated > 0) " (estimated month)" else "")
            else "Portfolio value " + IndianCurrencyFormatter.format(p.value) + " · ${"%.0f".format(100 - p.noData)}% covered by fund data"
        rows.removeAllViews()
        val dens = activity.resources.displayMetrics.density
        val entries: List<Triple<String, Int?, Double>> = if (!sectorMode) {
            series.map { Triple(it.name, it.color as Int?, it.values[idx]) }
        } else {
            val bs = sectorBase(p)
            p.sectors.orEmpty().map { (k, v) ->
                Triple(k, picked[k]?.let { slot -> color(slotColors[slot]) }, v / bs * 100.0)
            }
        }
        entries.filter { it.third >= 0.05 }.sortedByDescending { it.third }.forEach { (name, col, v) ->
            val s = StackedAreaChartView.Series(name, col ?: color(R.color.viz_neutral), DoubleArray(0))
            val row = LinearLayout(activity).apply { orientation = LinearLayout.VERTICAL; setPadding(0, (6 * dens).toInt(), 0, (6 * dens).toInt()) }
            val line = LinearLayout(activity).apply { gravity = Gravity.CENTER_VERTICAL }
            line.addView(dot(s.color))
            line.addView(TextView(activity).apply {
                text = s.name; textSize = 14f; layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f)
            })
            line.addView(TextView(activity).apply {
                text = "%.1f%%".format(v); textSize = 14f; setTypeface(typeface, android.graphics.Typeface.BOLD)
            })
            row.addView(line)
            val track = LinearLayout(activity).apply {
                layoutParams = LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, (4 * dens).toInt()).apply { topMargin = (5 * dens).toInt() }
                background = GradientDrawable().apply { cornerRadius = 2 * dens; setColor(color(R.color.viz_grid)) }
                weightSum = 100f
            }
            track.addView(View(activity).apply {
                layoutParams = LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.MATCH_PARENT, v.toFloat().coerceIn(0.5f, 100f))
                background = GradientDrawable().apply { cornerRadius = 2 * dens; setColor(s.color) }
            })
            row.addView(track)
            if (sectorMode) {
                row.isClickable = true
                row.setOnClickListener { toggleSector(name) }
                row.alpha = if (col == null) 0.72f else 1f
            }
            rows.addView(row)
        }
        val contrib = root.findViewById<TextView>(R.id.overTimeContrib)
        fun fmt(m: Map<String, Double>?): String = m.orEmpty().entries.sortedByDescending { it.value }.take(3)
            .joinToString(", ") { "${FundNameFormatter.shorten(it.key).ifBlank { it.key }} ${"%.1f".format(it.value)}%" }
        val parts = listOfNotNull(
            fmt(p.otherBy).takeIf { it.isNotBlank() }?.let { "Other comes from: $it" },
            fmt(p.noDataBy).takeIf { it.isNotBlank() }?.let { "No fund data for: $it" }
        )
        if (sectorMode) {
            val c = cov(p)
            val non = p.cash + p.debt + p.foreign + p.other
            fun pc(x: Double) = "%.1f%%".format(x / c * 100)
            val t = "Not in the sector split: ${pc(non)} of covered value - cash ${pc(p.cash)}, debt ${pc(p.debt)}, foreign ${pc(p.foreign)}, other ${pc(p.other)}." +
                (if (p.unclassified >= 0.5) " Indian shares missing from the AMFI cap list (${pc(p.unclassified)}) are still sector-tagged." else "")
            contrib.visibility = View.VISIBLE
            contrib.text = t
        } else {
            contrib.visibility = if (parts.isEmpty()) View.GONE else View.VISIBLE
            contrib.text = parts.joinToString("\n")
        }
    }

    private fun forceRefresh() {
        refreshButton.isEnabled = false
        refreshButton.text = "Checking…"
        bg.execute {
            val res = Bridge.forceSyncFundHistoriesAtPath(PortfolioStorage.filePath(activity))
            main.post {
                refreshButton.isEnabled = true
                refreshButton.text = "Refresh data"
                if (isErr(res)) Toast.makeText(activity, "Could not reach fund data - try again later", Toast.LENGTH_LONG).show()
                load()
            }
        }
    }
}
