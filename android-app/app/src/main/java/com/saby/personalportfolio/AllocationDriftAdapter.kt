package com.saby.personalportfolio

import android.graphics.Color
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale
import kotlin.math.abs

class AllocationDriftAdapter(private val slices: List<AllocationDriftSlice>) :
    RecyclerView.Adapter<AllocationDriftAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val label: TextView = view.findViewById(R.id.driftLabel)
        val values: TextView = view.findViewById(R.id.driftValues)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_allocation_drift, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val slice = slices[position]
        holder.label.text = slice.label

        val driftSign = if (slice.drift > 0) "+" else ""
        holder.values.text = String.format(
            Locale.getDefault(),
            "Actual: %.1f%%  |  Target: %.1f%%  |  Drift: %s%.1f%%",
            slice.actual, slice.target, driftSign, slice.drift
        )

        // Within 3 points either way reads as "on target" (green);
        // beyond that scales toward amber/red the further off it is.
        // This is a display convenience, not a claim about what
        // tolerance actually matters for rebalancing - that's a personal
        // judgment call, not something to bake in as a hard rule.
        holder.values.setTextColor(
            when {
                abs(slice.drift) <= 3 -> Color.parseColor("#2E7D32") // green
                abs(slice.drift) <= 8 -> Color.parseColor("#F9A825") // amber
                else -> Color.parseColor("#C62828")                 // red
            }
        )
    }

    override fun getItemCount(): Int = slices.size
}
