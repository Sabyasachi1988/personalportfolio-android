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
        val h = height.toFloat()
        val w = width.toFloat()
        val radius = h / 2f

        canvas.drawRoundRect(0f, 0f, w, h, radius, radius, trackPaint)

        val fillWidth = (actualPercent / 100f) * w
        if (fillWidth > 0f) {
            canvas.drawRoundRect(0f, 0f, fillWidth.coerceAtLeast(h), h, radius, radius, fillPaint)
        }

        val markerX = (targetPercent / 100f) * w
        canvas.drawRect((markerX - 3f).coerceAtLeast(0f), 0f, (markerX + 3f).coerceAtMost(w), h, markerPaint)
    }
}
