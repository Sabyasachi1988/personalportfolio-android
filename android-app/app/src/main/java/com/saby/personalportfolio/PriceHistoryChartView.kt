package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.util.AttributeSet
import android.view.GestureDetector
import android.view.MotionEvent
import android.view.ScaleGestureDetector
import android.view.View
import android.view.ViewConfiguration
import androidx.core.content.ContextCompat
import java.text.SimpleDateFormat
import java.util.Locale

/**
 * Renders one price history series as a simple line, with pinch-zoom,
 * pan, and axis chrome (y-axis price gridlines, x-axis date labels).
 * Deliberately does NOT replicate ProgressionChartView's weekly/daily
 * RESOLUTION-SWITCHING machinery (see that class's extensive doc
 * comments on how hard-won and delicate that behavior is) - this view's
 * data is always already fully daily (see ComputePriceHistory/
 * ComputeMultiSeriesHistory - a single fund/benchmark/overlay series is
 * a cheap lookup, unlike whole-portfolio Progression which re-solves
 * XIRR per checkpoint), so zoom here is a much simpler "show a narrower
 * index range of the same already-loaded points" operation, no
 * re-fetch or re-resolution needed at any zoom level.
 *
 * Interaction model:
 *  - At full zoom (the default, whole history visible): single-finger
 *    drag scrubs the crosshair, exactly as before this feature existed.
 *    Panning has no effect at full zoom (nothing further to pan to), so
 *    drag keeps its original meaning rather than being repurposed.
 *  - Pinch (2 fingers) zooms in/out at any time, centered on the pinch
 *    focal point, clamped between [MIN_WINDOW_POINTS] and the full
 *    series length.
 *  - Once zoomed in, single-finger drag PANS the visible window instead
 *    (there's now somewhere to pan to); a tap with negligible movement
 *    still scrubs the crosshair to that point.
 *  - Double-tap resets to full zoom.
 */
class PriceHistoryChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    companion object {
        private const val MIN_WINDOW_POINTS = 5
    }

    var onPointScrubbed: ((point: PricePoint) -> Unit)? = null

    private var points: List<PricePoint> = emptyList()
    private var windowStart = 0
    private var windowEnd = 0 // inclusive
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
    private val gridlinePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1.5f
        color = ContextCompat.getColor(context, R.color.colorNeutral)
        alpha = 60
    }
    private val axisLabelPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
        color = ContextCompat.getColor(context, R.color.colorNeutral)
        textSize = 24f
    }

    private val path = Path()

    // Chrome padding - wider on the left/bottom than the old fixed 24f
    // on every side, to make room for y-axis price labels and x-axis
    // date labels without the line itself running under them.
    private val chartPaddingTop = 20f
    private val chartPaddingRight = 16f
    private val chartPaddingLeft = 96f
    private val chartPaddingBottom = 44f

    private val dateStoredFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
    private val dateAxisFormat = SimpleDateFormat("d MMM ''yy", Locale.getDefault())

    private val scaleDetector = ScaleGestureDetector(context, ScaleListener())
    private val gestureDetector = GestureDetector(context, GestureListener())
    private val touchSlop = ViewConfiguration.get(context).scaledTouchSlop

    private var panLastX = 0f
    private var isPanning = false

    fun setPoints(newPoints: List<PricePoint>) {
        points = newPoints.sortedBy { it.date }
        windowStart = 0
        windowEnd = (points.size - 1).coerceAtLeast(0)
        scrubbedIndex = if (points.isNotEmpty()) points.size - 1 else -1
        invalidate()
        scrubbedIndex.takeIf { it >= 0 }?.let { onPointScrubbed?.invoke(points[it]) }
    }

    /** True once the visible window is narrower than the full series - see class doc comment. */
    private fun isZoomed(): Boolean = points.isNotEmpty() && (windowEnd - windowStart + 1) < points.size

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        if (windowEnd - windowStart < 1) return
        val visible = points.subList(windowStart, windowEnd + 1)

        val w = width.toFloat()
        val h = height.toFloat()
        val chartLeft = chartPaddingLeft
        val chartRight = w - chartPaddingRight
        val chartTop = chartPaddingTop
        val chartBottom = h - chartPaddingBottom
        val chartWidth = (chartRight - chartLeft).coerceAtLeast(1f)
        val chartHeight = (chartBottom - chartTop).coerceAtLeast(1f)

        val minPrice = visible.minOf { it.price }
        val maxPrice = visible.maxOf { it.price }
        val priceRange = (maxPrice - minPrice).let { if (it <= 0.0) 1.0 else it }

        fun xFor(localIndex: Int): Float = chartLeft + chartWidth * localIndex / (visible.size - 1).toFloat().coerceAtLeast(1f)
        fun yFor(price: Double): Float = chartTop + chartHeight * (1f - ((price - minPrice) / priceRange).toFloat())

        // Y-axis gridlines + price labels: min / mid / max of the
        // CURRENTLY VISIBLE window, not the full series - so zooming
        // into a narrower price range re-scales the axis to that
        // range, which is what makes zoom actually useful for reading
        // fine-grained moves rather than just stretching the same line.
        axisLabelPaint.textAlign = Paint.Align.RIGHT
        val midPrice = (minPrice + maxPrice) / 2.0
        listOf(maxPrice to chartTop, midPrice to chartTop + chartHeight / 2f, minPrice to chartBottom).forEach { (price, y) ->
            canvas.drawLine(chartLeft, y, chartRight, y, gridlinePaint)
            canvas.drawText(PricePerUnitFormatter.format(price, decimals = 2), chartLeft - 12f, y + 8f, axisLabelPaint)
        }

        // X-axis date labels: start / mid / end of the visible window.
        val labelY = chartBottom + 34f
        val firstLabel = formatAxisDate(visible.first().date)
        val lastLabel = formatAxisDate(visible.last().date)
        axisLabelPaint.textAlign = Paint.Align.LEFT
        canvas.drawText(firstLabel, chartLeft + 8f, labelY, axisLabelPaint)
        axisLabelPaint.textAlign = Paint.Align.RIGHT
        canvas.drawText(lastLabel, chartRight - 8f, labelY, axisLabelPaint)
        if (visible.size > 2) {
            val midDate = formatAxisDate(visible[visible.size / 2].date)
            axisLabelPaint.textAlign = Paint.Align.CENTER
            canvas.drawText(midDate, (chartLeft + chartRight) / 2f, labelY, axisLabelPaint)
        }

        path.reset()
        visible.forEachIndexed { i, p ->
            val x = xFor(i)
            val y = yFor(p.price)
            if (i == 0) path.moveTo(x, y) else path.lineTo(x, y)
        }
        canvas.drawPath(path, linePaint)

        val localScrub = scrubbedIndex - windowStart
        if (localScrub in visible.indices) {
            val x = xFor(localScrub)
            val y = yFor(visible[localScrub].price)
            canvas.drawLine(x, chartTop, x, chartBottom, crosshairPaint)
            canvas.drawCircle(x, y, 8f, dotPaint)
        }
    }

    private fun formatAxisDate(stored: String): String = try {
        dateAxisFormat.format(dateStoredFormat.parse(stored) ?: stored)
    } catch (e: Exception) {
        stored
    }

    private fun scrubAt(rawX: Float) {
        if (windowEnd - windowStart < 1) return
        val chartLeft = chartPaddingLeft
        val chartWidth = (width - chartPaddingLeft - chartPaddingRight).coerceAtLeast(1f)
        val fraction = ((rawX - chartLeft) / chartWidth).coerceIn(0f, 1f)
        val windowSize = windowEnd - windowStart
        val localIndex = (fraction * windowSize).toInt().coerceIn(0, windowSize)
        val index = windowStart + localIndex
        if (index != scrubbedIndex) {
            scrubbedIndex = index
            invalidate()
            onPointScrubbed?.invoke(points[index])
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (points.isEmpty()) return false
        scaleDetector.onTouchEvent(event)
        gestureDetector.onTouchEvent(event)

        // While a pinch is actively in progress, let ScaleGestureDetector
        // own the gesture exclusively - don't also scrub/pan from the
        // same motion events, which would fight the zoom.
        if (scaleDetector.isInProgress) {
            isPanning = false
            return true
        }

        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                panLastX = event.x
                isPanning = false
                if (!isZoomed()) {
                    scrubAt(event.x) // full-zoom: preserve original drag-to-scrub behavior immediately
                }
            }
            MotionEvent.ACTION_MOVE -> {
                if (isZoomed()) {
                    val dx = event.x - panLastX
                    if (isPanning || Math.abs(dx) > touchSlop) {
                        isPanning = true
                        panBy(dx)
                        panLastX = event.x
                    }
                } else {
                    scrubAt(event.x)
                }
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                if (isZoomed() && !isPanning) {
                    scrubAt(event.x) // a tap (no real drag) while zoomed still scrubs
                }
                isPanning = false
            }
        }
        return true
    }

    private fun panBy(dxPixels: Float) {
        val windowSize = windowEnd - windowStart
        val chartWidth = (width - chartPaddingLeft - chartPaddingRight).coerceAtLeast(1f)
        val indexDelta = (-dxPixels / chartWidth * windowSize).toInt()
        if (indexDelta == 0) return
        var newStart = windowStart + indexDelta
        var newEnd = windowEnd + indexDelta
        if (newStart < 0) {
            newEnd -= newStart
            newStart = 0
        }
        if (newEnd > points.size - 1) {
            val over = newEnd - (points.size - 1)
            newStart -= over
            newEnd = points.size - 1
        }
        newStart = newStart.coerceAtLeast(0)
        windowStart = newStart
        windowEnd = newEnd
        invalidate()
    }

    private inner class ScaleListener : ScaleGestureDetector.SimpleOnScaleGestureListener() {
        override fun onScale(detector: ScaleGestureDetector): Boolean {
            if (points.size <= MIN_WINDOW_POINTS) return true
            val windowSize = windowEnd - windowStart + 1
            // scaleFactor > 1 means fingers moved apart (zoom IN, so the
            // window should shrink) - hence dividing, not multiplying.
            val newWindowSize = (windowSize / detector.scaleFactor)
                .toInt()
                .coerceIn(MIN_WINDOW_POINTS, points.size)
            if (newWindowSize == windowSize) return true

            // Keep the focal point's underlying data index fixed on
            // screen while resizing, so zooming feels anchored to where
            // the fingers actually are rather than always re-centering.
            val chartWidth = (width - chartPaddingLeft - chartPaddingRight).coerceAtLeast(1f)
            val focalFraction = ((detector.focusX - chartPaddingLeft) / chartWidth).coerceIn(0f, 1f)
            val focalIndex = windowStart + focalFraction * windowSize

            var newStart = (focalIndex - focalFraction * newWindowSize).toInt()
            var newEnd = newStart + newWindowSize - 1
            if (newStart < 0) {
                newEnd -= newStart
                newStart = 0
            }
            if (newEnd > points.size - 1) {
                val over = newEnd - (points.size - 1)
                newStart -= over
                newEnd = points.size - 1
            }
            newStart = newStart.coerceAtLeast(0)
            windowStart = newStart
            windowEnd = newEnd
            invalidate()
            return true
        }
    }

    private inner class GestureListener : GestureDetector.SimpleOnGestureListener() {
        override fun onDoubleTap(e: MotionEvent): Boolean {
            windowStart = 0
            windowEnd = (points.size - 1).coerceAtLeast(0)
            invalidate()
            return true
        }
    }
}
