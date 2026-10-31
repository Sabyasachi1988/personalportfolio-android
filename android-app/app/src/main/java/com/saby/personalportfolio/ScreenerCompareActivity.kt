package com.saby.personalportfolio

import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.HorizontalScrollView
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TableLayout
import android.widget.TableRow
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.view.ContextThemeWrapper
import androidx.core.content.ContextCompat
import com.google.android.material.chip.Chip
import com.google.android.material.chip.ChipGroup
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

/** Side-by-side comparison of the funds ticked in the screener: growth-of-100 chart plus a metric table with the best value marked. */
class ScreenerCompareActivity : AppCompatActivity() {
    private val main = Handler(Looper.getMainLooper())
    private val gson = Gson()
    private var funds: List<ScreenerFund> = emptyList()
    private var series: Map<String, NavSeries> = emptyMap()
    private var weeksBack = 156
    private lateinit var chart: GrowthChartView
    private lateinit var legend: LinearLayout
    private lateinit var note: TextView
    private val slots = intArrayOf(R.color.viz_s1, R.color.viz_s2, R.color.viz_s3, R.color.viz_s4, R.color.viz_s5, R.color.viz_s6)

    data class NavSeries(val w0: Int = 0, val nav: List<Double> = emptyList())

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()
    private fun color(id: Int) = ContextCompat.getColor(this, id)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val codes = intent.getIntArrayExtra("codes")?.toSet().orEmpty()
        funds = ScreenerStore.load(this).filter { it.row.code in codes }
        val box = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(dp(16), dp(16), dp(16), dp(24)) }
        box.addView(TextView(this).apply { text = "Compare funds"; textSize = 20f; setTypeface(typeface, Typeface.BOLD) })
        box.addView(TextView(this).apply { text = "Growth of ₹100 · tap or drag the chart"; textSize = 12f; setTextColor(color(R.color.colorNeutral)); setPadding(0, dp(2), 0, dp(8)) })

        val ctx = ContextThemeWrapper(this, com.google.android.material.R.style.Widget_Material3_Chip_Filter)
        val periods = ChipGroup(this).apply { isSingleSelection = true; isSelectionRequired = true }
        for ((label, w) in listOf("1Y" to 52, "3Y" to 156, "5Y" to 260, "Max" to 100000)) {
            periods.addView(Chip(ctx).apply {
                text = label; isCheckable = true; isChecked = w == weeksBack
                setOnClickListener { weeksBack = w; redraw() }
            })
        }
        box.addView(periods)
        chart = GrowthChartView(this)
        box.addView(chart, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, dp(260)))
        legend = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL; setPadding(0, dp(8), 0, dp(4)) }
        box.addView(legend)
        note = TextView(this).apply { textSize = 11f; setTextColor(color(R.color.colorNeutral)); setPadding(0, dp(2), 0, dp(10)) }
        box.addView(note)
        box.addView(TextView(this).apply { text = "Side by side"; textSize = 15f; setTypeface(typeface, Typeface.BOLD); setPadding(0, dp(6), 0, dp(6)) })
        box.addView(HorizontalScrollView(this).apply { addView(buildTable()) })
        box.addView(TextView(this).apply {
            text = "Green = best of the selected funds on that row. Returns over 1 year are annualised. Volatility and Sharpe use the last 3 years of month-end NAVs " +
                "(Sharpe uses a 6.5% risk-free rate). Cap mix is each fund's latest published portfolio, classified with AMFI's cap list."
            textSize = 11f; setTextColor(color(R.color.colorNeutral)); setPadding(0, dp(8), 0, dp(8))
        })
        box.addView(Button(this).apply { text = "Track these ${funds.size} funds"; setOnClickListener { ScreenerTrack.trackAll(this@ScreenerCompareActivity, funds.map { it.row }) } })
        setContentView(ScrollView(this).apply { addView(box) })

        Thread {
            val res = Bridge.screenerNavSeries(ScreenerStore.dir(this).absolutePath, funds.joinToString(",") { it.row.code.toString() })
            val parsed: Map<String, NavSeries> = try {
                gson.fromJson(res, object : TypeToken<Map<String, NavSeries>>() {}.type) ?: emptyMap()
            } catch (e: Exception) { emptyMap() }
            main.post { series = parsed; redraw() }
        }.start()
    }

    private val fmt = SimpleDateFormat("MMM yy", Locale.US).apply { timeZone = TimeZone.getTimeZone("UTC") }

    private fun redraw() {
        val usable = funds.filter { series[it.row.code.toString()]?.nav?.isNotEmpty() == true }
        if (usable.isEmpty()) { note.text = "No NAV history in the downloaded data for these funds."; chart.setData(emptyList(), emptyList()); return }
        val firstWeek = usable.maxOf { series[it.row.code.toString()]!!.w0 }
        val lastWeek = usable.minOf { val s = series[it.row.code.toString()]!!; s.w0 + s.nav.size - 1 }
        val start = maxOf(firstWeek, lastWeek - weeksBack)
        val n = lastWeek - start + 1
        if (n < 2) { note.text = "Not enough overlapping history to compare."; return }
        val lines = usable.mapIndexed { i, f ->
            val s = series[f.row.code.toString()]!!
            val base = s.nav[start - s.w0]
            GrowthChartView.Line(f.shortName, color(slots[i % slots.size]), DoubleArray(n) { s.nav[start - s.w0 + it] / base * 100.0 })
        }
        chart.setData((0 until n).map { fmt.format(Date((start + it) * 7L * 86400000L)) }, lines)
        legend.removeAllViews()
        lines.forEachIndexed { i, l ->
            val total = l.values.last() - 100
            legend.addView(LinearLayout(this).apply {
                gravity = Gravity.CENTER_VERTICAL; setPadding(0, dp(3), 0, dp(3))
                addView(View(context).apply {
                    layoutParams = LinearLayout.LayoutParams(dp(10), dp(10)).apply { marginEnd = dp(8) }
                    background = GradientDrawable().apply { shape = GradientDrawable.OVAL; setColor(l.color) }
                })
                addView(TextView(context).apply { text = l.name; textSize = 13f; maxLines = 2 }, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
                addView(TextView(context).apply {
                    text = "₹%.0f (%s%.1f%%)".format(l.values.last(), if (total >= 0) "+" else "", total); textSize = 13f; setTypeface(typeface, Typeface.BOLD)
                })
            })
        }
        val dropped = funds.size - usable.size
        note.text = "From ${fmt.format(Date(start * 7L * 86400000L))}, the start of the period or of the youngest fund's history (weekly NAVs)." +
            if (dropped > 0) " $dropped fund(s) had no NAV data." else ""
    }

    private data class Metric(val label: String, val higherBetter: Boolean?, val get: (ScreenerFund) -> Double?, val show: (Double) -> String)

    private fun buildTable(): TableLayout {
        val p1 = { v: Double -> "%+.1f%%".format(v) }
        val p0 = { v: Double -> "%.1f%%".format(v) }
        val metrics = listOf(
            Metric("1Y return", true, { it.ret("1y") }, p1), Metric("3Y return (p.a.)", true, { it.ret("3y") }, p1),
            Metric("5Y return (p.a.)", true, { it.ret("5y") }, p1), Metric("10Y return (p.a.)", true, { it.ret("10y") }, p1),
            Metric("3Y rolling median", true, { it.row.roll?.get("3y")?.getOrNull(0) }, p1),
            Metric("3Y rolling worst", true, { it.row.roll?.get("3y")?.getOrNull(1) }, p1),
            Metric("5Y rolling median", true, { it.row.roll?.get("5y")?.getOrNull(0) }, p1),
            Metric("Volatility", false, { it.row.vol }, p0), Metric("Max fall", true, { it.row.mdd }, p0),
            Metric("Sharpe", true, { it.row.sharpe }, { "%.2f".format(it) }),
            Metric("Age (years)", null, { it.row.age }, { "%.1f".format(it) }),
            Metric("Large cap", null, { it.large }, p0), Metric("Mid cap", null, { it.mid }, p0), Metric("Small cap", null, { it.small }, p0)
        )
        val table = TableLayout(this)
        val head = TableRow(this)
        head.addView(cell("", bold = true, width = 112))
        funds.forEachIndexed { i, f ->
            head.addView(LinearLayout(this).apply {
                orientation = LinearLayout.VERTICAL; setPadding(dp(6), dp(4), dp(6), dp(8))
                layoutParams = TableRow.LayoutParams(dp(118), ViewGroup.LayoutParams.WRAP_CONTENT)
                addView(View(context).apply {
                    layoutParams = LinearLayout.LayoutParams(dp(24), dp(4)).apply { bottomMargin = dp(4) }
                    background = GradientDrawable().apply { cornerRadius = dp(2).toFloat(); setColor(color(slots[i % slots.size])) }
                })
                addView(TextView(context).apply { text = f.shortName; textSize = 11.5f; setTypeface(typeface, Typeface.BOLD); maxLines = 4; ellipsize = android.text.TextUtils.TruncateAt.END })
            })
        }
        table.addView(head)
        for (m in metrics) {
            val vals = funds.map { m.get(it) }
            val best = if (m.higherBetter == null) null else vals.filterNotNull().let { l -> if (l.size < 2) null else if (m.higherBetter) l.maxOrNull()!! else l.minOrNull()!! }
            val row = TableRow(this)
            row.addView(cell(m.label, width = 112, muted = true))
            vals.forEach { v ->
                val isBest = best != null && v != null && v == best
                row.addView(cell(if (v == null) "–" else m.show(v), bold = isBest, good = isBest, width = 118))
            }
            table.addView(row)
        }
        return table
    }

    private fun cell(text: String, bold: Boolean = false, good: Boolean = false, muted: Boolean = false, width: Int = 118) = TextView(this).apply {
        this.text = text; textSize = 12.5f; setPadding(dp(6), dp(7), dp(6), dp(7))
        layoutParams = TableRow.LayoutParams(dp(width), ViewGroup.LayoutParams.WRAP_CONTENT)
        if (bold) setTypeface(typeface, Typeface.BOLD)
        setTextColor(color(if (good) R.color.colorGain else if (muted) R.color.colorNeutral else R.color.colorOnSurface))
    }
}
