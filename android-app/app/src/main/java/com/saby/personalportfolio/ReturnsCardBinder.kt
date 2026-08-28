package com.saby.personalportfolio

import android.view.View
import android.widget.TextView
import androidx.core.content.ContextCompat
import java.util.Locale

/**
 * Binds one ReturnsTableRow's data onto an inflated item_returns_row.xml
 * root view. Was previously a RecyclerView.Adapter (one card per row in
 * a long scrolling list) - now the Returns screen shows a single
 * picked card at a time (see ReturnsActivity), so this is just a plain
 * binder function over one already-inflated view instead.
 */
object ReturnsCardBinder {

    private class CellViews(root: View) {
        val label: TextView = root.findViewById(R.id.returnsCellLabel)
        val value: TextView = root.findViewById(R.id.returnsCellValue)
        val range: TextView = root.findViewById(R.id.returnsCellRange)
    }

    private class TenureViews(root: View) {
        val label: TextView = root.findViewById(R.id.tenureRowLabel)
        val trailing: TextView = root.findViewById(R.id.tenureRowTrailing)
        val rollingMedian: TextView = root.findViewById(R.id.tenureRowRollingMedian)
        val rollingRange: TextView = root.findViewById(R.id.tenureRowRollingRange)
    }

    fun bind(cardRoot: View, row: ReturnsTableRow) {
        val context = cardRoot.context
        cardRoot.findViewById<TextView>(R.id.returnsRowName).text =
            FundNameFormatter.shorten(row.name).ifBlank { row.name }

        val typeBadge = cardRoot.findViewById<TextView>(R.id.returnsRowTypeBadge)
        if (row.isBenchmark) {
            typeBadge.text = "Index"
            typeBadge.setTextColor(ContextCompat.getColor(context, R.color.colorAmber))
        } else {
            typeBadge.text = "Fund"
            typeBadge.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
        }

        bindTrailingCell(CellViews(cardRoot.findViewById(R.id.returnsCellDay)), "Day", row.day)
        bindTrailingCell(CellViews(cardRoot.findViewById(R.id.returnsCellMonth)), "1 Month", row.month)

        bindTenureRow(TenureViews(cardRoot.findViewById(R.id.returnsRowOneYear)), "1Y", row.oneYearTrailing, row.oneYearRolling)
        bindTenureRow(TenureViews(cardRoot.findViewById(R.id.returnsRowThreeYear)), "3Y", row.threeYearTrailing, row.threeYearRolling)
        bindTenureRow(TenureViews(cardRoot.findViewById(R.id.returnsRowFiveYear)), "5Y", row.fiveYearTrailing, row.fiveYearRolling)
        bindTenureRow(TenureViews(cardRoot.findViewById(R.id.returnsRowTenYear)), "10Y", row.tenYearTrailing, row.tenYearRolling)

        // Reset any previously-shown custom-period row from a different
        // card - each card starts fresh until the person types a period
        // and taps Show again for THIS fund.
        cardRoot.findViewById<View>(R.id.returnsRowCustomPeriod).visibility = View.GONE
        cardRoot.findViewById<android.widget.EditText>(R.id.returnsCustomPeriodYears).setText("")
    }

    /**
     * Shows the custom-period tenure row with the given (already-
     * computed by the caller, via Bridge.computeCustomPeriodReturn)
     * trailing + rolling result - called from ReturnsActivity, which
     * owns the portfolio JSON this bridge call needs and isn't
     * something this stateless binder has access to.
     */
    fun bindCustomPeriod(cardRoot: View, years: Double, trailing: TrailingReturn, rolling: RollingReturnStats) {
        val row = cardRoot.findViewById<View>(R.id.returnsRowCustomPeriod)
        row.visibility = View.VISIBLE
        val label = if (years == years.toLong().toDouble()) {
            String.format(Locale.getDefault(), "%dY", years.toLong())
        } else {
            String.format(Locale.getDefault(), "%.1fY", years)
        }
        bindTenureRow(TenureViews(row), label, trailing, rolling)
    }

    private fun bindTrailingCell(cell: CellViews, label: String, r: TrailingReturn) {
        cell.label.text = label
        val context = cell.value.context
        if (!r.hasData) {
            cell.value.text = "—"
            cell.value.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
            cell.range.text = ""
            return
        }
        cell.value.text = String.format(Locale.getDefault(), "%+.2f%%", r.percent)
        cell.value.setTextColor(
            ContextCompat.getColor(context, if (r.percent >= 0) R.color.colorGain else R.color.colorLoss)
        )
        cell.range.text = ""
    }

    private fun bindTenureRow(views: TenureViews, label: String, trailing: TrailingReturn, rolling: RollingReturnStats) {
        views.label.text = label
        val context = views.trailing.context

        if (!trailing.hasData) {
            views.trailing.text = "—"
            views.trailing.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
        } else {
            views.trailing.text = String.format(Locale.getDefault(), "%+.2f%%", trailing.percent)
            views.trailing.setTextColor(
                ContextCompat.getColor(context, if (trailing.percent >= 0) R.color.colorGain else R.color.colorLoss)
            )
        }

        if (!rolling.hasData) {
            views.rollingMedian.text = "—"
            views.rollingMedian.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
            views.rollingRange.text = "Not enough history"
        } else {
            views.rollingMedian.text = String.format(Locale.getDefault(), "%+.2f%%", rolling.median)
            views.rollingMedian.setTextColor(
                ContextCompat.getColor(context, if (rolling.median >= 0) R.color.colorGain else R.color.colorLoss)
            )
            views.rollingRange.text = String.format(Locale.getDefault(), "[%+.2f, %+.2f]", rolling.min, rolling.max)
        }
    }
}
