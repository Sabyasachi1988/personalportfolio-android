package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.CheckBox
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.RecyclerView

class ManageNamesAdapter(
    private val entries: List<NameListEntry>,
    private val onTap: (entry: NameListEntry) -> Unit,
    private val onToggleBenchmark: (entry: NameListEntry, usable: Boolean) -> Unit
) : RecyclerView.Adapter<ManageNamesAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val typeBadge: TextView = view.findViewById(R.id.nameRowTypeBadge)
        val defaultName: TextView = view.findViewById(R.id.nameRowDefaultName)
        val nickname: TextView = view.findViewById(R.id.nameRowNickname)
        val benchmarkCheckbox: CheckBox = view.findViewById(R.id.nameRowBenchmarkCheckbox)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_manage_name_row, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val entry = entries[position]
        val context = holder.itemView.context
        if (entry.isBenchmark) {
            holder.typeBadge.text = "Index"
            holder.typeBadge.setTextColor(ContextCompat.getColor(context, R.color.colorAmber))
        } else {
            holder.typeBadge.text = "Fund"
            holder.typeBadge.setTextColor(ContextCompat.getColor(context, R.color.colorNeutral))
        }
        // Default (real) name is ALWAYS shown, even when a nickname is
        // set - unlike every other screen, which shows only the
        // resolved display name, this is the one screen where the
        // person needs to see both: what they're overriding, and what
        // they're overriding it WITH.
        holder.defaultName.text = entry.name
        holder.nickname.text = if (entry.nickname.isNotBlank()) {
            "Nicknamed: ${entry.nickname}"
        } else {
            "No nickname set - tap to add one"
        }
        holder.itemView.setOnClickListener { onTap(entry) }

        // "Usable as benchmark" only makes sense for a FUND with a
        // real ISIN - AddBenchmarkFromAsset needs one (see its own Go
        // doc comment), and toggling it on a benchmark entry itself is
        // meaningless. GONE rather than disabled so it doesn't clutter
        // rows where it can never apply.
        if (!entry.isBenchmark && entry.isin.isNotBlank()) {
            holder.benchmarkCheckbox.visibility = View.VISIBLE
            // Clear the old listener before setting a new checked
            // state - CheckBox fires onCheckedChange on
            // setChecked(), and RecyclerView reuses this ViewHolder
            // for a DIFFERENT entry on scroll, which would otherwise
            // fire a toggle callback for the wrong fund.
            holder.benchmarkCheckbox.setOnCheckedChangeListener(null)
            holder.benchmarkCheckbox.isChecked = entry.usableAsBenchmark
            holder.benchmarkCheckbox.setOnCheckedChangeListener { _, checked ->
                onToggleBenchmark(entry, checked)
            }
        } else {
            holder.benchmarkCheckbox.visibility = View.GONE
            holder.benchmarkCheckbox.setOnCheckedChangeListener(null)
        }
    }

    override fun getItemCount(): Int = entries.size
}
