package com.saby.personalportfolio

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.View
import android.widget.SeekBar
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.appcompat.widget.PopupMenu
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.Executors

class ProgressionActivity : AppCompatActivity() {

    private val gson = Gson()
    // Loading a progression series recomputes every checkpoint from full
    // transaction/price history (see finance.computeProgressionPoint) -
    // benchmarked at real-world scale, the full weekly range alone can
    // run into the hundreds of milliseconds today and would get far
    // worse as years of history accumulate. This screen was the one
    // place in the app still doing that on the main thread - moved to
    // match the backgroundExecutor+mainThread.post pattern already used
    // everywhere else (Holdings, CapComposition, Settings, etc.).
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    // Debounces the daily-overlay fetch triggered by pinch-zooming past
    // dailyZoomThresholdDays - without this, a continuous pinch gesture
    // (which fires onWindowChanged many times per second) would trigger
    // a Bridge call on every single frame.
    private val dailyModeHandler = Handler(Looper.getMainLooper())
    private var pendingDailyModeRunnable: Runnable? = null
    private val dailyZoomDebounceMillis = 400L
    private val dailyZoomThresholdDays = 180

    private lateinit var memberTab: TextView
    private lateinit var axisTab: TextView
    private lateinit var currencyTab: TextView
    private lateinit var statusText: TextView
    private lateinit var dateText: TextView
    private lateinit var investedText: TextView
    private lateinit var valueText: TextView
    private lateinit var gainText: TextView
    private lateinit var xirrText: TextView
    private lateinit var chart: ProgressionChartView
    private lateinit var seekBar: SeekBar
    private lateinit var resetZoomButton: TextView

    // Index 0 is always "All (family)" (empty memberID); indices 1.. map 1:1 with memberIds - same convention as HoldingsActivity.
    private var memberIds: List<String> = emptyList()
    private var memberLabels: List<String> = emptyList()

    // The currently-DISPLAYED series (whichever of weeklySpine or a
    // daily-overlay range is active) - what updateDetailCard indexes
    // into. weeklySpine is always the full weekly range for the current
    // member/axis/fund selection, kept around so "Reset zoom" can return
    // to it even after a daily overlay has replaced `points` entirely.
    private var points: List<ProgressionPoint> = emptyList()
    private var weeklySpine: List<ProgressionPoint> = emptyList()
    private var inDailyMode = false
    // The [start, end] dates actually covered by the currently-loaded
    // daily dataset - NOT necessarily the same as the currently-visible
    // window (that dataset is usually padded wider than any one window
    // into it - see fetchAndSwitchToDailyRange's padding). Used by
    // onChartWindowChanged to tell "still panning within what's already
    // loaded, nothing to do" apart from "panned past the edge of loaded
    // data, need a new fetch" - see that function's doc comment for the
    // bug this fixes (panning used to silently get stuck at this exact
    // edge, with no mechanism to notice and fetch further).
    private var dailyDataStart: String? = null
    private var dailyDataEnd: String? = null

    private var currentAxis: ProgressionAxis = ProgressionAxis.WHOLE_PORTFOLIO

    // When set, the picker is in "specific fund" mode: the axis
    // selection is ignored and ComputeAssetProgression is used instead
    // of ComputeProgression (see loadAndShowProgression). Null means the
    // normal axis-based (whole portfolio / equity split) mode.
    private var selectedAssetId: String? = null
    // When set, the picker is in "fund group" mode - browsing several
    // same-labeled funds' COMBINED growth story (see
    // finance.ComputeGroupProgression and store.Asset.GroupLabel's doc
    // comment, e.g. several different-AMC "Nifty 50" funds). Mutually
    // exclusive with selectedAssetId - selecting one always clears the
    // other, same convention as axis-vs-asset mode already had.
    private var selectedGroupLabel: String? = null
    private var assets: List<AssetSummary> = emptyList()
    // Distinct, non-blank GroupLabel values present among `assets`, in
    // first-seen order - populated in loadAssetList, shown in the
    // picker only when at least one fund has actually been labeled
    // (see Settings → Manage Fund Groups).
    private var groupLabels: List<String> = emptyList()
    // AssetID -> Account currency, so "Native" currency display resolves
    // correctly for a specific foreign-brokerage fund in fund mode (see
    // loadAndShowProgression's use of this).
    private var accountCurrencyByAssetId: Map<String, String> = emptyMap()

    private var selectedMemberIndex = 0
    private var selectedAxisIndex = 0
    private var selectedCurrencyIndex = 0

