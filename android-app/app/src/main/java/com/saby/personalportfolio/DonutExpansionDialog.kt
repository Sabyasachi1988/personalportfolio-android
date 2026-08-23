package com.saby.personalportfolio

import android.content.Context
import android.view.LayoutInflater
import android.view.View
import android.widget.TextView
import androidx.appcompat.app.AlertDialog

/**
 * Shows a donut chart's slices in an enlarged dialog with a full,
 * untruncated vertical legend - used wherever a compact donut card is
 * tapped (Holdings' per-fund breakdown, Allocation's three sections).
 *
 * Replaces two things that used to be separate, inconsistent behaviors:
 *  - Holdings previously always showed a horizontally-scrolling chip
 *    legend under the small donut, most of it off-screen at a glance and
 *    truncated even for the visible chips.
 *  - Allocation previously navigated straight to a filtered Holdings
 *    view on any donut tap - useful, but surprising as the FIRST thing
 *    a tap does, and inconsistent with how Holdings' donut behaved.
 *
 * Now everywhere: tap the compact donut -> see the full picture here.
 * If `onSliceSelected` is provided, a hint appears and tapping a slice
 * (on the enlarged donut ring OR its legend row) both invokes it AND
 * closes the dialog - this is where Allocation's "jump to these
 * holdings" action now lives, as a deliberate second step rather than
 * the tap's only possible outcome.
 */
object DonutExpansionDialog {

    fun show(
        context: Context,
        title: String,
        slices: List<DonutChartView.Slice>,
        navigationHint: String? = null,
        onSliceSelected: ((label: String) -> Unit)? = null
    ) {
        if (slices.isEmpty()) return

        val view: View = LayoutInflater.from(context).inflate(R.layout.dialog_expanded_donut, null)
        val titleView = view.findViewById<TextView>(R.id.expandedDonutTitle)
        val chart = view.findViewById<DonutChartView>(R.id.expandedDonutChart)
        val legend = view.findViewById<DonutLegendView>(R.id.expandedDonutLegend)
        val hintView = view.findViewById<TextView>(R.id.expandedDonutHint)

        titleView.text = title
        chart.setSlices(slices)
        legend.chipMode = false
        legend.setSlices(slices)

        val dialog = AlertDialog.Builder(context)
            .setView(view)
            .setPositiveButton("Close", null)
            .create()

        if (onSliceSelected != null) {
            hintView.visibility = View.VISIBLE
            hintView.text = navigationHint ?: "Tap a segment to view its holdings"
            val select: (String) -> Unit = { label ->
                onSliceSelected(label)
                dialog.dismiss()
            }
            chart.onSliceTapped = { label, _ -> select(label) }
            legend.onRowTapped = select
        } else {
            hintView.visibility = View.GONE
            chart.onSliceTapped = null
            legend.onRowTapped = null
        }

        dialog.show()
    }
}
