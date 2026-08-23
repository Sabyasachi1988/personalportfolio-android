package com.saby.personalportfolio

import android.view.View
import android.view.ViewGroup

/**
 * Horizontally-scrolling card rows (Allocation's fund/segment detail
 * cards) use a fixed 220dp card width so several cards preview
 * partially at the row's edge, hinting "swipe for more". But with only
 * ONE card, a fixed narrow width just leaves it floating with a large
 * empty gap beside it (reported directly: a single card "ends at the
 * middle of the page") - there's nothing to hint at scrolling toward,
 * so the fixed width has no purpose in that case. This makes the card
 * fill the row instead whenever it's the only one.
 */
object HorizontalCardWidth {
    private const val FIXED_WIDTH_DP = 220

    fun apply(view: View, isOnlyCard: Boolean) {
        val density = view.resources.displayMetrics.density
        val params = view.layoutParams ?: return
        params.width = if (isOnlyCard) {
            ViewGroup.LayoutParams.MATCH_PARENT
        } else {
            (FIXED_WIDTH_DP * density).toInt()
        }
        view.layoutParams = params
    }
}
