package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.RectF
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import kotlin.math.atan2
import kotlin.math.hypot

class DonutChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    data class Slice(val label: String, val percent: Float, val color: Int? = null)

    /** Called when the person taps a slice, with that slice's label and percent. */
    var onSliceTapped: ((label: String, percent: Float) -> Unit)? = null

    private var slices: List<Slice> = emptyList()
    // Precomputed [startAngle, endAngle) for each slice in `slices`, in
    // the same -90-based coordinate system used for drawing - kept
    // separate from drawing so hit-testing matches what's drawn exactly.
    private var sliceRanges: List<Pair<Float, Float>> = emptyList()

    private val arcPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE }
    private val emptyPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        color = 0x33888888
    }
    private val rectF = RectF()

    private val sliceColors: List<Int> by lazy { ChartColors.palette(context) }

    // Small visual gap between adjacent slices (degrees) - makes slice
    // boundaries readable even when two slices land on similar colors,
    // rather than relying on color contrast alone.
    private val gapDegrees = 3f

    private var currentStrokeWidth = 0f

    fun setSlices(newSlices: List<Slice>) {
        // Only positive-percent slices are drawable; anything at 0%
        // would just be an invisible zero-length arc anyway.
        slices = newSlices.filter { it.percent > 0f }

        val ranges = mutableListOf<Pair<Float, Float>>()
        var startAngle = -90f
        for (slice in slices) {
            val sweep = (slice.percent / 100f) * 360f
            ranges.add(startAngle to (startAngle + sweep))
            startAngle += sweep
        }
        sliceRanges = ranges

        invalidate()
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)

        currentStrokeWidth = width.coerceAtMost(height) * 0.18f
        arcPaint.strokeWidth = currentStrokeWidth
        emptyPaint.strokeWidth = currentStrokeWidth

        val inset = currentStrokeWidth / 2f + 4f
        rectF.set(inset, inset, width - inset, height - inset)

        if (slices.isEmpty()) {
            canvas.drawArc(rectF, 0f, 360f, false, emptyPaint)
            return
        }

        for ((index, range) in sliceRanges.withIndex()) {
            val (start, end) = range
            val fullSweep = end - start
            // Guard tiny slices: never let the gap eat the whole slice.
            val gap = gapDegrees.coerceAtMost(fullSweep * 0.3f)
            arcPaint.color = slices[index].color ?: sliceColors[index % sliceColors.size]
            canvas.drawArc(rectF, start + gap / 2f, fullSweep - gap, false, arcPaint)
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (event.action != MotionEvent.ACTION_UP || slices.isEmpty()) {
            return super.onTouchEvent(event)
        }

        val centerX = width / 2f
        val centerY = height / 2f
        val dx = event.x - centerX
        val dy = event.y - centerY
        val distanceFromCenter = hypot(dx, dy)

        // Only counts as a hit on the ring itself, not the empty middle
        // or outside the chart entirely.
        val ringRadius = rectF.width() / 2f
        val ringHalfWidth = currentStrokeWidth / 2f + 8f
        if (distanceFromCenter < ringRadius - ringHalfWidth || distanceFromCenter > ringRadius + ringHalfWidth) {
            return super.onTouchEvent(event)
        }

        // atan2 gives degrees from the positive x-axis; convert to the
        // same -90-based, clockwise-from-top system used for drawing.
        var angle = Math.toDegrees(atan2(dy.toDouble(), dx.toDouble())).toFloat()
        while (angle < -90f) angle += 360f
        while (angle >= 270f) angle -= 360f

        for ((index, range) in sliceRanges.withIndex()) {
            val (start, end) = range
            if (angle >= start && angle < end) {
                performClick()
                onSliceTapped?.invoke(slices[index].label, slices[index].percent)
                return true
            }
        }
        return super.onTouchEvent(event)
    }

    override fun performClick(): Boolean {
        super.performClick()
        return true
    }
}
