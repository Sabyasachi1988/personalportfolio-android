package com.saby.personalportfolio

import android.content.Intent
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class HoldingsAdapter(private val holdings: List<Holding>) :
    RecyclerView.Adapter<RecyclerView.ViewHolder>() {

    companion object {
        private const val TYPE_HEADER = 0
        private const val TYPE_ROW = 1

        // Shared by HoldingsAdapter and GroupedHoldingsAdapter's own
        // per-row Day gain/loss line - see Holding.DayGain's doc comment
        // (internal/finance/holdings.go) for what this figure measures:
        // this fund's own most recent day-over-day move, not a
        // portfolio-wide anchored figure the way the Dashboard's Day
        // chip is.
        fun bindDayGain(view: TextView, hasDayGain: Boolean, dayGain: Double, dayGainPercent: Double) {
            if (!hasDayGain) {
                view.visibility = View.GONE
                return
            }
            view.visibility = View.VISIBLE
            val percentSign = if (dayGainPercent >= 0) "+" else ""
            val color = androidx.core.content.ContextCompat.getColor(
                view.context, if (dayGain >= 0) R.color.colorGain else R.color.colorLoss
            )
            view.text = String.format(
                Locale.getDefault(),
                "Day: %s (%s%.2f%%)",
                IndianCurrencyFormatter.formatSigned(dayGain, decimals = 0), percentSign, dayGainPercent
            )
            view.setTextColor(color)
        }
    }

    // Only priced holdings can be shown as weights of a real total; the
    // header is skipped entirely if nothing is priced yet, rather than
    // showing a misleading or empty chart.
    private val pricedHoldings = holdings.filter { it.hasPrice }
    private val hasHeader = pricedHoldings.isNotEmpty()

    class HeaderHolder(view: View) : RecyclerView.ViewHolder(view) {
        val container: View = view.findViewById(R.id.holdingsChartHeaderContainer)
        val donut: DonutChartView = view.findViewById(R.id.perFundDonut)
    }

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.holdingName)
        val currentValue: TextView = view.findViewById(R.id.holdingCurrentValue)
        val gainBadge: TextView = view.findViewById(R.id.holdingGainBadge)
        val secondaryLine: TextView = view.findViewById(R.id.holdingSecondaryLine)
        val dayGain: TextView = view.findViewById(R.id.holdingDayGain)
    }

    override fun getItemViewType(position: Int): Int {
        return if (hasHeader && position == 0) TYPE_HEADER else TYPE_ROW
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RecyclerView.ViewHolder {
        return if (viewType == TYPE_HEADER) {
            val view = LayoutInflater.from(parent.context)
                .inflate(R.layout.item_holdings_chart_header, parent, false)
            HeaderHolder(view)
        } else {
            val view = LayoutInflater.from(parent.context)
                .inflate(R.layout.item_holding, parent, false)
            RowHolder(view)
        }
    }

    override fun onBindViewHolder(holder: RecyclerView.ViewHolder, position: Int) {
        if (holder is HeaderHolder) {
            val totalValue = pricedHoldings.sumOf { it.currentValue }
            val slices = pricedHoldings
                .filter { totalValue > 0 }
                .map { DonutChartView.Slice(FundNameFormatter.shorten(it.canonicalName.ifEmpty { it.assetName }), ((it.currentValue / totalValue) * 100).toFloat()) }
                .sortedByDescending { it.percent }
            holder.donut.setSlices(slices)
            val openExpanded = {
                DonutExpansionDialog.show(holder.itemView.context, "Portfolio by fund", slices)
            }
            holder.donut.onSliceTapped = { _, _ -> openExpanded() }
            holder.container.setOnClickListener { openExpanded() }
            return
        }

        val rowHolder = holder as RowHolder
        val h = holdings[if (hasHeader) position - 1 else position]

        // Tap-through to the same per-fund chart (with transaction
        // markers + range presets) the Returns screen's own row tap
        // already opens - see ReturnsDetailActivity. Holdings is the
        // natural "specific fund" card list for this: Allocation's own
        // tabs (market cap / equity origin / portfolio class / tags)
        // are all AGGREGATED slices with no single fund's identity to
        // navigate with, so there's no equivalent tap target to add
        // there.
        rowHolder.itemView.setOnClickListener {
            val intent = Intent(rowHolder.itemView.context, ReturnsDetailActivity::class.java)
            intent.putExtra(ReturnsDetailActivity.EXTRA_SERIES_ID, h.assetId)
            intent.putExtra(ReturnsDetailActivity.EXTRA_NAME, FundNameFormatter.shorten(h.assetName))
            intent.putExtra(ReturnsDetailActivity.EXTRA_IS_BENCHMARK, false)
            rowHolder.itemView.context.startActivity(intent)
        }

        rowHolder.name.text = FundNameFormatter.shorten(h.canonicalName.ifEmpty { h.assetName }).ifBlank { "(unnamed asset)" }

        val alsoHeldByPart = if (h.alsoHeldByMembers.isNotEmpty()) {
            " · also held by ${h.alsoHeldByMembers.joinToString(", ")}"
        } else {
            ""
        }

        if (h.hasPrice) {
            rowHolder.currentValue.text = IndianCurrencyFormatter.format(h.currentValue, decimals = 0)

            val gainColor = androidx.core.content.ContextCompat.getColor(
                rowHolder.itemView.context, if (h.gain >= 0) R.color.colorGain else R.color.colorLoss
            )
            val gainSign = if (h.gain >= 0) "+" else ""
            rowHolder.gainBadge.text = String.format(Locale.getDefault(), "%s%.2f%%", gainSign, h.gainPercent)
            rowHolder.gainBadge.setTextColor(gainColor)

            val xirrPart = if (h.hasXirr) String.format(Locale.getDefault(), " · XIRR %.2f%%", h.xirr) else ""
            rowHolder.secondaryLine.text = String.format(
                Locale.getDefault(),
                "%s units · Invested %s%s%s",
                unitsDisplay(h.unitsHeld), IndianCurrencyFormatter.format(h.netInvested, decimals = 0), xirrPart, alsoHeldByPart
            )
            bindDayGain(rowHolder.dayGain, h.hasDayGain, h.dayGain, h.dayGainPercent)
        } else {
            rowHolder.currentValue.text = "Price not available"
            rowHolder.gainBadge.text = ""
            rowHolder.secondaryLine.text = String.format(
                Locale.getDefault(), "%s units · Invested %s%s", unitsDisplay(h.unitsHeld), IndianCurrencyFormatter.format(h.netInvested, decimals = 0), alsoHeldByPart
            )
            rowHolder.dayGain.visibility = View.GONE
        }
    }

    // Units held combined with a fund/stock's publicly-known price
    // directly reveals its rupee value - see IncognitoMode's doc
    // comment on why rupee amounts are masked; a raw unit count sitting
    // right next to a masked amount would defeat that entirely, since
    // anyone can look up a fund's NAV or a stock's price themselves. A
    // FIXED placeholder, not one sized to the real digit count, for the
    // same "don't let the mask's own shape leak information" reasoning
    // as IndianCurrencyFormatter's MASK.
    private fun unitsDisplay(units: Double): String {
        if (IncognitoMode.isEnabled) return "•••"
        return String.format(Locale.getDefault(), "%.3f", units)
    }

    override fun getItemCount(): Int = holdings.size + if (hasHeader) 1 else 0
}
