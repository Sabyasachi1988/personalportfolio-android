package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import androidx.core.content.ContextCompat

/**
 * Renders one price history series as a simple line - full history,
 * always fully zoomed out. Deliberately does NOT replicate
 * ProgressionChartView's pinch-zoom/pan/daily-refetch machinery (see
 * that class's extensive doc comments on how hard-won and delicate that
 * behavior is) - this view exists for a quick "what has this looked
 * like over time" glance at a fund or benchmark's raw price/NAV level,
 * not for the detailed, zoomable portfolio-value analysis Progression
 * already does. If that ever turns out to be insufficient, the right
 * move is extracting Progression's zoom/pan logic into something
 * reusable, not duplicating it here from scratch.
 *
 * Single interaction: tap/drag anywhere to scrub - reports the nearest
 * point via [onPointScrubbed] so the hosting Activity can show its
 * date/value in a detail line, same "tap for detail" spirit as the
 * other charts in this app, without this view needing to know how
 * that's displayed.
 */
class PriceHistoryChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    var onPointScrubbed: ((point: PricePoint) -> Unit)? = null

    private var points: List<PricePoint> = emptyList()
    private var minPrice = 0.0
    private var maxPrice = 0.0
    private var scrubbedIndex = -1

    private val linePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 4f
        strokeCap = Paint.Cap.ROUND
        strokeJoin = Paint.Join.ROUND
        color = ContextCompat.getColor(context, R.color.colorPrimary)
    }
    private val crosshairPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 2f
        color = ContextCompat.getColor(context, R.color.colorNeutral)
    }
    private val dotPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
        color = ContextCompat.getColor(context, R.color.colorPrimary)
    }

    private val path = Path()
    private val paddingPx = 24f

    fun setPoints(newPoints: List<PricePoint>) {
        points = newPoints.sortedBy { it.date }
        minPrice = points.minOfOrNull { it.price } ?: 0.0
        maxPrice = points.maxOfOrNull { it.price } ?: 0.0
        scrubbedIndex = if (points.isNotEmpty()) points.size - 1 else -1
        invalidate()
        scrubbedIndex.takeIf { it >= 0 }?.let { onPointScrubbed?.invoke(points[it]) }
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        if (points.size < 2) return

        val w = width.toFloat()
        val h = height.toFloat()
        val chartLeft = paddingPx
        val chartRight = w - paddingPx
        val chartTop = paddingPx
        val chartBottom = h - paddingPx
        val chartWidth = (chartRight - chartLeft).coerceAtLeast(1f)
        val chartHeight = (chartBottom - chartTop).coerceAtLeast(1f)

        val priceRange = (maxPrice - minPrice).let { if (it <= 0.0) 1.0 else it }

        fun xFor(index: Int): Float = chartLeft + chartWidth * index / (points.size - 1).toFloat()
        fun yFor(price: Double): Float = chartTop + chartHeight * (1f - ((price - minPrice) / priceRange).toFloat())

        path.reset()
        points.forEachIndexed { i, p ->
            val x = xFor(i)
            val y = yFor(p.price)
            if (i == 0) path.moveTo(x, y) else path.lineTo(x, y)
        }
        canvas.drawPath(path, linePaint)

        if (scrubbedIndex in points.indices) {
            val x = xFor(scrubbedIndex)
            val y = yFor(points[scrubbedIndex].price)
            canvas.drawLine(x, chartTop, x, chartBottom, crosshairPaint)
            canvas.drawCircle(x, y, 8f, dotPaint)
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (points.isEmpty()) return false
        when (event.action) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE -> {
                val chartWidth = (width - 2 * paddingPx).coerceAtLeast(1f)
                val fraction = ((event.x - paddingPx) / chartWidth).coerceIn(0f, 1f)
                val index = (fraction * (points.size - 1)).toInt().coerceIn(0, points.size - 1)
                if (index != scrubbedIndex) {
                    scrubbedIndex = index
                    invalidate()
                    onPointScrubbed?.invoke(points[index])
                }
                return true
            }
        }
        return super.onTouchEvent(event)
    }
}
