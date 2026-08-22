package com.saby.personalportfolio

import android.graphics.Color
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
    }

    // Only priced holdings can be shown as weights of a real total; the
    // header is skipped entirely if nothing is priced yet, rather than
    // showing a misleading or empty chart.
    private val pricedHoldings = holdings.filter { it.hasPrice }
    private val hasHeader = pricedHoldings.isNotEmpty()

    class HeaderHolder(view: View) : RecyclerView.ViewHolder(view) {
        val donut: DonutChartView = view.findViewById(R.id.perFundDonut)
        val legend: DonutLegendView = view.findViewById(R.id.perFundLegend)

        // Android queues successive Toast.makeText(...).show() calls
        // rather than replacing an in-flight one - tapping a second
        // slice right after the first would otherwise wait out the
        // first toast's full duration before showing anything new.
        // Keeping one Toast reference and cancelling it before showing
        // the next makes the tap feel immediate.
        private var currentToast: android.widget.Toast? = null

        fun showSliceToast(label: String, percent: Float) {
            currentToast?.cancel()
            val toast = android.widget.Toast.makeText(
                itemView.context,
                String.format(Locale.getDefault(), "%s: %.1f%%", label, percent),
                android.widget.Toast.LENGTH_SHORT
            )
            currentToast = toast
            toast.show()
        }
    }

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.holdingName)
        val currentValue: TextView = view.findViewById(R.id.holdingCurrentValue)
        val gainBadge: TextView = view.findViewById(R.id.holdingGainBadge)
        val secondaryLine: TextView = view.findViewById(R.id.holdingSecondaryLine)
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
                .map { DonutChartView.Slice(FundNameFormatter.shorten(it.assetName), ((it.currentValue / totalValue) * 100).toFloat()) }
                .sortedByDescending { it.percent }
            holder.donut.setSlices(slices)
            holder.legend.setSlices(slices)
            holder.donut.onSliceTapped = { label, percent ->
                holder.showSliceToast(label, percent)
            }
            return
        }

        val rowHolder = holder as RowHolder
        val h = holdings[if (hasHeader) position - 1 else position]

        rowHolder.name.text = FundNameFormatter.shorten(h.assetName).ifBlank { "(unnamed asset)" }

        if (h.hasPrice) {
            rowHolder.currentValue.text = IndianCurrencyFormatter.format(h.currentValue, decimals = 0)

            val gainColor = if (h.gain >= 0) Color.parseColor("#2E7D32") else Color.parseColor("#C62828")
            val gainSign = if (h.gain >= 0) "+" else ""
            rowHolder.gainBadge.text = String.format(Locale.getDefault(), "%s%.1f%%", gainSign, h.gainPercent)
            rowHolder.gainBadge.setTextColor(gainColor)

            val xirrPart = if (h.hasXirr) String.format(Locale.getDefault(), " · XIRR %.1f%%", h.xirr) else ""
            rowHolder.secondaryLine.text = String.format(
                Locale.getDefault(),
                "%.3f units · Invested %s%s",
                h.unitsHeld, IndianCurrencyFormatter.format(h.netInvested, decimals = 0), xirrPart
            )
        } else {
            rowHolder.currentValue.text = "Price not available"
            rowHolder.gainBadge.text = ""
            rowHolder.secondaryLine.text = String.format(
                Locale.getDefault(), "%.3f units · Invested %s", h.unitsHeld, IndianCurrencyFormatter.format(h.netInvested, decimals = 0)
            )
        }
    }

    override fun getItemCount(): Int = holdings.size + if (hasHeader) 1 else 0
}
