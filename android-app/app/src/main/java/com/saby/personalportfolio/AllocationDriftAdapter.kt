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

    // One scale for the whole table, not one per row (see
    // TargetProgressBarView.setValues doc comment for why per-row scaling
    // was wrong). +15% headroom so the largest bar doesn't touch the very
    // edge of its row.
    private val sharedMaxVal: Float = (slices.flatMap { listOf(it.actual, it.target) }
        .maxOrNull()?.toFloat() ?: 1f).coerceAtLeast(1f) * 1.15f

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
        val segmentColor = CapSegmentColors.forLabel(context, slice.label)
        holder.bar.setValues(
            actual = slice.actual.toFloat(),
            target = slice.target.toFloat(),
            sharedMaxVal = sharedMaxVal,
            // Same fixed color as the donut uses for this exact label -
            // previously this used a generic amber/teal over/under
            // scheme that had nothing to do with the donut's colors,
            // which is exactly what looked inconsistent between the two
            // charts on the same screen.
            overColor = segmentColor,
            underColor = segmentColor,
            trackColor = ContextCompat.getColor(context, R.color.colorSurfaceVariant),
            markerColor = ContextCompat.getColor(context, R.color.colorLoss)
        )
    }

    override fun getItemCount(): Int = slices.size
}
