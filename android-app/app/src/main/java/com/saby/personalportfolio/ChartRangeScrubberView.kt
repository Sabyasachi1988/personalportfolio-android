package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.RectF
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import androidx.core.content.ContextCompat

/**
 * A thin "minimap" scrollbar below a zoomed chart (PriceHistoryChartView
 * or OverlayChartView) - shows the current visible window's position and
 * width against the FULL data range, and lets a person drag it to move
 * the window WITHOUT touching the chart itself.
 *
 * This exists because dragging directly on the chart when it's zoomed in
 * already means "pan" (see PriceHistoryChartView/OverlayChartView's own
 * pan gesture) - which is exactly right for exploring, but means there's
 * no way to just read a value at one specific point without also risking
 * nudging the pan position. A confirmed real report: wanting to check
 * NAV/return at a certain time while zoomed in, but any touch on the
 * chart to get there also moves it. This scrubber gives a second,
 * independent way to move the window - drag the highlighted band here,
 * and use taps/scrub directly on the chart for reading values without
 * worrying about accidentally panning.
 *
 * Deliberately hidden (see [setRange]) whenever the window covers the
 * WHOLE series - there's nothing to scroll independently of in that
 * case, and showing a full-width, non-interactive band would just be
 * visual clutter.
 */
class ChartRangeScrubberView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    /** Invoked with the new (startIndex, endIndex) once the person drags the band to a new position - endIndex - startIndex is always preserved (dragging moves the window, it doesn't resize it). */
    var onRangeDragged: ((startIndex: Int, endIndex: Int) -> Unit)? = null

    private var totalCount = 0
    private var windowStart = 0
    private var windowEnd = 0

    private val trackPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = ContextCompat.getColor(context, R.color.colorSurfaceVariant)
    }
    private val bandPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = ContextCompat.getColor(context, R.color.colorSecondary)
        alpha = 200
    }
    private val trackRect = RectF()
    private val bandRect = RectF()
    private val cornerRadius = 8f

    private var dragStartX = 0f
    private var dragStartWindowStart = 0
    private var isDragging = false

    init {
        isClickable = true
    }

    /**
     * Sets the full data-point count and the currently visible window
     * (inclusive indices), same convention as PriceHistoryChartView's
     * own windowStart/windowEnd. Call on every zoom/pan/setPoints, same
     * as invalidate() would be called on the chart itself. Automatically
     * hides this view when the window covers the whole series - see
     * class doc comment.
     */
    fun setRange(total: Int, startIndex: Int, endIndex: Int) {
        totalCount = total
        windowStart = startIndex
        windowEnd = endIndex
        visibility = if (total > 0 && (endIndex - startIndex + 1) < total) VISIBLE else GONE
        invalidate()
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        if (totalCount <= 0) return

        trackRect.set(0f, 0f, width.toFloat(), height.toFloat())
        canvas.drawRoundRect(trackRect, cornerRadius, cornerRadius, trackPaint)

        val startFraction = windowStart.toFloat() / totalCount.toFloat()
        val endFraction = (windowEnd + 1).toFloat() / totalCount.toFloat()
        bandRect.set(
            width * startFraction, 0f,
            width * endFraction, height.toFloat()
        )
        canvas.drawRoundRect(bandRect, cornerRadius, cornerRadius, bandPaint)
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (totalCount <= 0) return false
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                dragStartX = event.x
                dragStartWindowStart = windowStart
                isDragging = true
                parent?.requestDisallowInterceptTouchEvent(true)
                return true
            }
            MotionEvent.ACTION_MOVE -> {
                if (!isDragging) return false
                val dx = event.x - dragStartX
                val windowSize = windowEnd - windowStart
                val indexDelta = (dx / width.toFloat() * totalCount).toInt()
                var newStart = dragStartWindowStart + indexDelta
                var newEnd = newStart + windowSize
                if (newStart < 0) {
                    newEnd -= newStart
                    newStart = 0
                }
                if (newEnd > totalCount - 1) {
                    val over = newEnd - (totalCount - 1)
                    newStart -= over
                    newEnd = totalCount - 1
                }
                newStart = newStart.coerceAtLeast(0)
                if (newStart != windowStart || newEnd != windowEnd) {
                    windowStart = newStart
                    windowEnd = newEnd
                    invalidate()
                    onRangeDragged?.invoke(newStart, newEnd)
                }
                return true
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                isDragging = false
                parent?.requestDisallowInterceptTouchEvent(false)
                return true
            }
        }
        return false
    }
}
