package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.core.content.ContextCompat
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class AllocationDriftAdapter(private val slices: List<AllocationDriftSlice>) :
    RecyclerView.Adapter<AllocationDriftAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val label: TextView = view.findViewById(R.id.driftLabel)
        val values: TextView = view.findViewById(R.id.driftValues)
        val bar: TargetProgressBarView = view.findViewById(R.id.driftBar)
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

        val context = holder.itemView.context
        holder.bar.setValues(
            actual = slice.actual.toFloat(),
            target = slice.target.toFloat(),
            overColor = ContextCompat.getColor(context, R.color.colorAmber),
            underColor = ContextCompat.getColor(context, R.color.colorSecondary),
            // Theme-aware, not a hardcoded translucent black - the
            // previous track color was ~8% opaque black, which vanished
            // entirely against a near-black dark-mode background.
            trackColor = ContextCompat.getColor(context, R.color.colorSurfaceVariant),
            markerColor = ContextCompat.getColor(context, R.color.colorLoss)
        )
    }

    override fun getItemCount(): Int = slices.size
}
