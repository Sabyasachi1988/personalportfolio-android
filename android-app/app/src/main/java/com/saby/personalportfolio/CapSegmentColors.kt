package com.saby.personalportfolio

import android.content.Context
import androidx.core.content.ContextCompat

/**
 * Cap-segment labels are a small, known, fixed set (Large/Mid/Small
 * Cap, Cash, plus the heuristic fallback buckets). Unlike arbitrary fund
 * names, it's worth giving each one a genuinely fixed color used
 * everywhere it appears, rather than whatever position it happens to
 * land in a generic index-cycled palette - that's what caused the same
 * "Large Cap" to show as a different color on the donut vs. the drift
 * bar on the same screen.
 */
object CapSegmentColors {
    fun forLabel(context: Context, label: String): Int {
        val palette = ChartColors.palette(context)
        fun c(id: Int) = androidx.core.content.ContextCompat.getColor(context, id)
        // The market-cap labels use the SAME validated colours as the Over Time chart, so a segment is one colour
        // on the Dashboard, Allocation and Over Time. (Other label families keep the general palette.)
        return when (label) {
            "Large Cap" -> c(R.color.viz_s1)
            "Mid Cap" -> c(R.color.viz_s2)
            "Small Cap" -> c(R.color.viz_s3)
            "Cash" -> c(R.color.viz_s4)
            "Debt" -> c(R.color.viz_s5)
            "Multi Cap", "Flexi Cap" -> c(R.color.viz_s6)
            "International Equity" -> c(R.color.viz_s7)
            "Others", "Commodity" -> c(R.color.viz_neutral)
            "Unclassified" -> c(R.color.viz_neutral_soft)
            "Equity" -> palette[7]
            "Indian Equity" -> palette[0]
            // Any other/unclassified label falls back to a stable hash
            // of its own text, so it's at least consistent across the
            // donut and legend even without a dedicated slot.
            else -> palette[label.hashCode().and(Int.MAX_VALUE) % palette.size]
        }
    }
}
