package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.LinearGradient
import android.graphics.Paint
import android.graphics.Path
import android.graphics.Shader
import android.util.AttributeSet
import android.view.GestureDetector
import android.view.MotionEvent
import android.view.ScaleGestureDetector
import android.view.View
import androidx.core.content.ContextCompat
import java.text.SimpleDateFormat
import java.util.Locale
import kotlin.math.roundToInt

/**
 * Two-series line chart (Invested vs Value) over an ordered list of
 * ProgressionPoints, with a scrubber the person can drag or tap to browse
 * to any point - see ProgressionActivity for how the scrubbed index
 * drives the detail card above this view.
 *
 * X-axis is INDEX-spaced (each point gets equal horizontal spacing),
 * not calendar-time-spaced. The underlying series is already weekly
 * (see finance.WeeklyDates), so index-spacing and time-spacing agree
 * almost everywhere - they only diverge for the one appended "today"
 * point when today isn't a Monday, which would otherwise be compressed
 * into an illegibly thin final sliver on a true time axis. Index-spacing
 * keeps every point equally readable and tappable at the cost of slightly
 * misrepresenting that one interval's true width - judged the better
 * tradeoff for a touch-driven scrubber on a small screen.
 *
 * A row of date labels is always drawn along the bottom, independent of
 * the scrubber - previously the chart gave no sense of the time span
 * being shown until you actively touched it, which was disorienting
 * (is this 3 months or 20 years?). The labels answer that at a glance.
 *
 * Supports pinch-to-zoom: a two-finger pinch narrows/widens the visible
 * WINDOW of points (windowStart..windowEnd, both indices into the full
 * `points` list), re-scaling both axes to that window - zooming in on a
 * choppy recent stretch actually shows its shape instead of it being a
 * flat sliver against years of history. Double-tap resets to the full
 * range. onZoomChanged reports whether the view is currently zoomed, so
 * the hosting Activity can show/hide a "Reset zoom" affordance.
 */
class ProgressionChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    /** Called whenever the scrubbed index changes, including on initial layout (defaults to the last point). */
    var onScrub: ((index: Int) -> Unit)? = null

    /** Called whenever the zoom window changes, with true if currently zoomed in (not showing the full range). */
    var onZoomChanged: ((zoomed: Boolean) -> Unit)? = null

    private var points: List<ProgressionPoint> = emptyList()
    private var scrubbedIndex: Int = -1

    // Visible window into `points`, both inclusive. Defaults to the full
    // range; pinch-zoom narrows it, double-tap or resetZoom() restores it.
    private var windowStart: Int = 0
    private var windowEnd: Int = 0
    private val minWindowSpan = 3 // don't allow zooming in past ~4 visible points - a single-segment chart isn't useful

    private val density = context.resources.displayMetrics.density
    private val axisDateFormat = SimpleDateFormat("MMM ''yy", Locale.US)
    private val isoDateFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)

    private val investedColor = ContextCompat.getColor(context, R.color.colorSecondary)

    private val investedPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 3f * density
        strokeCap = Paint.Cap.ROUND
        strokeJoin = Paint.Join.ROUND
        color = investedColor
    }
    private val valuePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 5f * density
        strokeCap = Paint.Cap.ROUND
        strokeJoin = Paint.Join.ROUND
    }
    // Soft gradient fill under the Value line only, fading to nothing at
    // the baseline - purely decorative polish, doesn't affect any hit
    // testing or scrub math.
    private val valueFillPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
    }
    private val scrubLinePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1.5f * density
        color = ContextCompat.getColor(context, R.color.colorOnSurface)
        alpha = 100
    }
    private val scrubDotPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
    }
    private val gridPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1f * density
        color = ContextCompat.getColor(context, R.color.colorOnSurface)
        alpha = 28
    }
    private val axisLabelPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
        color = ContextCompat.getColor(context, R.color.colorNeutral)
        textSize = 11f * density
    }
    private val axisTickPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1.5f * density
        color = ContextCompat.getColor(context, R.color.colorNeutral)
        alpha = 140
    }
    private val gainColor = ContextCompat.getColor(context, R.color.colorGain)
    private val lossColor = ContextCompat.getColor(context, R.color.colorLoss)

    // Inset from the view's own edges so the scrub dot and stroke width
    // aren't clipped when a point sits exactly at the min/max.
    private val edgeInset = 12f
    private val topInset = 16f
    // Reserved strip at the bottom purely for axis tick marks + date text
    // - the value/invested lines never draw into this band, so labels
    // never overlap the data.
    private val axisBandHeight = 28f * density
    private val chartBottomInset = 12f + axisBandHeight

    private val scaleDetector = ScaleGestureDetector(context, ScaleListener())
    private val gestureDetector = GestureDetector(context, object : GestureDetector.SimpleOnGestureListener() {
        override fun onDoubleTap(e: MotionEvent): Boolean {
            resetZoom()
            return true
        }
    })

    init {
        isClickable = true
    }

    fun setPoints(newPoints: List<ProgressionPoint>) {
        points = newPoints
        windowStart = 0
        windowEnd = (points.size - 1).coerceAtLeast(0)
        scrubbedIndex = (points.size - 1).coerceAtLeast(-1)
        valuePaint.color = currentSeriesColor()
        invalidate()
        onZoomChanged?.invoke(false)
        if (scrubbedIndex >= 0) onScrub?.invoke(scrubbedIndex)
    }

    /** Programmatically move the scrubber, e.g. from an external slider/seek control. Clamped to the current visible window. */
    fun scrubTo(index: Int) {
        if (points.isEmpty()) return
        val clamped = index.coerceIn(windowStart, windowEnd)
        if (clamped == scrubbedIndex) return
        scrubbedIndex = clamped
        invalidate()
        onScrub?.invoke(scrubbedIndex)
    }

    /** Restores the full range after a pinch-zoom. Safe to call even when not zoomed. */
    fun resetZoom() {
        if (points.isEmpty()) return
        val wasZoomed = isZoomed()
        windowStart = 0
        windowEnd = points.size - 1
        scrubbedIndex = scrubbedIndex.coerceIn(windowStart, windowEnd)
        invalidate()
        if (wasZoomed) {
            onZoomChanged?.invoke(false)
            onScrub?.invoke(scrubbedIndex)
        }
    }

    private fun isZoomed(): Boolean = points.isNotEmpty() && (windowStart > 0 || windowEnd < points.size - 1)

    private fun currentSeriesColor(): Int {
        val last = points.lastOrNull() ?: return gainColor
        return if (last.gain >= 0) gainColor else lossColor
    }

    private fun xForIndex(index: Int): Float {
        val span = (windowEnd - windowStart).coerceAtLeast(1)
        val usableWidth = width - 2 * edgeInset
        return edgeInset + usableWidth * (index - windowStart) / span.toFloat()
    }

    private fun indexForX(x: Float): Int {
        val span = (windowEnd - windowStart).coerceAtLeast(1)
        val usableWidth = (width - 2 * edgeInset).coerceAtLeast(1f)
        val fraction = ((x - edgeInset) / usableWidth).coerceIn(0f, 1f)
        return (windowStart + fraction * span).roundToInt().coerceIn(windowStart, windowEnd)
    }

    private fun yForValue(v: Float, minV: Float, maxV: Float): Float {
        val usableHeight = height - chartBottomInset - topInset
        if (maxV <= minV) return height - chartBottomInset
        val fraction = (v - minV) / (maxV - minV)
        return height - chartBottomInset - fraction * usableHeight
    }

    /** Short "MMM ''yy" label for a "YYYY-MM-DD" point date; falls back to the raw string if it doesn't parse (shouldn't happen - dates always come from the bridge in this format). */
    private fun axisLabelFor(isoDate: String): String {
        val parsed = try { isoDateFormat.parse(isoDate) } catch (e: Exception) { null } ?: return isoDate
        return axisDateFormat.format(parsed)
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        val visibleCount = windowEnd - windowStart + 1
        if (points.size < 2 || visibleCount < 2) return

        var minV = Float.MAX_VALUE
        var maxV = -Float.MAX_VALUE
        for (i in windowStart..windowEnd) {
            val p = points[i]
            minV = minOf(minV, p.invested.toFloat(), p.value.toFloat())
            maxV = maxOf(maxV, p.invested.toFloat(), p.value.toFloat())
        }
        // Only anchor at zero for the full, unzoomed range - zooming in
        // on a narrow window is specifically to see that window's own
        // shape, which a forced-zero baseline would flatten right back
        // out again.
        if (!isZoomed() && minV > 0f) minV = 0f
        val headroom = (maxV - minV) * 0.08f
        maxV += headroom
        if (isZoomed()) minV -= headroom

        // Light horizontal gridlines at 25/50/75% of the value range, purely
        // as a reading aid - no labels on these, the numbers already live
        // in the detail card above.
        for (fraction in listOf(0.25f, 0.5f, 0.75f)) {
            val y = yForValue(minV + (maxV - minV) * fraction, minV, maxV)
            canvas.drawLine(edgeInset, y, width - edgeInset, y, gridPaint)
        }

        val investedPath = Path()
        val valuePath = Path()
        val fillPath = Path()
        var lastX = edgeInset
        for (i in windowStart..windowEnd) {
            val p = points[i]
            val x = xForIndex(i)
            val yInvested = yForValue(p.invested.toFloat(), minV, maxV)
            val yValue = yForValue(p.value.toFloat(), minV, maxV)
            if (i == windowStart) {
                investedPath.moveTo(x, yInvested)
                valuePath.moveTo(x, yValue)
                fillPath.moveTo(x, height - chartBottomInset)
                fillPath.lineTo(x, yValue)
            } else {
                investedPath.lineTo(x, yInvested)
                valuePath.lineTo(x, yValue)
                fillPath.lineTo(x, yValue)
            }
            lastX = x
        }
        fillPath.lineTo(lastX, height - chartBottomInset)
        fillPath.close()

        valueFillPaint.shader = LinearGradient(
            0f, topInset, 0f, height - chartBottomInset,
            (valuePaint.color and 0x00FFFFFF) or 0x33000000,
            (valuePaint.color and 0x00FFFFFF) or 0x00000000,
            Shader.TileMode.CLAMP
        )
        canvas.drawPath(fillPath, valueFillPaint)

        canvas.drawPath(investedPath, investedPaint)
        canvas.drawPath(valuePath, valuePaint)

        drawAxisLabels(canvas)

        if (scrubbedIndex in windowStart..windowEnd) {
            val x = xForIndex(scrubbedIndex)
            canvas.drawLine(x, topInset, x, height - chartBottomInset, scrubLinePaint)

            val p = points[scrubbedIndex]
            scrubDotPaint.color = investedPaint.color
            canvas.drawCircle(x, yForValue(p.invested.toFloat(), minV, maxV), 4f * density, scrubDotPaint)
            scrubDotPaint.color = valuePaint.color
            canvas.drawCircle(x, yForValue(p.value.toFloat(), minV, maxV), 5.5f * density, scrubDotPaint)
        }
    }

    /**
     * Draws evenly-spaced tick marks + short date labels along the
     * bottom band, scoped to the current visible window. Picks about one
     * label per ~56dp of width (fewer on a narrow phone, more on a
     * tablet) so labels never overlap each other, always including the
     * first and last visible point so the full shown span is legible
     * even between ticks.
     */
    private fun drawAxisLabels(canvas: Canvas) {
        val visibleCount = windowEnd - windowStart + 1
        if (visibleCount < 2) return
        val tickY = height - chartBottomInset + 6f * density
        val textY = height - 8f * density

        val approxLabelWidth = 56f * density
        val maxLabels = (width / approxLabelWidth).toInt().coerceIn(2, 6)
        val step = ((visibleCount - 1).toFloat() / (maxLabels - 1).coerceAtLeast(1)).coerceAtLeast(1f)

        val indices = LinkedHashSet<Int>()
        var i = windowStart.toFloat()
        while (i <= windowEnd) {
            indices.add(Math.round(i))
            i += step
        }
        indices.add(windowEnd)

        for (index in indices) {
            val x = xForIndex(index)
            canvas.drawLine(x, height - chartBottomInset, x, tickY, axisTickPaint)
            val label = axisLabelFor(points[index].date)
            val textWidth = axisLabelPaint.measureText(label)
            // Clamp horizontally so the first/last label's text doesn't
            // run off the view's edge, while the tick itself stays exactly
            // at its true x position.
            val textX = (x - textWidth / 2f).coerceIn(0f, width - textWidth)
            canvas.drawText(label, textX, textY, axisLabelPaint)
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (points.isEmpty()) return super.onTouchEvent(event)

        scaleDetector.onTouchEvent(event)
        gestureDetector.onTouchEvent(event)

        // While a pinch is actively in progress (2+ fingers down), don't
        // also treat the gesture as a single-finger scrub - the two
        // interpretations of the same touch stream would otherwise fight
        // each other every frame.
        if (scaleDetector.isInProgress || event.pointerCount > 1) {
            return true
        }

        when (event.action) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE -> {
                scrubToX(event.x)
                return true
            }
            MotionEvent.ACTION_UP -> {
                scrubToX(event.x)
                performClick()
                return true
            }
        }
        return super.onTouchEvent(event)
    }

    private fun scrubToX(x: Float) {
        val visibleCount = windowEnd - windowStart + 1
        if (visibleCount <= 1) return
        scrubTo(indexForX(x))
    }

    override fun performClick(): Boolean {
        super.performClick()
        return true
    }

    private inner class ScaleListener : ScaleGestureDetector.SimpleOnScaleGestureListener() {
        override fun onScale(detector: ScaleGestureDetector): Boolean {
            if (points.size < 2) return true
            val wasZoomed = isZoomed()
            val focusIndex = indexForX(detector.focusX)
            val currentSpan = (windowEnd - windowStart).coerceAtLeast(1)
            val maxSpan = points.size - 1
            var newSpan = (currentSpan / detector.scaleFactor).roundToInt().coerceIn(minWindowSpan.coerceAtMost(maxSpan), maxSpan)

            // Keep the pinch focus point at the same relative position
            // within the new window, rather than always re-centering -
            // this is what makes "pinch on the part you care about" feel
            // like it's zooming into that spot rather than the middle.
            val relPos = if (currentSpan > 0) (focusIndex - windowStart).toFloat() / currentSpan else 0.5f
            var newStart = (focusIndex - relPos * newSpan).roundToInt()
            var newEnd = newStart + newSpan
            if (newStart < 0) {
                newEnd -= newStart
                newStart = 0
            }
            if (newEnd > maxSpan) {
                newStart -= (newEnd - maxSpan)
                newEnd = maxSpan
            }
            newStart = newStart.coerceAtLeast(0)

            windowStart = newStart
            windowEnd = newEnd
            scrubbedIndex = scrubbedIndex.coerceIn(windowStart, windowEnd)
            invalidate()

            val nowZoomed = isZoomed()
            if (nowZoomed != wasZoomed) onZoomChanged?.invoke(nowZoomed)
            return true
        }
    }
}
