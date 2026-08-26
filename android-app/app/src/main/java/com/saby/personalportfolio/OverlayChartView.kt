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

/** One series being compared - a color is assigned by the hosting Activity from the existing 14-color chart palette (see colors.xml). */
data class OverlaySeries(
    val seriesId: String,
    val name: String,
    val isBenchmark: Boolean,
    val points: List<PricePoint>,
    val color: Int
)

/**
 * Multi-series comparison chart: each series is normalized to a common
 * base of 100 at a BASE DATE, so "which fund/index actually performed
 * better over this stretch" is readable directly from the lines'
 * relative heights, regardless of how different the funds' raw NAVs are.
 *
 * Series can have different date coverage (a benchmark added recently
 * vs. a fund held for years) - rather than assuming exact date
 * alignment, this builds a UNION of every distinct date across all
 * selected series as a shared index axis, and carries each series'
 * price forward across dates it doesn't have an exact point for (same
 * carry-forward convention as store.PriceAsOf/finance.priceOnOrBefore
 * on the Go side) - a series simply has no line before its own first
 * real data point.
 *
 * Base-date behavior (see [setLockBaseDate]):
 *  - Unlocked (default): the base date auto-tracks the LEFT EDGE of
 *    whatever window is currently visible, so zooming/panning into a
 *    stretch re-normalizes every series to start at 100 for that
 *    stretch - "zoom into 2021" and "rebase to 2021" are the same
 *    gesture, per Saby's own confirmed design.
 *  - Locked: the base date is pinned wherever it was at the moment of
 *    locking, and stays there through further zoom/pan until unlocked.
 *
 * Gesture model is the same pinch-zoom/pan/double-tap-reset scheme as
 * PriceHistoryChartView (see that class's doc comment) - deliberately
 * NOT sharing a base class with it, since the index space here (a
 * union of dates with carry-forward) is a different enough concept from
 * a single series' own point list that a shared abstraction would add
 * more indirection than it'd save for two fairly small views.
 */
class OverlayChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    companion object {
        private const val MIN_WINDOW_POINTS = 5
    }

    /** Invoked on every scrub/zoom/pan with the scrubbed union-date and each series' normalized value there (null = no data yet for that series). */
    var onScrubbed: ((date: String, values: List<Pair<OverlaySeries, Double?>>) -> Unit)? = null

    private var series: List<OverlaySeries> = emptyList()
    private var unionDates: List<String> = emptyList()
    // carried[s][i] = series[s]'s price carried forward to unionDates[i], or null if series[s] has no data yet at that date.
    private var carried: List<List<Double?>> = emptyList()

    private var windowStart = 0
    private var windowEnd = 0 // inclusive
    private var scrubbedIndex = -1
    private var lockBaseDate = false
    private var lockedBaseIndex = 0

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
    private val crosshairPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 2f
        color = ContextCompat.getColor(context, R.color.colorNeutral)
    }
    private val baseLinePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1.5f
        color = ContextCompat.getColor(context, R.color.colorNeutral)
        pathEffect = android.graphics.DashPathEffect(floatArrayOf(10f, 8f), 0f)
    }

    private val linePath = Path()
    private val chartPaddingTop = 20f
    private val chartPaddingRight = 16f
    private val chartPaddingLeft = 80f
    private val chartPaddingBottom = 44f

    private val dateStoredFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
    private val dateAxisFormat = SimpleDateFormat("d MMM ''yy", Locale.getDefault())

    private val scaleDetector = ScaleGestureDetector(context, ScaleListener())
    private val gestureDetector = GestureDetector(context, GestureListener())
    private val touchSlop = ViewConfiguration.get(context).scaledTouchSlop
    private var panLastX = 0f
    private var isPanning = false

    fun setSeries(newSeries: List<OverlaySeries>) {
        series = newSeries
        val dateSet = sortedSetOf<String>()
        newSeries.forEach { s -> s.points.forEach { dateSet.add(it.date) } }
        unionDates = dateSet.toList()

        carried = newSeries.map { s ->
            val byDate = s.points.associate { it.date to it.price }
            var last: Double? = null
            unionDates.map { d ->
                byDate[d]?.let { last = it }
                last
            }
        }

        windowStart = 0
        windowEnd = (unionDates.size - 1).coerceAtLeast(0)
        lockedBaseIndex = windowStart
        scrubbedIndex = windowEnd
        invalidate()
        fireScrubCallback()
    }

    /** See class doc comment's Base-date behavior section. */
    fun setLockBaseDate(locked: Boolean) {
        lockBaseDate = locked
        if (locked) {
            lockedBaseIndex = windowStart
        }
        invalidate()
        fireScrubCallback()
    }

    private fun baseIndex(): Int = if (lockBaseDate) lockedBaseIndex.coerceIn(0, unionDates.size - 1) else windowStart

    /**
     * The price used as "100" for one series. Deliberately NOT just
     * carried[seriesIndex][baseIndex()] - a series added more recently
     * than another (e.g. a benchmark quick-added last week, compared
     * against a fund held since 2013) has no data at all near the
     * WHOLE-selection's earliest date, so pinning every series' base to
     * the exact same literal index left that series with a permanently
     * null base price and NO LINE EVER DRAWN, for its entire history -
     * a real, confirmed bug (reported as "5 picked, only 3 lines show").
     * Instead, each series bases off its own FIRST available price at
     * or after the nominal base date - so a shorter-history series
     * simply starts its line (at 100) from wherever its own data
     * begins, which is the standard, expected behavior for this kind of
     * comparison chart when series don't share a common start date.
     */
    private fun basePriceFor(seriesIndex: Int): Double? {
        val arr = carried.getOrNull(seriesIndex) ?: return null
        val base = baseIndex()
        for (i in base until arr.size) {
            val v = arr[i]
            if (v != null) return v
        }
        return null
    }

    private fun normalizedAt(seriesIndex: Int, unionIndex: Int, basePrice: Double?): Double? {
        val bp = basePrice ?: return null
        val price = carried.getOrNull(seriesIndex)?.getOrNull(unionIndex) ?: return null
        if (bp <= 0.0) return null
        return price / bp * 100.0
    }

    private fun isZoomed(): Boolean = unionDates.isNotEmpty() && (windowEnd - windowStart + 1) < unionDates.size

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        if (unionDates.isEmpty() || windowEnd - windowStart < 1 || series.isEmpty()) return

        // Computed ONCE per draw (not per point) - basePriceFor does a
        // forward scan, so calling it per-point-per-series would be
        // needlessly repeated work across the whole visible window.
        val basePrices = series.indices.map { basePriceFor(it) }

        val w = width.toFloat()
        val h = height.toFloat()
        val chartLeft = chartPaddingLeft
        val chartRight = w - chartPaddingRight
        val chartTop = chartPaddingTop
        val chartBottom = h - chartPaddingBottom
        val chartWidth = (chartRight - chartLeft).coerceAtLeast(1f)
        val chartHeight = (chartBottom - chartTop).coerceAtLeast(1f)
        val windowSize = windowEnd - windowStart

        // Visible normalized-value range across every series in the
        // current window, so the y-axis always fits whichever series is
        // furthest from 100 right now (not a fixed 0-200 scale, which
        // would waste most of the chart height for close-run comparisons).
        var minVal = Double.MAX_VALUE
        var maxVal = -Double.MAX_VALUE
        for (s in series.indices) {
            for (i in windowStart..windowEnd) {
                val v = normalizedAt(s, i, basePrices[s]) ?: continue
                if (v < minVal) minVal = v
                if (v > maxVal) maxVal = v
            }
        }
        if (minVal == Double.MAX_VALUE) return // nothing to draw yet in this window
        if (minVal == maxVal) {
            minVal -= 1.0
            maxVal += 1.0
        }
        val valRange = (maxVal - minVal).coerceAtLeast(0.01)

        fun xFor(localIndex: Int): Float = chartLeft + chartWidth * localIndex / windowSize.toFloat().coerceAtLeast(1f)
        fun yFor(v: Double): Float = chartTop + chartHeight * (1f - ((v - minVal) / valRange).toFloat())

        // Y-axis gridlines + labels (index values, not currency - this
        // is a normalized-to-100 comparison, not a price chart).
        axisLabelPaint.textAlign = Paint.Align.RIGHT
        val midVal = (minVal + maxVal) / 2.0
        listOf(maxVal to chartTop, midVal to chartTop + chartHeight / 2f, minVal to chartBottom).forEach { (v, y) ->
            canvas.drawLine(chartLeft, y, chartRight, y, gridlinePaint)
            canvas.drawText(String.format(Locale.getDefault(), "%.0f", v), chartLeft - 12f, y + 8f, axisLabelPaint)
        }

        // Dashed base line at 100, when it falls within the visible range.
        if (100.0 in minVal..maxVal) {
            canvas.drawLine(chartLeft, yFor(100.0), chartRight, yFor(100.0), baseLinePaint)
        }

        // X-axis date labels.
        val labelY = chartBottom + 34f
        axisLabelPaint.textAlign = Paint.Align.LEFT
        canvas.drawText(formatAxisDate(unionDates[windowStart]), chartLeft + 8f, labelY, axisLabelPaint)
        axisLabelPaint.textAlign = Paint.Align.RIGHT
        canvas.drawText(formatAxisDate(unionDates[windowEnd]), chartRight - 8f, labelY, axisLabelPaint)
        if (windowSize > 2) {
            axisLabelPaint.textAlign = Paint.Align.CENTER
            canvas.drawText(formatAxisDate(unionDates[windowStart + windowSize / 2]), (chartLeft + chartRight) / 2f, labelY, axisLabelPaint)
        }

        // One line per series - gaps (null) simply break the path rather
        // than interpolating across a stretch the series has no data for.
        val linePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
            style = Paint.Style.STROKE
            strokeWidth = 4f
            strokeCap = Paint.Cap.ROUND
            strokeJoin = Paint.Join.ROUND
        }
        for (s in series.indices) {
            linePaint.color = series[s].color
            linePath.reset()
            var started = false
            for (i in windowStart..windowEnd) {
                val v = normalizedAt(s, i, basePrices[s])
                if (v == null) {
                    started = false
                    continue
                }
                val x = xFor(i - windowStart)
                val y = yFor(v)
                if (!started) {
                    linePath.moveTo(x, y)
                    started = true
                } else {
                    linePath.lineTo(x, y)
                }
            }
            canvas.drawPath(linePath, linePaint)
        }

        if (scrubbedIndex in windowStart..windowEnd) {
            val x = xFor(scrubbedIndex - windowStart)
            canvas.drawLine(x, chartTop, x, chartBottom, crosshairPaint)
        }
    }

    private fun formatAxisDate(stored: String): String = try {
        dateAxisFormat.format(dateStoredFormat.parse(stored) ?: stored)
    } catch (e: Exception) {
        stored
    }

    private fun fireScrubCallback() {
        if (scrubbedIndex !in unionDates.indices) return
        val basePrices = series.indices.map { basePriceFor(it) }
        val values = series.mapIndexed { s, sr -> sr to normalizedAt(s, scrubbedIndex, basePrices[s]) }
        onScrubbed?.invoke(unionDates[scrubbedIndex], values)
    }

    private fun scrubAt(rawX: Float) {
        if (windowEnd - windowStart < 1) return
        val chartWidth = (width - chartPaddingLeft - chartPaddingRight).coerceAtLeast(1f)
        val fraction = ((rawX - chartPaddingLeft) / chartWidth).coerceIn(0f, 1f)
        val windowSize = windowEnd - windowStart
        val localIndex = (fraction * windowSize).toInt().coerceIn(0, windowSize)
        val index = windowStart + localIndex
        if (index != scrubbedIndex) {
            scrubbedIndex = index
            invalidate()
            fireScrubCallback()
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (unionDates.isEmpty()) return false
        scaleDetector.onTouchEvent(event)
        gestureDetector.onTouchEvent(event)

        if (scaleDetector.isInProgress) {
            isPanning = false
            return true
        }

        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                panLastX = event.x
                isPanning = false
                if (!isZoomed()) {
                    scrubAt(event.x)
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
                    scrubAt(event.x)
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
        if (newEnd > unionDates.size - 1) {
            val over = newEnd - (unionDates.size - 1)
            newStart -= over
            newEnd = unionDates.size - 1
        }
        newStart = newStart.coerceAtLeast(0)
        windowStart = newStart
        windowEnd = newEnd
        invalidate()
        if (!lockBaseDate) fireScrubCallback() // auto-rebase: the base (window start) just moved
    }

    private inner class ScaleListener : ScaleGestureDetector.SimpleOnScaleGestureListener() {
        override fun onScale(detector: ScaleGestureDetector): Boolean {
            if (unionDates.size <= MIN_WINDOW_POINTS) return true
            val windowSize = windowEnd - windowStart + 1
            val newWindowSize = (windowSize / detector.scaleFactor)
                .toInt()
                .coerceIn(MIN_WINDOW_POINTS, unionDates.size)
            if (newWindowSize == windowSize) return true

            val chartWidth = (width - chartPaddingLeft - chartPaddingRight).coerceAtLeast(1f)
            val focalFraction = ((detector.focusX - chartPaddingLeft) / chartWidth).coerceIn(0f, 1f)
            val focalIndex = windowStart + focalFraction * windowSize

            var newStart = (focalIndex - focalFraction * newWindowSize).toInt()
            var newEnd = newStart + newWindowSize - 1
            if (newStart < 0) {
                newEnd -= newStart
                newStart = 0
            }
            if (newEnd > unionDates.size - 1) {
                val over = newEnd - (unionDates.size - 1)
                newStart -= over
                newEnd = unionDates.size - 1
            }
            newStart = newStart.coerceAtLeast(0)
            windowStart = newStart
            windowEnd = newEnd
            invalidate()
            if (!lockBaseDate) fireScrubCallback() // auto-rebase: the base (window start) just moved
            return true
        }
    }

    private inner class GestureListener : GestureDetector.SimpleOnGestureListener() {
        override fun onDoubleTap(e: MotionEvent): Boolean {
            windowStart = 0
            windowEnd = (unionDates.size - 1).coerceAtLeast(0)
            invalidate()
            if (!lockBaseDate) fireScrubCallback()
            return true
        }
    }
}
