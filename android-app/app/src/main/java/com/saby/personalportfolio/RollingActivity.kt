package com.saby.personalportfolio

import android.content.Context
import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.InputType
import android.view.View
import android.widget.EditText
import android.widget.PopupMenu
import android.widget.TextView
import androidx.appcompat.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.google.android.material.tabs.TabLayout
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.Executors

/**
 * Portfolio rolling return: pick a window length (in months) and see the
 * return of every such window across the portfolio's actual history, as
 * stats plus a line over time. The maths lives in [RollingMath].
 */
class RollingActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    private lateinit var tabs: TabLayout
    private lateinit var statusText: TextView
    private lateinit var titleText: TextView
    private lateinit var latestText: TextView
    private lateinit var latestCaption: TextView
    private lateinit var medianText: TextView
    private lateinit var worstText: TextView
    private lateinit var worstDate: TextView
    private lateinit var bestText: TextView
    private lateinit var bestDate: TextView
    private lateinit var positiveText: TextView
    private lateinit var countText: TextView
    private lateinit var scrubText: TextView
    private lateinit var noteText: TextView
    private lateinit var chart: RollingChartView
    private lateinit var memberTab: TextView
    private lateinit var axisTab: TextView
    private lateinit var chips: Map<Int, TextView>
    private lateinit var customChip: TextView

    private var memberIds: List<String> = emptyList()
    private var memberLabels: List<String> = emptyList()
    private var selectedMemberIndex = 0
    private var selectedAxisIndex = 0
    private var months = 12

    private var dates: List<String> = emptyList()
    private var days = IntArray(0)
    private var invested = DoubleArray(0)
    private var value = DoubleArray(0)
    private var loadGeneration = 0
    private var shown: RollingResult? = null

    private val presetMonths = listOf(1, 3, 6, 12, 24)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_rolling)

        months = getSharedPreferences(PREFS, Context.MODE_PRIVATE).getInt(KEY_MONTHS, 12).coerceIn(1, 120)

        tabs = findViewById(R.id.rollingSectionTabLayout)
        statusText = findViewById(R.id.rollingStatusText)
        titleText = findViewById(R.id.rollingTitle)
        latestText = findViewById(R.id.rollingLatest)
        latestCaption = findViewById(R.id.rollingLatestCaption)
        medianText = findViewById(R.id.rollingMedian)
        worstText = findViewById(R.id.rollingWorst)
        worstDate = findViewById(R.id.rollingWorstDate)
        bestText = findViewById(R.id.rollingBest)
        bestDate = findViewById(R.id.rollingBestDate)
        positiveText = findViewById(R.id.rollingPositive)
        countText = findViewById(R.id.rollingCount)
        scrubText = findViewById(R.id.rollingScrubText)
        noteText = findViewById(R.id.rollingNote)
        chart = findViewById(R.id.rollingChart)
        memberTab = findViewById(R.id.rollingMemberTab)
        axisTab = findViewById(R.id.rollingAxisTab)
        customChip = findViewById(R.id.rollingChipCustom)
        chips = mapOf(
            1 to findViewById(R.id.rollingChip1M),
            3 to findViewById(R.id.rollingChip3M),
            6 to findViewById(R.id.rollingChip6M),
            12 to findViewById(R.id.rollingChip1Y),
            24 to findViewById(R.id.rollingChip2Y)
        )

        // Rolling is the third section of the Progression | Returns | Rolling
        // switch. Progression is the screen that launched this one, so it is
        // reached by finishing; Returns is its own screen.
        tabs.getTabAt(2)?.select()
        tabs.addOnTabSelectedListener(object : TabLayout.OnTabSelectedListener {
            override fun onTabSelected(tab: TabLayout.Tab) {
                when (tab.position) {
                    0 -> finish()
                    1 -> {
                        startActivity(Intent(this@RollingActivity, ReturnsActivity::class.java))
                        finish()
                    }
                }
            }
            override fun onTabUnselected(tab: TabLayout.Tab) {}
            override fun onTabReselected(tab: TabLayout.Tab) {}
        })

        chips.forEach { (m, chip) -> chip.setOnClickListener { setMonths(m) } }
        customChip.setOnClickListener { showCustomDialog() }

        memberTab.setOnClickListener { showMemberPicker() }
        axisTab.text = ProgressionAxis.entries[selectedAxisIndex].label
        axisTab.setOnClickListener { showAxisPicker() }

        chart.onScrub = { index -> showScrub(index) }

        renderChips()
        loadMemberList()
    }

    override fun onResume() {
        super.onResume()
        tabs.getTabAt(2)?.select()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadMemberList() {
        try {
            val portfolioJson = PortfolioLoadCache.load(PortfolioStorage.filePath(this))
            val membersJson = Bridge.listMembers(portfolioJson)
            val type = object : TypeToken<List<Member>>() {}.type
            val members: List<Member> = try { gson.fromJson(membersJson, type) ?: emptyList() } catch (e: Exception) { emptyList() }
            memberIds = listOf("") + members.map { it.id }
            memberLabels = listOf("All (family)") + members.map { it.name }
        } catch (e: Exception) {
            memberIds = listOf("")
            memberLabels = listOf("All (family)")
        }
        selectedMemberIndex = selectedMemberIndex.coerceIn(0, memberIds.size - 1)
        memberTab.text = memberLabels[selectedMemberIndex]
        loadSeries()
    }

    private fun showMemberPicker() {
        val popup = PopupMenu(this, memberTab)
        memberLabels.forEachIndexed { i, l -> popup.menu.add(0, i, i, l) }
        popup.setOnMenuItemClickListener { item ->
            selectedMemberIndex = item.itemId
            memberTab.text = memberLabels.getOrElse(selectedMemberIndex) { "All (family)" }
            loadSeries()
            true
        }
        popup.show()
    }

    private fun showAxisPicker() {
        val popup = PopupMenu(this, axisTab)
        ProgressionAxis.entries.forEachIndexed { i, a -> popup.menu.add(0, i, i, a.label) }
        popup.setOnMenuItemClickListener { item ->
            selectedAxisIndex = item.itemId
            axisTab.text = ProgressionAxis.entries[selectedAxisIndex].label
            loadSeries()
            true
        }
        popup.show()
    }

    private fun loadSeries() {
        val generation = ++loadGeneration
        statusText.text = "Loading…"
        val memberId = memberIds.getOrElse(selectedMemberIndex) { "" }
        val axis = ProgressionAxis.entries[selectedAxisIndex.coerceIn(0, ProgressionAxis.entries.size - 1)]

        backgroundExecutor.execute {
            try {
                val portfolioJson = PortfolioLoadCache.load(PortfolioStorage.filePath(this))
                val cachePath = PortfolioStorage.progressionCachePath(this)
                val today = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())
                val resultJson = Bridge.computeProgression(portfolioJson, memberId, axis.bridgeValue, today, cachePath)
                mainThread.post {
                    if (generation != loadGeneration || isFinishing) return@post
                    try {
                        applySeries(resultJson)
                    } catch (e: Exception) {
                        statusText.text = "Could not display rolling return: ${e.javaClass.simpleName}: ${e.message}"
                    }
                }
            } catch (e: Exception) {
                mainThread.post {
                    if (generation != loadGeneration) return@post
                    statusText.text = "Could not load rolling return: ${e.javaClass.simpleName}: ${e.message}"
                }
            }
        }
    }

    private fun applySeries(resultJson: String) {
        if (isBridgeError(resultJson)) {
            statusText.text = "Could not compute: $resultJson"
            clearResult()
            return
        }
        val type = object : TypeToken<List<ProgressionPoint>>() {}.type
        val loaded: List<ProgressionPoint> = try { gson.fromJson(resultJson, type) ?: emptyList() } catch (e: Exception) { emptyList() }
        if (loaded.isEmpty()) {
            statusText.text = "No history yet — this needs transactions and price history. Run \"Update Price History\" from Settings first."
            clearResult()
            return
        }
        dates = loaded.map { it.date }
        days = IntArray(loaded.size) { WindowMath.dayNumber(loaded[it].date) ?: 0 }
        invested = DoubleArray(loaded.size) { loaded[it].invested }
        value = DoubleArray(loaded.size) { loaded[it].value }
        statusText.text = "Weekly history, ${dates.first()} to ${dates.last()}"
        render()
    }

    private fun clearResult() {
        dates = emptyList(); days = IntArray(0); invested = DoubleArray(0); value = DoubleArray(0)
        shown = null
        chart.setData(emptyList(), DoubleArray(0))
        titleText.text = ""
        latestText.text = "—"
        latestText.setTextColor(ContextCompat.getColor(this, R.color.colorNeutral))
        latestCaption.text = ""
        listOf(medianText, worstText, worstDate, bestText, bestDate, positiveText, countText).forEach { it.text = "" }
        scrubText.text = ""
        noteText.text = ""
    }

    private fun setMonths(m: Int) {
        months = m.coerceIn(1, 120)
        getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit().putInt(KEY_MONTHS, months).apply()
        renderChips()
        if (days.isNotEmpty()) render()
    }

    private fun renderChips() {
        chips.forEach { (m, chip) -> chip.alpha = if (m == months) 1f else 0.55f }
        val custom = months !in presetMonths
        customChip.alpha = if (custom) 1f else 0.55f
        customChip.text = if (custom) "${months}M" else "Custom"
    }

    private fun showCustomDialog() {
        val input = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_NUMBER
            hint = "Months (1–120)"
            setText(months.toString())
            setSelection(text.length)
        }
        AlertDialog.Builder(this)
            .setTitle("Rolling window (months)")
            .setView(input)
            .setPositiveButton("Apply") { _, _ ->
                val m = input.text.toString().trim().toIntOrNull()
                if (m != null && m in 1..120) setMonths(m)
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun periodLabel(m: Int): String =
        if (m % 12 == 0) "${m / 12}-year" else "$m-month"

    private fun render() {
        val result = RollingMath.compute(days, invested, value, months)
        shown = result
        val kind = if (result.annualised) "annualised" else "total"
        titleText.text = "${periodLabel(months)} rolling return · $kind"

        noteText.text = "Time-weighted: deposits and withdrawals are removed, so this is what the markets did to your money, not how much you added. " +
            "Measured from when the portfolio first reached ₹1,00,000 (earlier weeks are too small to measure reliably). " +
            "Rupee-based, so international holdings include currency moves. Windows of a year or more are annualised; shorter ones are total returns."

        if (result.seriesStartIndex < 0) {
            showNoResult("This portfolio never reached ₹1,00,000, so a rolling return can't be measured yet.")
            return
        }
        if (result.points.isEmpty()) {
            val have = RollingMath.monthsBetween(days[result.seriesStartIndex], days.last())
            showNoResult("Not enough history for a ${periodLabel(months)} window: there are about $have month(s) of measurable history since the portfolio reached ₹1,00,000. Try a shorter period.")
            return
        }

        val latest = result.latest ?: 0.0
        latestText.text = signed(latest)
        latestText.setTextColor(colorFor(latest))
        latestCaption.text = "Latest window, ending ${pretty(dates[result.points.last().endIndex])}"
        medianText.text = signed(result.median ?: 0.0)
        medianText.setTextColor(colorFor(result.median ?: 0.0))
        result.worst?.let {
            worstText.text = signed(it.percent); worstText.setTextColor(colorFor(it.percent))
            worstDate.text = "ending ${pretty(dates[it.endIndex])}"
        }
        result.best?.let {
            bestText.text = signed(it.percent); bestText.setTextColor(colorFor(it.percent))
            bestDate.text = "ending ${pretty(dates[it.endIndex])}"
        }
        positiveText.text = String.format(Locale.US, "%.0f%%", result.positiveSharePct ?: 0.0)
        countText.text = result.points.size.toString()

        chart.setData(
            result.points.map { dates[it.endIndex] },
            DoubleArray(result.points.size) { result.points[it].percent }
        )
    }

    private fun showNoResult(message: String) {
        shown = null
        chart.setData(emptyList(), DoubleArray(0))
        latestText.text = "—"
        latestText.setTextColor(ContextCompat.getColor(this, R.color.colorNeutral))
        latestCaption.text = message
        listOf(medianText, worstText, worstDate, bestText, bestDate, positiveText, countText).forEach { it.text = "" }
        scrubText.text = ""
    }

    private fun showScrub(i: Int) {
        val r = shown ?: return
        val p = r.points.getOrNull(i) ?: return
        val startDay = RollingMath.monthsBefore(days[p.endIndex], months)
        scrubText.text = "${pretty(isoFromDay(startDay))} → ${pretty(dates[p.endIndex])}:  ${signed(p.percent)}"
        scrubText.setTextColor(colorFor(p.percent))
    }

    private fun isoFromDay(day: Int): String {
        val (y, m, d) = RollingMath.civilFromDays(day)
        return String.format(Locale.US, "%04d-%02d-%02d", y, m, d)
    }

    private fun pretty(iso: String): String = try {
        val parsed = SimpleDateFormat("yyyy-MM-dd", Locale.US).parse(iso)
        SimpleDateFormat("d MMM yyyy", Locale.US).format(parsed!!)
    } catch (e: Exception) { iso }

    private fun signed(v: Double) = String.format(Locale.US, "%+.2f%%", v)

    private fun colorFor(v: Double) =
        ContextCompat.getColor(this, if (v >= 0) R.color.colorGain else R.color.colorLoss)

    companion object {
        private const val PREFS = "rolling_prefs"
        private const val KEY_MONTHS = "months"
    }
}
