package com.saby.personalportfolio

import android.os.Bundle
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.SeekBar
import android.widget.Spinner
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class ProgressionActivity : AppCompatActivity() {

    private val gson = Gson()
    private val isoFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)

    private lateinit var memberSpinner: Spinner
    private lateinit var axisSpinner: Spinner
    private lateinit var currencySpinner: Spinner
    private lateinit var statusText: TextView
    private lateinit var spanText: TextView
    private lateinit var dateText: TextView
    private lateinit var investedText: TextView
    private lateinit var valueText: TextView
    private lateinit var gainText: TextView
    private lateinit var xirrText: TextView
    private lateinit var chart: ProgressionChartView
    private lateinit var seekBar: SeekBar

    // Index 0 is always "All (family)" (empty memberID); indices 1.. map 1:1 with memberIds - same convention as HoldingsActivity.
    private var memberIds: List<String> = emptyList()
    private var points: List<ProgressionPoint> = emptyList()
    private var currentAxis: ProgressionAxis = ProgressionAxis.WHOLE_PORTFOLIO

    // Guards against the chart's onScrub and the SeekBar's listener
    // re-triggering each other in a feedback loop when one drives the
    // other's position programmatically.
    private var syncingScrub = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_progression)

        memberSpinner = findViewById(R.id.progressionMemberSpinner)
        axisSpinner = findViewById(R.id.progressionAxisSpinner)
        currencySpinner = findViewById(R.id.progressionCurrencySpinner)
        statusText = findViewById(R.id.progressionStatusText)
        spanText = findViewById(R.id.progressionSpanText)
        dateText = findViewById(R.id.progressionDateText)
        investedText = findViewById(R.id.progressionInvestedText)
        valueText = findViewById(R.id.progressionValueText)
        gainText = findViewById(R.id.progressionGainText)
        xirrText = findViewById(R.id.progressionXirrText)
        chart = findViewById(R.id.progressionChart)
        seekBar = findViewById(R.id.progressionSeekBar)

        axisSpinner.adapter = ArrayAdapter(
            this, android.R.layout.simple_spinner_dropdown_item, ProgressionAxis.entries.map { it.label }
        )
        currencySpinner.adapter = ArrayAdapter(
            this, android.R.layout.simple_spinner_dropdown_item, DisplayCurrency.entries.map { it.label }
        )

        val reload = { _: AdapterView<*>?, _: android.view.View?, _: Int, _: Long -> loadAndShowProgression(); Unit }
        memberSpinner.onItemSelectedListener = simpleSelectionListener(reload)
        axisSpinner.onItemSelectedListener = simpleSelectionListener(reload)
        currencySpinner.onItemSelectedListener = simpleSelectionListener { _, _, _, _ -> updateDetailCard(seekBar.progress) }

        chart.onScrub = { index ->
            if (!syncingScrub) {
                syncingScrub = true
                seekBar.progress = index
                syncingScrub = false
            }
            updateDetailCard(index)
        }
        seekBar.setOnSeekBarChangeListener(object : SeekBar.OnSeekBarChangeListener {
            override fun onProgressChanged(bar: SeekBar?, progress: Int, fromUser: Boolean) {
                if (fromUser && !syncingScrub) {
                    syncingScrub = true
                    chart.scrubTo(progress)
                    syncingScrub = false
                    updateDetailCard(progress)
                }
            }
            override fun onStartTrackingTouch(bar: SeekBar?) {}
            override fun onStopTrackingTouch(bar: SeekBar?) {}
        })

        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.PROGRESSION)
    }

    override fun onResume() {
        super.onResume()
        // Re-sync every time this screen resumes, not just once in
        // onCreate - a screen reused via CLEAR_TOP (coming back to it
        // from another tab) never re-runs onCreate, so without this
        // its nav bar could keep showing a stale selection - same
        // pattern as every other bottom-nav screen.
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.PROGRESSION)
        loadMemberSpinner()
    }

    private fun simpleSelectionListener(onSelected: (AdapterView<*>?, android.view.View?, Int, Long) -> Unit) =
        object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: android.view.View?, position: Int, id: Long) =
                onSelected(parent, view, position, id)
            override fun onNothingSelected(parent: AdapterView<*>?) {}
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
        memberSpinner.setSelection(previousSelection.coerceAtMost(memberIds.size - 1))

        loadAndShowProgression()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadAndShowProgression() {
        if (memberIds.isEmpty()) return // spinner not populated yet - loadMemberSpinner will call back in
        val memberId = memberIds.getOrElse(memberSpinner.selectedItemPosition) { "" }
        currentAxis = ProgressionAxis.entries[axisSpinner.selectedItemPosition.coerceIn(0, ProgressionAxis.entries.size - 1)]

        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val today = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())

        val resultJson = Bridge.computeProgression(portfolioJson, memberId, currentAxis.bridgeValue, today)
        if (isBridgeError(resultJson)) {
            statusText.text = "Could not compute progression: $resultJson"
            points = emptyList()
            chart.setPoints(emptyList())
            return
        }

        val pointType = object : TypeToken<List<ProgressionPoint>>() {}.type
        points = try {
            gson.fromJson(resultJson, pointType) ?: emptyList()
        } catch (e: Exception) {
            statusText.text = "Could not read progression data: ${e.message}"
            emptyList()
        }

        if (points.isEmpty()) {
            statusText.text = "No progression data yet — this needs transactions and price history. Run \"Update Price History\" from Settings first."
            spanText.text = ""
            chart.setPoints(emptyList())
            dateText.text = ""
            investedText.text = ""
            valueText.text = ""
            gainText.text = ""
            xirrText.text = ""
            return
        }

        statusText.text = "Weekly checkpoints from first transaction to today. Drag the chart or slider to browse."
        spanText.text = spanSummary(points)
        seekBar.max = (points.size - 1).coerceAtLeast(0)
        chart.setPoints(points) // triggers onScrub for the last point, which updates the detail card
    }

    /**
     * "Jun 2024 – Aug 2026 · 2y 2m · 118 points" - gives an immediate
     * sense of scale (is this 3 months or 20 years of history?) without
     * needing to touch the chart at all, which the plain chart alone
     * didn't convey. Uses plain millisecond-difference month/year math
     * rather than java.time, since minSdk 24 doesn't have java.time
     * without core library desugaring (not currently enabled).
     */
    private fun spanSummary(pts: List<ProgressionPoint>): String {
        val first = pts.first().date
        val last = pts.last().date
        val firstDate = try { isoFormat.parse(first) } catch (e: Exception) { null }
        val lastDate = try { isoFormat.parse(last) } catch (e: Exception) { null }
        val monthLabel = SimpleDateFormat("MMM yyyy", Locale.US)
        val rangeLabel = if (firstDate != null && lastDate != null) {
            "${monthLabel.format(firstDate)} – ${monthLabel.format(lastDate)}"
        } else {
            "$first – $last"
        }

        val durationLabel = if (firstDate != null && lastDate != null) {
            val totalDays = ((lastDate.time - firstDate.time) / (1000L * 60 * 60 * 24)).toInt().coerceAtLeast(0)
            when {
                totalDays < 60 -> " · ${(totalDays / 7).coerceAtLeast(1)} weeks"
                else -> {
                    val totalMonths = totalDays / 30
                    val years = totalMonths / 12
                    val months = totalMonths % 12
                    val parts = mutableListOf<String>()
                    if (years > 0) parts.add("${years}y")
                    if (months > 0 || years == 0) parts.add("${months}m")
                    " · " + parts.joinToString(" ")
                }
            }
        } else {
            ""
        }

        return "$rangeLabel$durationLabel · ${pts.size} points"
    }

    private fun updateDetailCard(index: Int) {
        val p = points.getOrNull(index) ?: return
        val display = DisplayCurrency.entries[currencySpinner.selectedItemPosition.coerceIn(0, DisplayCurrency.entries.size - 1)]

        dateText.text = p.date
        valueText.text = ProgressionCurrency.format(
            ProgressionCurrency.convert(p.value, display, currentAxis, p)
        )

        val gainConverted = ProgressionCurrency.convert(p.gain, display, currentAxis, p)
        gainText.text = ProgressionCurrency.formatSigned(gainConverted) +
            String.format(Locale.getDefault(), "  (%.1f%%)", p.gainPercent)
        gainText.setTextColor(
            androidx.core.content.ContextCompat.getColor(
                this, if (p.gain >= 0) R.color.colorGain else R.color.colorLoss
            )
        )

        investedText.text = "Invested: " + ProgressionCurrency.format(
            ProgressionCurrency.convert(p.invested, display, currentAxis, p)
        )
        xirrText.text = if (p.hasXIRR) {
            String.format(Locale.getDefault(), "XIRR: %.1f%%", p.xirr)
        } else {
            "XIRR: not available for this point"
        }
    }
}
