package com.saby.personalportfolio

import android.os.Bundle
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

class ProgressionActivity : AppCompatActivity() {

    private val gson = Gson()

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
    private var points: List<ProgressionPoint> = emptyList()
    private var currentAxis: ProgressionAxis = ProgressionAxis.WHOLE_PORTFOLIO

    // When set, the picker is in "specific fund" mode: the axis
    // selection is ignored and ComputeAssetProgression is used instead
    // of ComputeProgression (see loadAndShowProgression). Null means the
    // normal axis-based (whole portfolio / equity split) mode.
    private var selectedAssetId: String? = null
    private var assets: List<AssetSummary> = emptyList()
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
            resetZoomButton.visibility = if (zoomed) View.VISIBLE else View.GONE
        }
        resetZoomButton.setOnClickListener { chart.resetZoom() }
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
     * One combined picker: the 4 whole-portfolio/equity axes, followed
     * by every individual fund - so browsing a single holding's own
     * growth story is one tap away from where you'd naturally look for
     * "what am I viewing", rather than a separate control competing for
     * the same limited row width.
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

        popup.setOnMenuItemClickListener { item ->
            val id = item.itemId
            if (id < ProgressionAxis.entries.size) {
                selectedAssetId = null
                selectedAxisIndex = id
                axisTab.text = ProgressionAxis.entries[id].label
            } else if (id > fundIdBase) {
                val asset = assets.getOrNull(id - fundIdBase - 1)
                if (asset != null) {
                    selectedAssetId = asset.id
                    axisTab.text = FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" }
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
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadAndShowProgression() {
        if (memberIds.isEmpty()) return // list not populated yet - loadMemberList will call back in
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val today = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())

        val assetId = selectedAssetId
        val resultJson: String
        if (assetId != null) {
            // Fund mode: a single fund is already fully scoped, so the
            // axis/member pickers don't apply - the currency picker's
            // "Native" option still needs to know whether this
            // particular fund is INR or foreign, so currentAxis is set
            // here purely as ProgressionCurrency's existing INR/CAD
            // switch (see ProgressionCurrency.nativeCurrencyFor), not
            // because this is really an "International Equity" view.
            currentAxis = if (accountCurrencyByAssetId[assetId] == "INR") {
                ProgressionAxis.WHOLE_PORTFOLIO
            } else {
                ProgressionAxis.INTERNATIONAL_EQUITY
            }
            resultJson = Bridge.computeAssetProgression(portfolioJson, assetId, today)
        } else {
            val memberId = memberIds.getOrElse(selectedMemberIndex) { "" }
            currentAxis = ProgressionAxis.entries[selectedAxisIndex.coerceIn(0, ProgressionAxis.entries.size - 1)]
            resultJson = Bridge.computeProgression(portfolioJson, memberId, currentAxis.bridgeValue, today)
        }

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
            chart.setPoints(emptyList())
            dateText.text = ""
            investedText.text = ""
            valueText.text = ""
            gainText.text = ""
            xirrText.text = ""
            return
        }

        statusText.text = if (assetId != null) "Weekly checkpoints for this fund" else "Weekly checkpoints"
        seekBar.max = (points.size - 1).coerceAtLeast(0)
        chart.setPoints(points) // triggers onScrub for the last point, which updates the detail card
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
