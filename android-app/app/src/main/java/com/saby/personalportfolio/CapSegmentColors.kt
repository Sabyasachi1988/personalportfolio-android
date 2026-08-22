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
        return when (label) {
            "Large Cap" -> palette[0]
            "Mid Cap" -> palette[1]
            "Small Cap" -> palette[2]
            "Cash" -> palette[3]
            "Multi Cap" -> palette[4]
            "Debt" -> palette[5]
            "Commodity" -> palette[6]
            // Any other/unclassified label falls back to a stable hash
            // of its own text, so it's at least consistent across the
            // donut and legend even without a dedicated slot.
            else -> palette[label.hashCode().and(Int.MAX_VALUE) % palette.size]
        }
    }
}
