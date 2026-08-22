package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class AllocationAdapter(private val slices: List<AllocationSlice>) :
    RecyclerView.Adapter<AllocationAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val label: TextView = view.findViewById(R.id.sliceLabel)
        val valueLine: TextView = view.findViewById(R.id.sliceValueLine)
        val barFilled: View = view.findViewById(R.id.sliceBarFilled)
        val barEmpty: View = view.findViewById(R.id.sliceBarEmpty)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_allocation, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val slice = slices[position]

        holder.label.text = slice.label
        holder.valueLine.text = String.format(
            Locale.getDefault(),
            "%s (%.1f%%)",
            IndianCurrencyFormatter.format(slice.value),
            slice.percent
        )

        // Weight-based bar: clamp to [0, 100] defensively, since these
        // weights come from a live computation, not a fixed source.
        val filledWeight = slice.percent.toFloat().coerceIn(0f, 100f)
        val emptyWeight = 100f - filledWeight
        (holder.barFilled.layoutParams as android.widget.LinearLayout.LayoutParams).weight = filledWeight
        (holder.barEmpty.layoutParams as android.widget.LinearLayout.LayoutParams).weight = emptyWeight
        holder.barFilled.requestLayout()
        holder.barEmpty.requestLayout()
    }

    override fun getItemCount(): Int = slices.size
}
