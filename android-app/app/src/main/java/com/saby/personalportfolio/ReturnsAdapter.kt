package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class ReturnsAdapter(
    private val rows: List<ReturnsTableRow>,
    private val onRowTapped: (ReturnsTableRow) -> Unit
) : RecyclerView.Adapter<ReturnsAdapter.RowHolder>() {

    class CellViews(root: View) {
        val label: TextView = root.findViewById(R.id.returnsCellLabel)
        val value: TextView = root.findViewById(R.id.returnsCellValue)
        val range: TextView = root.findViewById(R.id.returnsCellRange)
    }

    class TenureRowViews(root: View) {
        val label: TextView = root.findViewById(R.id.tenureRowLabel)
        val trailing: TextView = root.findViewById(R.id.tenureRowTrailing)
        val rollingMedian: TextView = root.findViewById(R.id.tenureRowRollingMedian)
        val rollingRange: TextView = root.findViewById(R.id.tenureRowRollingRange)
    }

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.returnsRowName)
        val typeBadge: TextView = view.findViewById(R.id.returnsRowTypeBadge)
        val day = CellViews(view.findViewById(R.id.returnsCellDay))
        val month = CellViews(view.findViewById(R.id.returnsCellMonth))
        val oneYear = TenureRowViews(view.findViewById(R.id.returnsRowOneYear))
        val threeYear = TenureRowViews(view.findViewById(R.id.returnsRowThreeYear))
        val fiveYear = TenureRowViews(view.findViewById(R.id.returnsRowFiveYear))
        val tenYear = TenureRowViews(view.findViewById(R.id.returnsRowTenYear))
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_returns_row, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val row = rows[position]
        holder.name.text = FundNameFormatter.shorten(row.name).ifBlank { row.name }
        val context = holder.itemView.context
        if (row.isBenchmark) {
            holder.typeBadge.text = "Index"
            holder.typeBadge.setTextColor(ContextCompat.getColor(context, R.color.colorAmber))
        } else {
            holder.typeBadge.text = "Fund"
            holder.typeBadge.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
        }

        bindTrailingCell(holder.day, "Day", row.day)
        bindTrailingCell(holder.month, "1 Month", row.month)

        bindTenureRow(holder.oneYear, "1Y", row.oneYearTrailing, row.oneYearRolling)
        bindTenureRow(holder.threeYear, "3Y", row.threeYearTrailing, row.threeYearRolling)
        bindTenureRow(holder.fiveYear, "5Y", row.fiveYearTrailing, row.fiveYearRolling)
        bindTenureRow(holder.tenYear, "10Y", row.tenYearTrailing, row.tenYearRolling)

        holder.itemView.setOnClickListener { onRowTapped(row) }
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
        cell.value.text = String.format(Locale.getDefault(), "%+.1f%%", r.percent)
        cell.value.setTextColor(
            ContextCompat.getColor(context, if (r.percent >= 0) R.color.colorGain else R.color.colorLoss)
        )
        cell.range.text = ""
    }

    private fun bindTenureRow(row: TenureRowViews, label: String, trailing: TrailingReturn, rolling: RollingReturnStats) {
        row.label.text = label
        val context = row.trailing.context

        if (trailing.hasData) {
            row.trailing.text = String.format(Locale.getDefault(), "%+.1f%%", trailing.percent)
            row.trailing.setTextColor(
                ContextCompat.getColor(context, if (trailing.percent >= 0) R.color.colorGain else R.color.colorLoss)
            )
        } else {
            row.trailing.text = "—"
            row.trailing.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
        }

        if (rolling.hasData) {
            row.rollingMedian.text = String.format(Locale.getDefault(), "%+.1f%%", rolling.median)
            row.rollingMedian.setTextColor(
                ContextCompat.getColor(context, if (rolling.median >= 0) R.color.colorGain else R.color.colorLoss)
            )
            row.rollingRange.text = String.format(Locale.getDefault(), "[%+.0f, %+.0f]", rolling.min, rolling.max)
        } else {
            row.rollingMedian.text = "—"
            row.rollingMedian.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
            row.rollingRange.text = "not enough history"
        }
    }

    override fun getItemCount(): Int = rows.size
}