    // Guards against the chart's onScrub and the SeekBar's listener
    // re-triggering each other in a feedback loop when one drives the
    // other's position programmatically.
    private var syncingScrub = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_progression)

        memberTab = findViewById(R.id.progressionMemberTab)
        axisTab = findViewById(R.id.progressionAxisTab)
        currencyTab = findViewById(R.id.progressionCurrencyTab)
        statusText = findViewById(R.id.progressionStatusText)
        dateText = findViewById(R.id.statsCardDate)
        investedText = findViewById(R.id.statsCardInvested)
        valueText = findViewById(R.id.statsCardValue)
        gainText = findViewById(R.id.statsCardGain)
        xirrText = findViewById(R.id.statsCardXirr)
        chart = findViewById(R.id.progressionChart)
        seekBar = findViewById(R.id.progressionSeekBar)
        resetZoomButton = findViewById(R.id.progressionResetZoomButton)

        // Progression's card shows Date and Invested, which the shared
        // card hides by default (only this screen browses point-in-time,
        // so only this screen needs a Date line).
        dateText.visibility = View.VISIBLE
        investedText.visibility = View.VISIBLE

        axisTab.text = ProgressionAxis.entries[selectedAxisIndex].label
        currencyTab.text = DisplayCurrency.entries[selectedCurrencyIndex].label

        memberTab.setOnClickListener { showMemberPicker() }
        axisTab.setOnClickListener { showAxisOrFundPicker() }
        currencyTab.setOnClickListener { showCurrencyPicker() }

        chart.onScrub = { index ->
            if (!syncingScrub) {
                syncingScrub = true
                seekBar.progress = index
                syncingScrub = false
            }
            updateDetailCard(index)
        }
        chart.onZoomChanged = { zoomed ->
            resetZoomButton.visibility = if (zoomed || inDailyMode) View.VISIBLE else View.GONE
        }
        chart.onZoomOutBeyondBounds = {
            // Only meaningful while showing a bounded daily-zoom window -
            // if we're already on the weekly spine, this IS the outer
            // bound and there's genuinely nothing wider to fall back to.
            if (inDailyMode) {
                resetToWeeklyView()
            }
        }
        chart.onWindowChanged = { startDate, endDate, spanDays ->
            onChartWindowChanged(startDate, endDate, spanDays)
        }
        resetZoomButton.setOnClickListener { resetToWeeklyView() }
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
        loadMemberList()
        loadAssetList()
    }

    override fun onPause() {
        super.onPause()
        // Don't let a debounced daily-fetch fire (and touch views) after
        // the screen has been navigated away from.
        pendingDailyModeRunnable?.let { dailyModeHandler.removeCallbacks(it) }
    }

    private fun showMemberPicker() {
        val popup = PopupMenu(this, memberTab)
        memberLabels.forEachIndexed { index, label -> popup.menu.add(0, index, index, label) }
        popup.setOnMenuItemClickListener { item ->
            selectedMemberIndex = item.itemId
            memberTab.text = memberLabels.getOrElse(selectedMemberIndex) { "All (family)" }
            loadAndShowProgression()
            true
        }
        popup.show()
    }

    /**
     * One combined picker: the 4 whole-portfolio/equity axes, then every
     * individual fund, then every fund GROUP (if any funds have been
     * labeled via Settings → Manage Fund Groups) - so browsing a single
     * holding's own growth story, or a consolidated group's (e.g.
     * several different-AMC "Nifty 50" funds combined), is one tap away
     * from where you'd naturally look for "what am I viewing", rather
     * than a separate control competing for the same limited row width.
     */
    private fun showAxisOrFundPicker() {
        val popup = PopupMenu(this, axisTab)
        ProgressionAxis.entries.forEachIndexed { index, axis -> popup.menu.add(0, index, index, axis.label) }

        val fundIdBase = ProgressionAxis.entries.size
        if (assets.isNotEmpty()) {
            val header = popup.menu.add(0, fundIdBase, fundIdBase, "── Specific fund ──")
            header.isEnabled = false
            assets.forEachIndexed { i, asset ->
                val itemId = fundIdBase + 1 + i
                popup.menu.add(0, itemId, itemId, FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" })
            }
        }

        val groupIdBase = fundIdBase + 1 + assets.size
        if (groupLabels.isNotEmpty()) {
            val header = popup.menu.add(0, groupIdBase, groupIdBase, "── Fund group ──")
            header.isEnabled = false
            groupLabels.forEachIndexed { i, label ->
                val itemId = groupIdBase + 1 + i
                popup.menu.add(0, itemId, itemId, label)
            }
        }

        popup.setOnMenuItemClickListener { item ->
            val id = item.itemId
            if (id < ProgressionAxis.entries.size) {
                selectedAssetId = null
                selectedGroupLabel = null
                selectedAxisIndex = id
                axisTab.text = ProgressionAxis.entries[id].label
            } else if (id in (fundIdBase + 1) until groupIdBase) {
                val asset = assets.getOrNull(id - fundIdBase - 1)
                if (asset != null) {
                    selectedAssetId = asset.id
                    selectedGroupLabel = null
                    axisTab.text = FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" }
                }
            } else if (id > groupIdBase) {
                val label = groupLabels.getOrNull(id - groupIdBase - 1)
                if (label != null) {
                    selectedGroupLabel = label
                    selectedAssetId = null
                    axisTab.text = label
                }
            }
            loadAndShowProgression()
            true
        }
        popup.show()
    }

    private fun showCurrencyPicker() {
        val popup = PopupMenu(this, currencyTab)
        DisplayCurrency.entries.forEachIndexed { index, currency -> popup.menu.add(0, index, index, currency.label) }
        popup.setOnMenuItemClickListener { item ->
            selectedCurrencyIndex = item.itemId
            currencyTab.text = DisplayCurrency.entries[selectedCurrencyIndex].label
            updateDetailCard(seekBar.progress)
            true
        }
        popup.show()
    }

    private fun loadMemberList() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val membersJson = Bridge.listMembers(portfolioJson)

        val memberType = object : TypeToken<List<Member>>() {}.type
        val members: List<Member> = try {
            gson.fromJson(membersJson, memberType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }

        memberIds = listOf("") + members.map { it.id }
        memberLabels = listOf("All (family)") + members.map { it.name }
        selectedMemberIndex = selectedMemberIndex.coerceAtMost(memberIds.size - 1).coerceAtLeast(0)
        memberTab.text = memberLabels.getOrElse(selectedMemberIndex) { "All (family)" }

        loadAndShowProgression()
    }

    private fun loadAssetList() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val snapshot: PortfolioManualEntrySnapshot = try {
            gson.fromJson(portfolioJson, PortfolioManualEntrySnapshot::class.java)
        } catch (e: Exception) {
            PortfolioManualEntrySnapshot(emptyList(), emptyList(), emptyList())
        }
        assets = snapshot.assets.orEmpty()
        val currencyByAccountId = snapshot.accounts.orEmpty().associate { it.id to it.currency }
        accountCurrencyByAssetId = assets.associate { it.id to (currencyByAccountId[it.accountId] ?: "INR") }
        groupLabels = assets.mapNotNull { it.groupLabel.takeIf { label -> label.isNotBlank() } }.distinct()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadAndShowProgression() {
        if (memberIds.isEmpty()) return // list not populated yet - loadMemberList will call back in
        pendingDailyModeRunnable?.let { dailyModeHandler.removeCallbacks(it) }
        inDailyMode = false
        dailyDataStart = null
        dailyDataEnd = null

        val assetId = selectedAssetId
        val groupLabel = selectedGroupLabel
        val memberId = memberIds.getOrElse(selectedMemberIndex) { "" }
        val axisForFetch = ProgressionAxis.entries[selectedAxisIndex.coerceIn(0, ProgressionAxis.entries.size - 1)]

        statusText.text = "Loading…"

        backgroundExecutor.execute {
            // Every other screen in this app wraps its background Bridge
            // calls in try/catch (CapCompositionActivity, HoldingsActivity,
            // etc.) - this one didn't, which was a real gap: an uncaught
            // exception on ANY thread crashes the whole Android process by
            // default, no per-thread isolation. That's consistent with
            // exactly what was reported - the app abruptly disappearing
            // when opening Progression, and needing to unlock again right
            // after even with a grace period configured, since a process
            // crash wipes AppLockManager's in-memory state regardless of
            // that setting. This doesn't identify what specifically threw
            // (if anything still does after this fix, the message below
            // will say so precisely, rather than the whole app vanishing
            // with no diagnostic trail at all).
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                val portfolioJson = Bridge.loadPortfolio(portfolioPath)
                val cachePath = PortfolioStorage.progressionCachePath(this)
                val today = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())

                val resolvedAxis: ProgressionAxis
                val resultJson: String
                if (groupLabel != null) {
                    // Group mode: several same-labeled funds combined - see
                    // finance.ComputeGroupProgression. Currency-native
                    // resolution defaults to INR (WHOLE_PORTFOLIO), same
                    // reasoning as fund mode's INR default below - a fund
                    // group is overwhelmingly likely to be same-currency in
                    // practice (e.g. several Indian "Nifty 50" funds).
                    resolvedAxis = ProgressionAxis.WHOLE_PORTFOLIO
                    resultJson = Bridge.computeGroupProgression(portfolioJson, memberId, groupLabel, today, cachePath)
                } else if (assetId != null) {
                    // Fund mode: a single fund is already fully scoped, so the
                    // axis/member pickers don't apply - the currency picker's
                    // "Native" option still needs to know whether this
                    // particular fund is INR or foreign, so the axis is set
                    // here purely as ProgressionCurrency's existing INR/CAD
                    // switch (see ProgressionCurrency.nativeCurrencyFor), not
                    // because this is really an "International Equity" view.
                    resolvedAxis = if (accountCurrencyByAssetId[assetId] == "INR") {
                        ProgressionAxis.WHOLE_PORTFOLIO
                    } else {
                        ProgressionAxis.INTERNATIONAL_EQUITY
                    }
                    resultJson = Bridge.computeAssetProgression(portfolioJson, assetId, today, cachePath)
                } else {
                    resolvedAxis = axisForFetch
                    resultJson = Bridge.computeProgression(portfolioJson, memberId, resolvedAxis.bridgeValue, today, cachePath)
                }

                mainThread.post {
                    // A try/catch wrapped only around the code that
                    // SCHEDULES this block (see the outer try above)
                    // does NOT protect the block's own contents - this
                    // Runnable executes later, on its own call stack,
                    // once the main thread's Looper gets to it. If
                    // applyLoadedProgression (or anything it triggers
                    // synchronously - chart.setPoints fires onScrub,
                    // which updates the detail card) throws, that's a
                    // separate crash the outer catch below never sees.
                    // Confirmed the hard way: the outer try/catch alone
                    // did NOT stop the crash - Android's own generic
                    // crash dialog still appeared instead of this
                    // screen's error text, which is exactly what a
                    // throw happening here, unprotected, would look
                    // like.
                    try {
                        currentAxis = resolvedAxis
                        applyLoadedProgression(resultJson, isFundMode = assetId != null, isGroupMode = groupLabel != null, groupLabel = groupLabel)
                    } catch (e: Exception) {
                        statusText.text = "Could not display progression: ${e.javaClass.simpleName}: ${e.message}"
                    }
                }
            } catch (e: Exception) {
                mainThread.post {
                    statusText.text = "Could not load progression: ${e.javaClass.simpleName}: ${e.message}"
                }
            }
        }
    }

    private fun applyLoadedProgression(resultJson: String, isFundMode: Boolean, isGroupMode: Boolean = false, groupLabel: String? = null) {
        if (isBridgeError(resultJson)) {
            statusText.text = "Could not compute progression: $resultJson"
            points = emptyList()
            weeklySpine = emptyList()
            chart.setPoints(emptyList())
            return
        }

        val pointType = object : TypeToken<List<ProgressionPoint>>() {}.type
        val loaded: List<ProgressionPoint> = try {
            gson.fromJson(resultJson, pointType) ?: emptyList()
        } catch (e: Exception) {
            statusText.text = "Could not read progression data: ${e.message}"
            emptyList()
        }

        if (loaded.isEmpty()) {
            statusText.text = "No progression data yet — this needs transactions and price history. Run \"Update Price History\" from Settings first."
            points = emptyList()
            weeklySpine = emptyList()
            chart.setPoints(emptyList())
            dateText.text = ""
            investedText.text = ""
            valueText.text = ""
            gainText.text = ""
            xirrText.text = ""
            return
        }

        weeklySpine = loaded
        points = loaded
        statusText.text = when {
            isGroupMode -> "Weekly checkpoints for $groupLabel (combined)"
            isFundMode -> "Weekly checkpoints for this fund"
            else -> "Weekly checkpoints"
        }
        seekBar.max = (loaded.size - 1).coerceAtLeast(0)
        chart.setPoints(loaded) // triggers onScrub for the last point, which updates the detail card
    }

    /**
     * Reacts to the chart's zoom/pan window changing by fetching real
     * daily-resolution data and swapping it in - see
     * ProgressionChartView.onWindowChanged's doc comment. Debounced so a
     * continuous pinch/pan gesture doesn't fire a Bridge call on every
     * frame.
     *
     * Triggers a NEW fetch whenever the visible window has reached
     * (touches) either edge of whatever daily data is already loaded
     * (dailyDataStart/End) - not just the first time daily mode is
     * entered. The previous version of this check compared startDate/
     * endDate against loadedStart/loadedEnd with `>=`/`<=` and skipped
     * the fetch when "contained" - but startDate/endDate come from
     * points[windowStart]/points[windowEnd], and windowStart/windowEnd
     * are ALWAYS clamped by the chart view to stay inside the currently-
     * loaded array (see ProgressionChartView's window clamping) - so
     * that comparison was true BY CONSTRUCTION, every single time, and
     * could never detect "panned/zoomed to the edge of what's loaded".
     * In practice this meant the very first daily-mode fetch (crossing
     * the 180-day threshold from weekly) was the ONLY fetch that ever
     * happened - any further pan or zoom-in within that one 6-month
     * window silently never widened it, which is exactly what pinned a
     * person at that window's edges with nowhere further to go.
     * Comparing for EQUALITY at either edge instead correctly detects
     * "the window has been pushed all the way to what's loaded" (the
     * chart can't report a date past that, so equality is the true
     * signal, not `>`/`<`), and re-fetches a fresh padded range
     * recentered on the current position - which is also why the
     * padding in paddedDailyRange matters: it's what gives the NEXT pan
     * real room before this fires again.
     */
    private fun onChartWindowChanged(startDate: String, endDate: String, spanDays: Int) {
        pendingDailyModeRunnable?.let { dailyModeHandler.removeCallbacks(it) }
        if (weeklySpine.isEmpty()) return
        if (startDate.isBlank() || endDate.isBlank()) return
        if (spanDays !in 1..dailyZoomThresholdDays) return

        if (inDailyMode) {
            val loadedStart = dailyDataStart
            val loadedEnd = dailyDataEnd
            // Plain string comparison is safe here - every date is
            // "yyyy-MM-dd", where lexicographic order already matches
            // chronological order.
            val atLeftEdge = loadedStart != null && startDate <= loadedStart
            val atRightEdge = loadedEnd != null && endDate >= loadedEnd
            if (loadedStart != null && loadedEnd != null && !atLeftEdge && !atRightEdge) {
                return // comfortably inside what's already loaded, no need for a wider fetch
            }
        }

        val runnable = Runnable { fetchAndSwitchToDailyRange(startDate, endDate) }
        pendingDailyModeRunnable = runnable
        dailyModeHandler.postDelayed(runnable, dailyZoomDebounceMillis)
    }

    /**
     * Widens a requested daily-fetch range well beyond exactly what's
     * currently visible (50% padding on each side), so panning within
     * the loaded data has real room to move before hitting its edge and
     * needing another fetch. Fetching EXACTLY the visible window (the
     * previous behavior) left zero slack - the instant you panned at
     * all, you were already at the loaded data's edge, which is what
     * "gets stuck almost immediately" looked like. Capped so the total
     * padded width never exceeds dailyZoomThresholdDays (the
     * benchmarked-safe daily-fetch size - see internal/benchmark), and
     * never requests dates after today.
     */
    private fun paddedDailyRange(startDate: String, endDate: String): Pair<String, String> {
        val fmt = SimpleDateFormat("yyyy-MM-dd", Locale.US)
        val start = try { fmt.parse(startDate) } catch (e: Exception) { null } ?: return startDate to endDate
        val end = try { fmt.parse(endDate) } catch (e: Exception) { null } ?: return startDate to endDate

        val spanMillis = (end.time - start.time).coerceAtLeast(0L)
        val paddingMillis = spanMillis / 2
        var paddedStartMillis = start.time - paddingMillis
        var paddedEndMillis = end.time + paddingMillis

        val maxWidthMillis = dailyZoomThresholdDays.toLong() * 24 * 60 * 60 * 1000
        if (paddedEndMillis - paddedStartMillis > maxWidthMillis) {
            val center = (start.time + end.time) / 2
            paddedStartMillis = center - maxWidthMillis / 2
            paddedEndMillis = center + maxWidthMillis / 2
        }

        val todayMillis = Date().time
        if (paddedEndMillis > todayMillis) {
            val overshoot = paddedEndMillis - todayMillis
            paddedEndMillis = todayMillis
            paddedStartMillis -= overshoot // give the freed-up room to the start side instead of just losing it
        }

        return fmt.format(Date(paddedStartMillis)) to fmt.format(Date(paddedEndMillis))
    }

    private fun fetchAndSwitchToDailyRange(requestedStartDate: String, requestedEndDate: String) {
        val assetId = selectedAssetId
        val groupLabel = selectedGroupLabel
        val memberId = memberIds.getOrElse(selectedMemberIndex) { "" }
        val axisForFetch = currentAxis
        val (startDate, endDate) = paddedDailyRange(requestedStartDate, requestedEndDate)

        backgroundExecutor.execute {
            // Same reasoning as loadAndShowProgression's try/catch - an
            // uncaught exception here (this fires on every pinch-zoom
            // gesture past the daily threshold, debounced) would also
            // crash the whole app. Failing silently here (not even an
            // error message) is deliberate and matches the existing
            // isBridgeError early-return just below - this is a
            // background enhancement to an already-showing weekly view,
            // not a required load, so the right failure behavior is
            // "stay on what's already on screen", same as it already was
            // for a Bridge-level error.
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                val portfolioJson = Bridge.loadPortfolio(portfolioPath)
                val resultJson = if (groupLabel != null) {
                    Bridge.computeGroupProgressionDailyRange(portfolioJson, memberId, groupLabel, startDate, endDate)
                } else if (assetId != null) {
                    Bridge.computeAssetProgressionDailyRange(portfolioJson, assetId, startDate, endDate)
                } else {
                    Bridge.computeProgressionDailyRange(portfolioJson, memberId, axisForFetch.bridgeValue, startDate, endDate)
                }

                if (isBridgeError(resultJson)) return@execute // silently keep the weekly view - this is a background enhancement, not a required load
                val pointType = object : TypeToken<List<ProgressionPoint>>() {}.type
                val dailyPoints: List<ProgressionPoint> = try {
                    gson.fromJson(resultJson, pointType) ?: emptyList()
                } catch (e: Exception) {
                    emptyList()
                }
                if (dailyPoints.size < 2) return@execute // not enough to show a meaningful chart - stay on weekly

                mainThread.post {
                    // Same gap as loadAndShowProgression's mainThread.post
                    // - protecting the code that SCHEDULES this block does
                    // not protect what's INSIDE it once it actually runs.
                    try {
                        inDailyMode = true
                        points = dailyPoints
                        // Recorded from the ACTUAL returned data, not the
                        // padded request - the backend may have clamped
                        // to available history or to today, and this
                        // must reflect what's truly loaded for
                        // onChartWindowChanged's boundary check to be
                        // correct.
                        dailyDataStart = dailyPoints.first().date
                        dailyDataEnd = dailyPoints.last().date
                        seekBar.max = (dailyPoints.size - 1).coerceAtLeast(0)
                        chart.setPoints(dailyPoints)
                        resetZoomButton.visibility = View.VISIBLE
                        statusText.text = when {
                            groupLabel != null -> "Daily detail for $groupLabel (combined)"
                            assetId != null -> "Daily detail for this fund"
                            else -> "Daily detail"
                        }
                    } catch (e: Exception) {
                        // Same "stay on what's already on screen" reasoning
                        // as the isBridgeError early-return above - this is
                        // a background enhancement, not a required load.
                    }
                }
            } catch (e: Exception) {
                // Stay on the weekly view, same as a Bridge-level error above.
            }
        }
    }

    /** "Reset zoom" while in daily mode means "back to the weekly spine", not just re-fitting the currently-loaded daily range. */
    private fun resetToWeeklyView() {
        pendingDailyModeRunnable?.let { dailyModeHandler.removeCallbacks(it) }
        if (inDailyMode) {
            inDailyMode = false
            dailyDataStart = null
            dailyDataEnd = null
            points = weeklySpine
            seekBar.max = (weeklySpine.size - 1).coerceAtLeast(0)
            chart.setPoints(weeklySpine)
            statusText.text = when {
                selectedGroupLabel != null -> "Weekly checkpoints for ${selectedGroupLabel} (combined)"
                selectedAssetId != null -> "Weekly checkpoints for this fund"
                else -> "Weekly checkpoints"
            }
        } else {
            chart.resetZoom()
        }
    }

    private fun updateDetailCard(index: Int) {
        val p = points.getOrNull(index) ?: return
        val display = DisplayCurrency.entries[selectedCurrencyIndex.coerceIn(0, DisplayCurrency.entries.size - 1)]

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
