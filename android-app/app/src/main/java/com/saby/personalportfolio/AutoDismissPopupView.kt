package com.saby.personalportfolio

import android.content.Context
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.os.Handler
import android.os.Looper
import android.util.AttributeSet
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.content.ContextCompat

/**
 * A small, self-dismissing info card - the replacement for a
 * manual-close AlertDialog on a quick informational tap (a transaction
 * marker on a chart, a Dashboard period-gain chip). Confirmed real
 * complaint: the AlertDialog version needed an explicit "OK" tap for
 * information that's meant to be glanced at, not acted on.
 *
 * Deliberately NOT Android's built-in Toast - MainActivity's period-gain
 * chip popup was ALREADY migrated away from Toast once before (see that
 * call site's own doc comment) after a confirmed real bug: modern
 * Android silently TRUNCATES a long Toast to one line, and the added
 * date-range text sat past that cutoff and never became visible. This
 * view shows the full message with no such limit, while still
 * auto-dismissing like a Toast would.
 *
 * Meant to be added ONCE per screen (as a FrameLayout overlay child,
 * initially GONE) and reused via repeated show() calls - NOT
 * re-created per tap. Tapping a second marker/chip while this is still
 * showing calls show() again, which cancels the pending auto-hide,
 * immediately swaps in the new content, and restarts the timer - so the
 * first tap's information never lingers or delays the second's.
 *
 * Two ways to trigger it, per explicit feedback that the original fixed
 * 4-second display felt too long:
 *  - show(): a quick tap - displays for the user's configured duration
 *    (see PopupDurationPreference) then auto-hides on its own.
 *  - showPersistent() + dismissAfterLinger(): a press-and-hold - shows
 *    with NO timer while the finger is down (showPersistent), then once
 *    released (dismissAfterLinger) stays up for the same configured
 *    duration more before going away - "as we leave the point... stays
 *    for half a second to one second" was the original ask; the exact
 *    duration is now the person's own choice in Settings, not fixed.
 */
class AutoDismissPopupView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : LinearLayout(context, attrs) {

    private val titleView: TextView
    private val messageView: TextView
    private val handler = Handler(Looper.getMainLooper())
    private val hideRunnable = Runnable { visibility = GONE }

    // Read once per show/dismissAfterLinger call, not cached at
    // construction - so a change made in Settings mid-session (the
    // person opens Settings, changes the duration, comes back) takes
    // effect on the very next popup without needing this view
    // recreated.
    private val configuredDurationMs: Long
        get() = PopupDurationPreference.durationMs(context)

    init {
        orientation = VERTICAL
        val hPad = dpToPx(16)
        val vPad = dpToPx(12)
        setPadding(hPad, vPad, hPad, vPad)
        elevation = dpToPx(6).toFloat()
        background = GradientDrawable().apply {
            cornerRadius = dpToPx(14).toFloat()
            setColor(ContextCompat.getColor(context, R.color.colorSurfaceVariant))
        }
        visibility = GONE
        // A tap on the card itself dismisses it immediately, for anyone
        // who's read it before the timer runs out and wants it gone.
        isClickable = true
        setOnClickListener { dismiss() }

        titleView = TextView(context).apply {
            textSize = 16f
            setTypeface(typeface, Typeface.BOLD)
        }
        messageView = TextView(context).apply {
            textSize = 14f
            setTextColor(ContextCompat.getColor(context, R.color.colorOnSurface))
            setLineSpacing(dpToPx(3).toFloat(), 1f)
        }
        addView(titleView, LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT))
        addView(
            messageView,
            LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT).apply { topMargin = dpToPx(6) }
        )
    }

    /**
     * Shows (or replaces) the card's content for a quick tap - displays
     * for LINGER_DURATION_MS then auto-hides. accentColorRes tints the
     * title only - e.g. colorGain/colorLoss for a buy/sell marker, or
     * left unset (colorOnSurface) for a neutral message like the
     * Dashboard's period-gain explanation. anchorX/anchorY, if given,
     * position the card near that point in the PARENT's own coordinate
     * space - see positionNear's doc comment. Left unset (null),
     * whatever position was last set stays (or the XML-declared
     * default gravity, on first use).
     */
    fun show(
        title: String, message: String, accentColorRes: Int = R.color.colorOnSurface,
        anchorX: Float? = null, anchorY: Float? = null
    ) {
        setContent(title, message, accentColorRes)
        if (anchorX != null && anchorY != null) positionNear(anchorX, anchorY)
        visibility = VISIBLE
        handler.removeCallbacks(hideRunnable)
        handler.postDelayed(hideRunnable, configuredDurationMs)
    }

    /**
     * Shows the card's content with NO auto-dismiss timer - for a
     * press-and-hold, where the popup should stay up for exactly as
     * long as the finger stays down. Call dismissAfterLinger() once the
     * hold ends to start the short auto-hide timer.
     */
    fun showPersistent(title: String, message: String, accentColorRes: Int, anchorX: Float, anchorY: Float) {
        setContent(title, message, accentColorRes)
        positionNear(anchorX, anchorY)
        visibility = VISIBLE
        handler.removeCallbacks(hideRunnable) // no timer while held
    }

    /** Starts the short auto-hide timer - call once a showPersistent() hold has ended (finger lifted). */
    fun dismissAfterLinger() {
        handler.removeCallbacks(hideRunnable)
        handler.postDelayed(hideRunnable, configuredDurationMs)
    }

    private fun setContent(title: String, message: String, accentColorRes: Int) {
        handler.removeCallbacks(hideRunnable) // cancel any pending auto-dismiss from a PREVIOUS tap/hold - see class doc comment
        titleView.text = title
        titleView.setTextColor(ContextCompat.getColor(context, accentColorRes))
        messageView.text = message
    }

    /**
     * Positions this card near (anchorX, anchorY) - the PARENT's own
     * coordinate space (a chart view filling its FrameLayout parent at
     * (0,0) means the chart's own touch coordinates ARE parent-relative
     * coordinates already, no translation needed at the call site).
     * Deferred to the NEXT layout pass (view.post) because the card's
     * width/height for THIS content aren't known until Android actually
     * measures/lays out the new text - reading width/height immediately
     * after setting VISIBLE would still return the previous frame's
     * (possibly stale, possibly zero) size.
     *
     * Prefers appearing ABOVE the touched point (a tooltip pointing
     * down at it) since a finger touching the chart is usually resting
     * below/on top of where a card below it would need to go anyway;
     * flips to BELOW when there isn't enough room above (a marker near
     * the very top of the chart). Horizontally centered on the touch
     * point, clamped so it never runs off either edge of the parent.
     */
    private fun positionNear(anchorX: Float, anchorY: Float) {
        post {
            val parentView = parent as? android.view.ViewGroup ?: return@post
            val gap = dpToPx(14)
            var left = anchorX - width / 2f
            left = left.coerceIn(0f, (parentView.width - width).coerceAtLeast(0).toFloat())
            var top = anchorY - height - gap
            if (top < 0f) {
                top = anchorY + gap // not enough room above - flip below the point instead
            }
            top = top.coerceIn(0f, (parentView.height - height).coerceAtLeast(0).toFloat())
            x = left
            y = top
        }
    }

    fun dismiss() {
        handler.removeCallbacks(hideRunnable)
        visibility = GONE
    }

    private fun dpToPx(dp: Int): Int = (dp * resources.displayMetrics.density).toInt()
}
