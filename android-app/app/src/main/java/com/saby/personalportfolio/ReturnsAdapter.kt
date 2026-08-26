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

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.returnsRowName)
        val typeBadge: TextView = view.findViewById(R.id.returnsRowTypeBadge)
        val day = CellViews(view.findViewById(R.id.returnsCellDay))
        val month = CellViews(view.findViewById(R.id.returnsCellMonth))
        val oneYear = CellViews(view.findViewById(R.id.returnsCellOneYear))
        val threeYear = CellViews(view.findViewById(R.id.returnsCellThreeYear))
        val fiveYear = CellViews(view.findViewById(R.id.returnsCellFiveYear))
        val tenYear = CellViews(view.findViewById(R.id.returnsCellTenYear))
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

        bindTrailing(holder.day, "Day", row.day)
        bindTrailing(holder.month, "1 Month", row.month)
        bindRolling(holder.oneYear, "1 Year", row.oneYear)
        bindRolling(holder.threeYear, "3 Year", row.threeYear)
        bindRolling(holder.fiveYear, "5 Year", row.fiveYear)
        bindRolling(holder.tenYear, "10 Year", row.tenYear)

        holder.itemView.setOnClickListener { onRowTapped(row) }
    }

    private fun bindTrailing(cell: CellViews, label: String, r: TrailingReturn) {
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

    private fun bindRolling(cell: CellViews, label: String, r: RollingReturnStats) {
        cell.label.text = label
        val context = cell.value.context
        if (!r.hasData) {
            cell.value.text = "—"
            cell.value.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
            cell.range.text = "Not enough history"
            return
        }
        cell.value.text = String.format(Locale.getDefault(), "%+.1f%%", r.median)
        cell.value.setTextColor(
            ContextCompat.getColor(context, if (r.median >= 0) R.color.colorGain else R.color.colorLoss)
        )
        cell.range.text = String.format(Locale.getDefault(), "[%+.0f, %+.0f]", r.min, r.max)
    }

    override fun getItemCount(): Int = rows.size
}
