package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.util.AttributeSet
import android.view.View

class TargetProgressBarView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    private var actualPercent = 0f
    private var targetPercent = 0f

    private val trackPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
    private val fillPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
    private val markerPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }

    fun setValues(actual: Float, target: Float, overColor: Int, underColor: Int, trackColor: Int, markerColor: Int) {
        // Scale against whichever of actual/target is larger (with
        // headroom), rather than a fixed 0-100 - a fund at 3% cash
        // target vs 8% actual should still show a visually meaningful
        // bar, not a sliver near zero on a scale sized for 100%.
        val maxVal = maxOf(actual, target, 1f) * 1.15f
        actualPercent = (actual / maxVal * 100f).coerceIn(0f, 100f)
        targetPercent = (target / maxVal * 100f).coerceIn(0f, 100f)
        trackPaint.color = trackColor
        fillPaint.color = if (actual >= target) overColor else underColor
        markerPaint.color = markerColor
        invalidate()
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        val fullHeight = height.toFloat()
        val w = width.toFloat()

        // The visual bar is shorter than the View's own height, leaving
        // room above/below for the marker to genuinely protrude within
        // real (unclipped) bounds - drawing outside a View's own bounds
        // gets clipped by Android, so this can't be done by drawing past
        // the edges of a bar that fills the whole View.
        val barHeight = fullHeight * 0.55f
        val barTop = (fullHeight - barHeight) / 2f
        val barBottom = barTop + barHeight
        val radius = barHeight / 2f

        canvas.drawRoundRect(0f, barTop, w, barBottom, radius, radius, trackPaint)

        val fillWidth = (actualPercent / 100f) * w
        if (fillWidth > 0f) {
            canvas.drawRoundRect(0f, barTop, fillWidth.coerceAtLeast(barHeight), barBottom, radius, radius, fillPaint)
        }

        // The marker spans the View's full height (taller than the bar
        // itself), so it always reads as a distinct "goal line" whether
        // it lands on empty track (underweight) or inside the solid
        // fill (overweight) - a marker confined to the bar's own height
        // becomes visually indistinguishable when it sits inside a
        // similarly-toned fill.
        val markerX = (targetPercent / 100f) * w
        canvas.drawRect(
            (markerX - 3f).coerceAtLeast(0f), 0f,
            (markerX + 3f).coerceAtMost(w), fullHeight,
            markerPaint
        )
    }
}
