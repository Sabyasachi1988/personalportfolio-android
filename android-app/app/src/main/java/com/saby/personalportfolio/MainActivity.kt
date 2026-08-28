package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.ImageButton
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import com.google.android.material.floatingactionbutton.FloatingActionButton
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.util.Locale
import java.util.concurrent.Executors

class MainActivity : AppCompatActivity() {

    /** The 3 views that make up one period-gain chip (Day/1 Year/Calendar Year) - see bindPeriodGainLine. */
    private data class PeriodGainChipViews(val container: View, val amount: TextView, val percent: TextView)

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())
    private lateinit var totalValue: TextView
    private lateinit var gainLine: TextView
    private lateinit var xirrLine: TextView
    private lateinit var holdingsCountLine: TextView
    private lateinit var periodGainsRow: View
    private lateinit var periodGainDayChip: PeriodGainChipViews
    private lateinit var periodGainYearChip: PeriodGainChipViews
    private lateinit var periodGainCalendarYearChip: PeriodGainChipViews
    private lateinit var donutMarketCap: DonutChartView
    private lateinit var donutLegendMarketCap: DonutLegendView
    private lateinit var donutOrigin: DonutChartView
    private lateinit var donutLegendOrigin: DonutLegendView
    private lateinit var donutClass: DonutChartView
    private lateinit var donutLegendClass: DonutLegendView
    private lateinit var memberSpinner: Spinner
    private lateinit var refreshButton: ImageButton
    private lateinit var incognitoButton: ImageButton
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
        periodGainsRow = findViewById(R.id.statsCardPeriodGainsRow)
        periodGainDayChip = PeriodGainChipViews(
            container = findViewById(R.id.statsCardPeriodGainDayChip),
            amount = findViewById(R.id.statsCardPeriodGainDayAmount),
            percent = findViewById(R.id.statsCardPeriodGainDayPercent)
        )
        periodGainYearChip = PeriodGainChipViews(
            container = findViewById(R.id.statsCardPeriodGainYearChip),
            amount = findViewById(R.id.statsCardPeriodGainYearAmount),
            percent = findViewById(R.id.statsCardPeriodGainYearPercent)
        )
        periodGainCalendarYearChip = PeriodGainChipViews(
            container = findViewById(R.id.statsCardPeriodGainCalendarYearChip),
            amount = findViewById(R.id.statsCardPeriodGainCalendarYearAmount),
            percent = findViewById(R.id.statsCardPeriodGainCalendarYearPercent)
        )
        periodGainsRow.visibility = View.VISIBLE
        donutMarketCap = findViewById(R.id.dashboardDonutMarketCap)
        donutLegendMarketCap = findViewById(R.id.dashboardDonutLegendMarketCap)
        donutOrigin = findViewById(R.id.dashboardDonutOrigin)
        donutLegendOrigin = findViewById(R.id.dashboardDonutLegendOrigin)
        donutClass = findViewById(R.id.dashboardDonutClass)
        donutLegendClass = findViewById(R.id.dashboardDonutLegendClass)
        memberSpinner = findViewById(R.id.dashboardMemberSpinner)
        refreshButton = findViewById(R.id.dashboardRefreshButton)
        incognitoButton = findViewById(R.id.dashboardIncognitoButton)
        updateIncognitoIcon()
        incognitoButton.setOnClickListener {
            IncognitoMode.setEnabled(this, !IncognitoMode.isEnabled)
            updateIncognitoIcon()
            loadDashboard()
        }
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
        findViewById<ImageButton>(R.id.dashboardSettingsButton).setOnClickListener {
            startActivity(Intent(this, SettingsActivity::class.java))
        }
        refreshButton.setOnClickListener { refreshAllPrices() }

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
            String.format(Locale.getDefault(), "%s: %.2f%%", label, percent),
            android.widget.Toast.LENGTH_SHORT
        )
        donutToast = toast
        toast.show()
    }

    private fun loadMemberSpinner() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
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
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val today = java.text.SimpleDateFormat("yyyy-MM-dd", Locale.US).format(java.util.Date())

        // ONE combined bridge call instead of 7 separate ones (Holdings,
        // XIRR, rolling Day/Year gains, Calendar Year gain, and all 3
        // allocation donuts) - see bridge.DashboardResult's doc comment.
        // This was the confirmed cause of Dashboard specifically feeling
        // laggier than other screens: each of the 7 calls independently
        // re-parsed the same portfolio JSON on the Go side, regardless of
        // PortfolioLoadCache already avoiding re-parsing on the Kotlin side.
        val resultJson = Bridge.computeDashboard(portfolioJson, memberId, today)
        val result: DashboardResult? = try {
            gson.fromJson(resultJson, DashboardResult::class.java)
        } catch (e: Exception) {
            null
        }
        val holdings = result?.holdings ?: emptyList()

        if (holdings.isEmpty()) {
            totalValue.text = "No holdings yet"
            gainLine.text = "Tap + below to import your first CAS statement"
            gainLine.setTextColor(androidx.core.content.ContextCompat.getColor(this, R.color.colorNeutral))
            xirrLine.text = ""
            holdingsCountLine.text = ""
            periodGainsRow.visibility = View.GONE
            donutMarketCap.setSlices(emptyList())
            donutLegendMarketCap.setSlices(emptyList())
            donutOrigin.setSlices(emptyList())
            donutLegendOrigin.setSlices(emptyList())
            donutClass.setSlices(emptyList())
            donutLegendClass.setSlices(emptyList())
            return
        }
        periodGainsRow.visibility = View.VISIBLE

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
                Locale.getDefault(), "%s (%.2f%%) overall",
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

        xirrLine.text = if (result?.hasXIRR == true) {
            String.format(Locale.getDefault(), "Portfolio XIRR: %.2f%%", result.xirr)
        } else {
            ""
        }

        showPeriodGains(result?.rollingGains ?: emptyList(), result?.calendarYearGain)

        holdingsCountLine.text = "${holdings.size} holding(s)"

        setDonut(donutMarketCap, donutLegendMarketCap, result?.marketCapSlices ?: emptyList())
        setDonut(donutOrigin, donutLegendOrigin, result?.originSlices ?: emptyList())
        setDonut(donutClass, donutLegendClass, result?.classSlices ?: emptyList())
    }

    /**
     * Populates the Day / 1-Year (rolling) / Calendar Year (YTD) chips
     * under the total-value line - see finance.ComputePeriodGains and
     * finance.ComputeCalendarYearGain's doc comments (Go) for exactly
     * what these figures mean: market movement only, NET of any money
     * added or withdrawn during each window, so a fresh SIP doesn't
     * inflate the figure. This is deliberately different from
     * statsCardGain above it, which is the since-inception total
     * (Value - Invested) - the two numbers answer different questions
     * and will often disagree in sign, that's expected, not a bug.
     *
     * Rupee amount is shown as the PRIMARY figure (percent alongside,
     * smaller/secondary) - more immediately readable at a glance than a
     * bare percent, especially for Day, where the percent alone barely
     * moves. Each chip's own background tints green/red/neutral by its
     * own gain sign, not just the text - a loss day or a down year reads
     * as a red chip, not just red text on a plain background.
     */
    private fun showPeriodGains(rollingGains: List<PeriodGain>, calendarYearGain: PeriodGain?) {
        val byLabel = rollingGains.associateBy { it.label }
        bindPeriodGainLine(periodGainDayChip, byLabel["Day"])
        bindPeriodGainLine(periodGainYearChip, byLabel["Year"])
        bindPeriodGainLine(periodGainCalendarYearChip, calendarYearGain)
    }

    private fun bindPeriodGainLine(chip: PeriodGainChipViews, g: PeriodGain?) {
        val bgColorRes: Int
        val textColorRes: Int
        if (g == null || !g.hasData) {
            bgColorRes = R.color.colorNeutralBg
            textColorRes = R.color.colorNeutral
            chip.amount.text = "—"
            chip.percent.text = "Not enough history"
            chip.container.setOnClickListener(null)
        } else {
            val isGain = g.gain >= 0
            bgColorRes = if (isGain) R.color.colorGainBg else R.color.colorLossBg
            textColorRes = if (isGain) R.color.colorGain else R.color.colorLoss
            // Rounded to the nearest rupee - see IndianCurrencyFormatter's
            // default decimals, changed app-wide for exactly this reason:
            // paise-level precision on a summary figure like this adds
            // visual noise without adding real information.
            chip.amount.text = IndianCurrencyFormatter.formatSigned(g.gain, decimals = 0)
            chip.percent.text = String.format(Locale.getDefault(), "%+.2f%%", g.percent)
            chip.container.setOnClickListener {
                // A dialog, not a Toast - the earlier Toast-based version
                // of this message was a confirmed real bug: modern
                // Android compresses a long Toast into a single
                // truncated line ("...thi..."), and the newly-added date
                // range sat at the END of that string, past the
                // truncation cutoff, so it never actually became visible
                // - the exact information this was built to surface.
                // AlertDialog always shows the full text, no truncation.
                //
                // Shows the ACTUAL dates being compared, not just what
                // the figure means - added after two separate real bugs
                // in "Day"'s anchor-date logic each produced a
                // plausible-looking but wrong number with no way to
                // check from the UI which dates were actually behind
                // it. Start==End (e.g. Calendar Year on Jan 1st itself)
                // is worth calling out plainly rather than showing a
                // zero-width range that looks like a typo.
                val rangeText = if (g.startDate.isNotBlank() && g.endDate.isNotBlank()) {
                    if (g.startDate == g.endDate) {
                        "As of ${g.endDate} - nothing to compare against yet."
                    } else {
                        "Comparing ${g.startDate} → ${g.endDate}."
                    }
                } else {
                    ""
                }
                android.app.AlertDialog.Builder(this)
                    .setTitle(g.label)
                    .setMessage("Market movement only - excludes any money added or withdrawn during this period.\n\n$rangeText")
                    .setPositiveButton("OK", null)
                    .show()
            }
        }
        val bgColor = androidx.core.content.ContextCompat.getColor(this, bgColorRes)
        (chip.container.background?.mutate() as? android.graphics.drawable.GradientDrawable)?.setColor(bgColor)
        val textColor = androidx.core.content.ContextCompat.getColor(this, textColorRes)
        chip.amount.setTextColor(textColor)
        chip.percent.setTextColor(textColor)
    }

    private fun updateIncognitoIcon() {
        incognitoButton.setImageResource(if (IncognitoMode.isEnabled) R.drawable.ic_eye_off else R.drawable.ic_eye)
    }

    private fun setDonut(chart: DonutChartView, legend: DonutLegendView, slices: List<AllocationSlice>) {
        val chartSlices = slices.map {
            DonutChartView.Slice(it.label, it.percent.toFloat(), CapSegmentColors.forLabel(this, it.label))
        }
        chart.setSlices(chartSlices)
        legend.setSlices(chartSlices)
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    /**
     * Refreshes today's price for EVERYTHING in one tap - both AMFI-NAV
     * mutual funds and Yahoo-symbol ETFs/stocks, unlike Holdings' own
     * "Refresh Prices" button, which only ever covered the AMFI side.
     * Lightweight (today's price only), not the full multi-year history
     * fetch Settings → Update Price History does.
     */
    private fun refreshAllPrices() {
        refreshButton.isEnabled = false

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                var portfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(portfolioJson)) {
                    mainThread.post { failRefresh("Failed to load portfolio: $portfolioJson") }
                    return@execute
                }

                val amfiResult = Bridge.refreshAmfiPrices(portfolioJson)
                var amfiMatched = 0
                if (isBridgeError(amfiResult)) {
                    mainThread.post { failRefresh("AMFI refresh failed: $amfiResult") }
                    return@execute
                }
                val amfiParsed = try {
                    gson.fromJson(amfiResult, RefreshAmfiResult::class.java)
                } catch (e: Exception) {
                    mainThread.post { failRefresh("AMFI refresh returned unexpected data: ${e.message}") }
                    return@execute
                }
                portfolioJson = gson.toJson(amfiParsed.portfolio)
                amfiMatched = amfiParsed.matchedCount

                val symbolResult = Bridge.refreshSymbolPrices(portfolioJson)
                var symbolMatched = 0
                if (!isBridgeError(symbolResult)) {
                    val symbolParsed = try {
                        gson.fromJson(symbolResult, RefreshSymbolResult::class.java)
                    } catch (e: Exception) {
                        null
                    }
                    if (symbolParsed != null) {
                        portfolioJson = gson.toJson(symbolParsed.portfolio)
                        symbolMatched = symbolParsed.matchedCount
                    }
                }

                val saveResult = Bridge.savePortfolio(portfolioPath, portfolioJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { failRefresh("Failed to save refreshed prices: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    refreshButton.isEnabled = true
                    Toast.makeText(
                        this,
                        "Refreshed $amfiMatched fund(s), $symbolMatched ETF/stock(s)",
                        Toast.LENGTH_LONG
                    ).show()
                    loadDashboard()
                }
            } catch (e: Exception) {
                mainThread.post { failRefresh("Refresh failed: ${e.message}") }
            }
        }
    }

    private fun failRefresh(message: String) {
        refreshButton.isEnabled = true
        Toast.makeText(this, message, Toast.LENGTH_LONG).show()
    }
}

private data class RefreshAmfiResult(
    val matchedCount: Int,
    val portfolio: com.google.gson.JsonObject
)

private data class RefreshSymbolResult(
    val matchedCount: Int,
    val failures: List<String>?,
    val portfolio: com.google.gson.JsonObject
)

// Mirrors finance.PeriodGain - see ComputePeriodGains' doc comment in
// Go for exactly what Gain/Percent mean (net of contributions during
// the window) and why.
data class PeriodGain(
    @com.google.gson.annotations.SerializedName("Label") val label: String,
    @com.google.gson.annotations.SerializedName("Gain") val gain: Double,
    @com.google.gson.annotations.SerializedName("Percent") val percent: Double,
    @com.google.gson.annotations.SerializedName("HasData") val hasData: Boolean,
    @com.google.gson.annotations.SerializedName("StartDate") val startDate: String = "",
    @com.google.gson.annotations.SerializedName("EndDate") val endDate: String = ""
)
