package com.saby.personalportfolio

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.util.AttributeSet
import android.view.ViewGroup
import android.widget.HorizontalScrollView
import kotlin.math.abs

/**
 * A plain HorizontalScrollView never settles on a card boundary - the
 * person can lift their finger anywhere mid-scroll, leaving one card
 * fully visible and its neighbor half-cut-off. Side by side, that reads
 * as the cards (and the donut charts inside them) being "offset" from
 * each other rather than cleanly aligned for comparison - this was a
 * confirmed real report about the Dashboard's 3-donut row specifically.
 *
 * This snaps to whichever direct child is closest to the scroll
 * position once scrolling has settled (no native "scroll ended" event
 * exists on this view, so settlement is detected the standard way: a
 * short delay after the last onScrollChanged with the position
 * unchanged). Deliberately NOT a RecyclerView + SnapHelper rewrite -
 * this container has a small, fixed number of already-inflated,
 * individually-referenced child views (see activity_main.xml's
 * dashboardDonutMarketCap/Origin/Class, bound directly by ID in
 * MainActivity), and swapping to a RecyclerView-adapter model would
 * require restructuring that binding for no behavioral gain over a
 * simple settle-and-snap layered onto the existing container.
 */
class SnappingHorizontalScrollView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : HorizontalScrollView(context, attrs) {

    private val handler = Handler(Looper.getMainLooper())
    private var lastScrollX = -1
    private val settleCheck = object : Runnable {
        override fun run() {
            if (scrollX == lastScrollX) {
                snapToNearestChild()
            } else {
                lastScrollX = scrollX
                handler.postDelayed(this, SETTLE_DELAY_MS)
            }
        }
    }

    override fun onScrollChanged(l: Int, t: Int, oldl: Int, oldt: Int) {
        super.onScrollChanged(l, t, oldl, oldt)
        handler.removeCallbacks(settleCheck)
        lastScrollX = l
        handler.postDelayed(settleCheck, SETTLE_DELAY_MS)
    }

    private fun snapToNearestChild() {
        val row = getChildAt(0) as? ViewGroup ?: return
        if (row.childCount == 0) return

        var closestChild = row.getChildAt(0)
        var closestDistance = Int.MAX_VALUE
        for (i in 0 until row.childCount) {
            val child = row.getChildAt(i)
            val distance = abs(child.left - scrollX)
            if (distance < closestDistance) {
                closestDistance = distance
                closestChild = child
            }
        }
        smoothScrollTo(closestChild.left, 0)
    }

    companion object {
        // Long enough that a normal continuous fling never falsely
        // "settles" mid-motion (onScrollChanged fires every frame while
        // still moving, resetting this timer each time), short enough
        // that the snap feels immediate once a finger-up or fling
        // genuinely stops.
        private const val SETTLE_DELAY_MS = 100L
    }
}
