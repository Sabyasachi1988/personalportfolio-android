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
 */
class AutoDismissPopupView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : LinearLayout(context, attrs) {

    companion object {
        const val DEFAULT_DURATION_MS = 4000L
    }

    private val titleView: TextView
    private val messageView: TextView
    private val handler = Handler(Looper.getMainLooper())
    private val hideRunnable = Runnable { visibility = GONE }

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
     * Shows (or replaces) the card's content. accentColorRes tints the
     * title only - e.g. colorGain/colorLoss for a buy/sell marker, or
     * left unset (colorOnSurface) for a neutral message like the
     * Dashboard's period-gain explanation.
     */
    fun show(title: String, message: String, accentColorRes: Int = R.color.colorOnSurface, durationMs: Long = DEFAULT_DURATION_MS) {
        handler.removeCallbacks(hideRunnable) // cancel any pending auto-dismiss from a PREVIOUS tap - see class doc comment
        titleView.text = title
        titleView.setTextColor(ContextCompat.getColor(context, accentColorRes))
        messageView.text = message
        visibility = VISIBLE
        handler.postDelayed(hideRunnable, durationMs)
    }

    fun dismiss() {
        handler.removeCallbacks(hideRunnable)
        visibility = GONE
    }

    private fun dpToPx(dp: Int): Int = (dp * resources.displayMetrics.density).toInt()
}
