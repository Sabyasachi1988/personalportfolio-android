package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

/**
 * Displays consolidated GroupedHolding rows (see
 * finance.GroupHoldingsByLabel) - reuses item_holding.xml exactly, same
 * as the plain per-fund HoldingsAdapter, so switching the "Group by
 * label" toggle doesn't change the visual language, just what each row
 * represents. A grouped row (2+ underlying funds) is tappable to see
 * which individual holdings it's made of - the underlying data always
 * stays distinguishable (a Nippon India Nifty 50 fund is still visibly
 * different from a Navi one), consolidation is purely a display choice.
 *
 * Includes the same tap-to-expand donut header as HoldingsAdapter, built
 * from these SAME grouped rows - so e.g. 3 funds labeled "Nifty 50"
 * among 12 total holdings show as exactly one "Nifty 50" slice plus 9
 * independent slices, matching the row list below it exactly. The donut
 * used to disappear entirely in grouped mode (this adapter had no
 * header view type at all) - that's fixed by giving it one here.
 *
 * EVERY row is tappable, not just grouped ones - onRowTapped decides
 * what a tap actually does (drill-down summary, or open a fund's own
 * chart) based on the row's own IsGroup/IsFamilyPool flags. This used
 * to be gated to grouped rows only here in the adapter itself (nothing
 * useful to do on tap for an ungrouped row, before family-pooled
 * ungrouped rows needed their own tap-through to a chart) - that gate
 * was a confirmed real regression once ungrouped rows DID have
 * somewhere to go: it silently swallowed the tap instead of calling
 * back at all, for every ungrouped row, plain fund-label mode included.
 */
class GroupedHoldingsAdapter(
    private val rows: List<GroupedHolding>,
    private val onRowTapped: (GroupedHolding) -> Unit
) : RecyclerView.Adapter<RecyclerView.ViewHolder>() {

    companion object {
        private const val TYPE_HEADER = 0
        private const val TYPE_ROW = 1
    }

    private val pricedRows = rows.filter { it.hasPrice }
    private val hasHeader = pricedRows.isNotEmpty()

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
            val view = LayoutInflater.from(parent.context).inflate(R.layout.item_holding, parent, false)
            RowHolder(view)
        }
    }

    override fun onBindViewHolder(holder: RecyclerView.ViewHolder, position: Int) {
        if (holder is HeaderHolder) {
            val totalValue = pricedRows.sumOf { it.currentValue }
            val slices = pricedRows
                .filter { totalValue > 0 }
                .map { DonutChartView.Slice(FundNameFormatter.shorten(it.displayName), ((it.currentValue / totalValue) * 100).toFloat()) }
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
        val row = rows[if (hasHeader) position - 1 else position]
        bindRow(rowHolder, row)
    }

    private fun bindRow(holder: RowHolder, row: GroupedHolding) {
        val namePrefix = if (row.isGroup) "▸ " else ""
        holder.name.text = namePrefix + row.displayName.ifBlank { "(unnamed)" }

        if (row.hasPrice) {
            holder.currentValue.text = IndianCurrencyFormatter.format(row.currentValue, decimals = 0)
            val gainColor = androidx.core.content.ContextCompat.getColor(
                holder.itemView.context, if (row.gain >= 0) R.color.colorGain else R.color.colorLoss
            )
            val gainSign = if (row.gain >= 0) "+" else ""
            holder.gainBadge.text = String.format(Locale.getDefault(), "%s%.2f%%", gainSign, row.gainPercent)
            holder.gainBadge.setTextColor(gainColor)
            HoldingsAdapter.bindDayGain(holder.dayGain, row.hasDayGain, row.dayGain, row.dayGainPercent)
        } else {
            holder.currentValue.text = "Price not available"
            holder.gainBadge.text = ""
            holder.dayGain.visibility = View.GONE
        }

        val xirrPart = if (row.hasXirr) String.format(Locale.getDefault(), " · XIRR %.2f%%", row.xirr) else ""
        val investedPart = "Invested ${IndianCurrencyFormatter.format(row.netInvested, decimals = 0)}"
        // "also held by" only makes sense for an UNGROUPED row - see
        // GroupedHolding.AlsoHeldByMembers' own Go doc comment for why
        // it's always empty on a grouped row anyway, but the isGroup
        // check here is the actual display-time guard.
        val alsoHeldByPart = if (!row.isGroup && row.alsoHeldByMembers.isNotEmpty()) {
            " · also held by ${row.alsoHeldByMembers.joinToString(", ")}"
        } else {
            ""
        }
        holder.secondaryLine.text = if (row.isGroup) {
            "${row.assetIds.size} funds · $investedPart$xirrPart · tap to see which"
        } else {
            "$investedPart$xirrPart$alsoHeldByPart"
        }

        holder.itemView.setOnClickListener { onRowTapped(row) }
    }

    override fun getItemCount(): Int = rows.size + if (hasHeader) 1 else 0
}
