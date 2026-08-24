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
 */
class GroupedHoldingsAdapter(
    private val rows: List<GroupedHolding>,
    private val onDrillDown: (GroupedHolding) -> Unit
) : RecyclerView.Adapter<GroupedHoldingsAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.holdingName)
        val currentValue: TextView = view.findViewById(R.id.holdingCurrentValue)
        val gainBadge: TextView = view.findViewById(R.id.holdingGainBadge)
        val secondaryLine: TextView = view.findViewById(R.id.holdingSecondaryLine)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_holding, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val row = rows[position]
        val namePrefix = if (row.isGroup) "▸ " else ""
        holder.name.text = namePrefix + row.displayName.ifBlank { "(unnamed)" }

        if (row.hasPrice) {
            holder.currentValue.text = IndianCurrencyFormatter.format(row.currentValue, decimals = 0)
            val gainColor = androidx.core.content.ContextCompat.getColor(
                holder.itemView.context, if (row.gain >= 0) R.color.colorGain else R.color.colorLoss
            )
            val gainSign = if (row.gain >= 0) "+" else ""
            holder.gainBadge.text = String.format(Locale.getDefault(), "%s%.1f%%", gainSign, row.gainPercent)
            holder.gainBadge.setTextColor(gainColor)
        } else {
            holder.currentValue.text = "Price not available"
            holder.gainBadge.text = ""
        }

        val xirrPart = if (row.hasXirr) String.format(Locale.getDefault(), " · XIRR %.1f%%", row.xirr) else ""
        val investedPart = "Invested ${IndianCurrencyFormatter.format(row.netInvested, decimals = 0)}"
        holder.secondaryLine.text = if (row.isGroup) {
            "${row.assetIds.size} funds · $investedPart$xirrPart · tap to see which"
        } else {
            "$investedPart$xirrPart"
        }

        holder.itemView.setOnClickListener {
            if (row.isGroup) onDrillDown(row)
        }
    }

    override fun getItemCount(): Int = rows.size
}
