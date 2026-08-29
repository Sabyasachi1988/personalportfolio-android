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
        private const val MARKER_RADIUS_PX = 15f
        private const val MARKER_TAP_RADIUS_PX = 42f // touch tolerance is deliberately larger than the drawn dot - a small dot is a small target to hit precisely with a finger
    }

    var onPointScrubbed: ((windowStartPoint: PricePoint, currentPoint: PricePoint) -> Unit)? = null

    /** Invoked whenever the visible window (zoom/pan/reset/set-range) changes - lets a hosting Activity keep an independent ChartRangeScrubberView in sync. Fires with (totalPointCount, windowStart, windowEnd). */
    var onWindowChanged: ((total: Int, start: Int, end: Int) -> Unit)? = null

    /** Invoked when a person taps directly on a transaction marker dot - see setMarkers/MARKER_TAP_RADIUS_PX. */
    var onMarkerTapped: ((TransactionMarker) -> Unit)? = null

    private var points: List<PricePoint> = emptyList()
    private var windowStart = 0
    private var windowEnd = 0 // inclusive
    private var scrubbedIndex = -1

    // Each marker paired with the index (into `points`) of its closest
    // matching date - resolved once in setMarkers, not on every frame,
    // since points/markers only change together (a fresh setPoints call
    // always precedes setMarkers for the same fund). Drawn/hit-tested at
    // that point's actual (x, y) on the line - not at a position derived
    // from the marker's OWN statement price - so a marker dot always
    // sits exactly on the line even on the rare day the statement price
    // and the later-fetched NAV differ by a paisa or two (see
    // TransactionMarker's Go doc comment on why those can differ).
    private var resolvedMarkers: List<Pair<Int, TransactionMarker>> = emptyList()

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
    // Buy/sell transaction markers. Buy is deliberately colorAmber, NOT
    // colorGain - a confirmed real bug: in this app's dark theme,
    // colorGain (#66BB6A) and colorPrimary (#66BB6A, the LINE's own
    // color) are the exact same hex value, so a buy dot was
    // indistinguishable from the line it sits on. colorAmber has no hue
    // overlap with either the green line or the red sell marker in
    // EITHER theme, so buy/sell/line stay 3 genuinely distinct colors
    // regardless of light/dark mode. Sell keeps colorLoss - already far
    // enough from the green line to read clearly on its own.
    private val buyMarkerPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
        color = ContextCompat.getColor(context, R.color.colorAmber)
    }
    private val sellMarkerPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
        color = ContextCompat.getColor(context, R.color.colorLoss)
    }
    // A bright outline ring around each marker, drawn in colorOnSurface
    // (near-white in dark theme, near-black in light theme - always the
    // theme's own high-contrast "text on surface" color) rather than
    // colorSurface (which in dark theme is #1C1C1C - a near-black ring
    // against a near-black chart background would itself be nearly
    // invisible, the same class of bug as the buy-color one above).
    private val markerRingPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 4f
        color = ContextCompat.getColor(context, R.color.colorOnSurface)
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
    // chartPaddingLeftMin is a FLOOR, not the actual left padding used -
    // see onDraw's own computation of the real left padding from the
    // actual y-axis label widths. A fixed 96f here was a confirmed real
    // bug: a NAV/index level with 4+ digits (e.g. "2,297.09") measures
    // wider than the old fixed budget at this text size, so the label
    // (drawn right-aligned, growing LEFTWARD from chartLeft-12f) got
    // clipped against the view's own left edge instead of fully
    // displaying. The floor just keeps small numbers from pulling the
    // axis in uncomfortably tight.
    private val chartPaddingLeftMin = 96f
    private val chartPaddingBottom = 44f

    private val dateStoredFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
    private val dateAxisFormat = SimpleDateFormat("d MMM ''yy", Locale.getDefault())

    private val scaleDetector = ScaleGestureDetector(context, ScaleListener())
    private val gestureDetector = GestureDetector(context, GestureListener())
    private val touchSlop = ViewConfiguration.get(context).scaledTouchSlop

    private var panLastX = 0f
    private var isPanning = false

    // The actual left padding used by the most recent onDraw - touch
    // handling (scrubAt/panBy) must use this SAME value, not the fixed
    // floor, or scrub/pan math would drift out of sync with where the
    // chart was actually drawn whenever a large NAV widened the axis
    // beyond chartPaddingLeftMin. Starts at the floor before any draw.
    private var lastDrawnChartLeft = chartPaddingLeftMin

    // Full drawn-chart geometry from the most recent onDraw, cached so
    // marker hit-testing (findMarkerNear, called from a tap gesture
    // OUTSIDE onDraw) can compute the exact same on-screen (x, y) for a
    // given point index that onDraw itself just used - without
    // duplicating a second copy of onDraw's xFor/yFor closures that
    // could silently drift out of sync with the actual drawing math.
    private var lastDrawnChartRight = 0f
    private var lastDrawnChartTop = 0f
    private var lastDrawnChartBottom = 0f
    private var lastDrawnMinPrice = 0.0
    private var lastDrawnPriceRange = 1.0
    private var lastDrawnWindowStart = 0
    private var lastDrawnWindowEnd = 0

    fun setPoints(newPoints: List<PricePoint>) {
        points = newPoints.sortedBy { it.date }
        windowStart = 0
        windowEnd = (points.size - 1).coerceAtLeast(0)
        scrubbedIndex = if (points.isNotEmpty()) points.size - 1 else -1
        resolvedMarkers = emptyList() // markers are resolved against THIS series' points - stale from a previous fund otherwise
        invalidate()
        fireScrubCallback()
        fireWindowChangedCallback()
    }

    /**
     * Overlays buy/sell transaction dots on the chart - see
     * TransactionMarker's Go doc comment for what's excluded (cash-only
     * events with no unit change). Each marker is resolved to the
     * closest date already present in the currently loaded price
     * series (setPoints must be called first) so it draws exactly on
     * the line rather than at a position independently derived from
     * the marker's own statement price. A marker whose date has no
     * NAV history at all yet (a genuine data gap) is silently dropped
     * rather than drawn somewhere misleading.
     */
    fun setMarkers(newMarkers: List<TransactionMarker>) {
        if (points.isEmpty()) {
            resolvedMarkers = emptyList()
            return
        }
        resolvedMarkers = newMarkers.mapNotNull { marker ->
            closestPointIndexForDate(marker.date)?.let { idx -> idx to marker }
        }
        invalidate()
    }

    // points is sorted ascending by "yyyy-MM-dd" date, which sorts
    // correctly as a plain string - binary search finds the first
    // index whose date is >= the marker's date (i.e. the closest
    // available NAV on or after the transaction date, the normal case
    // being an exact match since both come from the same fund).
    private fun closestPointIndexForDate(date: String): Int? {
        if (points.isEmpty()) return null
        var lo = 0
        var hi = points.size - 1
        while (lo < hi) {
            val mid = (lo + hi) / 2
            if (points[mid].date < date) lo = mid + 1 else hi = mid
        }
        return lo
    }

    /**
     * Sets the visible window to the closest available points to the
     * given dates (inclusive) - the manual counterpart to pinch-zoom,
     * for typing/picking an exact range instead of gesturing one. Dates
     * outside the series' actual range clamp to the nearest real point
     * rather than erroring - a person picking "1 Jan 2010" on a fund
     * that started trading in 2015 should land on its earliest point,
     * not get an empty chart. No-op if there are fewer than 2 points to
     * show between them.
     */
    fun setWindowByDates(startDate: String, endDate: String) {
        if (points.size < 2) return
        val lo = minOf(startDate, endDate)
        val hi = maxOf(startDate, endDate)
        var startIndex = points.indexOfFirst { it.date >= lo }
        if (startIndex < 0) startIndex = points.size - 1
        var endIndex = points.indexOfLast { it.date <= hi }
        if (endIndex < 0) endIndex = 0
        if (endIndex <= startIndex) endIndex = (startIndex + 1).coerceAtMost(points.size - 1)
        if (startIndex >= endIndex) startIndex = (endIndex - 1).coerceAtLeast(0)
        windowStart = startIndex
        windowEnd = endIndex
        scrubbedIndex = windowEnd
        invalidate()
        fireScrubCallback()
        fireWindowChangedCallback()
    }

    private fun fireScrubCallback() {
        if (scrubbedIndex !in points.indices || windowStart !in points.indices) return
        onPointScrubbed?.invoke(points[windowStart], points[scrubbedIndex])
    }

    private fun fireWindowChangedCallback() {
        onWindowChanged?.invoke(points.size, windowStart, windowEnd)
    }

    /**
     * Moves the visible window to an exact (startIndex, endIndex), same
     * window-size-preserving semantics as panBy - the ChartRangeScrubberView
     * counterpart to dragging directly on the chart, see that class's
     * doc comment for why a second, independent way to move the window
     * exists at all.
     */
    fun setWindowByIndex(startIndex: Int, endIndex: Int) {
        if (points.isEmpty()) return
        windowStart = startIndex.coerceIn(0, points.size - 1)
        windowEnd = endIndex.coerceIn(windowStart, points.size - 1)
        invalidate()
        fireScrubCallback()
        fireWindowChangedCallback()
    }

    /** True once the visible window is narrower than the full series - see class doc comment. */
    private fun isZoomed(): Boolean = points.isNotEmpty() && (windowEnd - windowStart + 1) < points.size

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        if (windowEnd - windowStart < 1) return
        val visible = points.subList(windowStart, windowEnd + 1)

        val w = width.toFloat()
        val h = height.toFloat()

        val minPrice = visible.minOf { it.price }
        val maxPrice = visible.maxOf { it.price }
        val midPrice = (minPrice + maxPrice) / 2.0
        val priceRange = (maxPrice - minPrice).let { if (it <= 0.0) 1.0 else it }

        // Widest of the 3 y-axis labels actually being drawn THIS frame
        // (not a fixed guess) decides how much left padding the chart
        // needs, so a large NAV/index level (4+ digits, e.g.
        // "2,297.09") gets the room it needs instead of being clipped -
        // see chartPaddingLeftMin's doc comment for the bug this fixes.
        val maxLabel = PricePerUnitFormatter.format(maxPrice, decimals = 2)
        val midLabel = PricePerUnitFormatter.format(midPrice, decimals = 2)
        val minLabel = PricePerUnitFormatter.format(minPrice, decimals = 2)
        val widestLabelWidth = maxOf(
            axisLabelPaint.measureText(maxLabel),
            axisLabelPaint.measureText(midLabel),
            axisLabelPaint.measureText(minLabel)
        )
        val chartLeft = (widestLabelWidth + 28f).coerceAtLeast(chartPaddingLeftMin)
        lastDrawnChartLeft = chartLeft
        val chartRight = w - chartPaddingRight
        val chartTop = chartPaddingTop
        val chartBottom = h - chartPaddingBottom
        val chartWidth = (chartRight - chartLeft).coerceAtLeast(1f)
        val chartHeight = (chartBottom - chartTop).coerceAtLeast(1f)

        // Cache the geometry this frame actually drew with, so
        // findMarkerNear (called later from a tap, outside onDraw) can
        // reproduce the exact same (x, y) for a given point index - see
        // the field's own doc comment above.
        lastDrawnChartRight = chartRight
        lastDrawnChartTop = chartTop
        lastDrawnChartBottom = chartBottom
        lastDrawnMinPrice = minPrice
        lastDrawnPriceRange = priceRange
        lastDrawnWindowStart = windowStart
        lastDrawnWindowEnd = windowEnd

        fun xFor(localIndex: Int): Float = chartLeft + chartWidth * localIndex / (visible.size - 1).toFloat().coerceAtLeast(1f)
        fun yFor(price: Double): Float = chartTop + chartHeight * (1f - ((price - minPrice) / priceRange).toFloat())

        // Y-axis gridlines + price labels: min / mid / max of the
        // CURRENTLY VISIBLE window, not the full series - so zooming
        // into a narrower price range re-scales the axis to that
        // range, which is what makes zoom actually useful for reading
        // fine-grained moves rather than just stretching the same line.
        axisLabelPaint.textAlign = Paint.Align.RIGHT
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

        // Buy/sell transaction markers - only those whose resolved
        // point index falls within the CURRENTLY VISIBLE window are
        // drawn, same "visible window" scoping as the crosshair/line
        // above, so zooming/panning naturally shows/hides markers along
        // with the rest of the chart.
        resolvedMarkers.forEach { (index, marker) ->
            val localIndex = index - windowStart
            if (localIndex !in visible.indices) return@forEach
            val x = xFor(localIndex)
            val y = yFor(visible[localIndex].price)
            val paint = if (marker.isBuy) buyMarkerPaint else sellMarkerPaint
            canvas.drawCircle(x, y, MARKER_RADIUS_PX, paint)
            canvas.drawCircle(x, y, MARKER_RADIUS_PX, markerRingPaint)
        }
    }

    private fun formatAxisDate(stored: String): String = try {
        dateAxisFormat.format(dateStoredFormat.parse(stored) ?: stored)
    } catch (e: Exception) {
        stored
    }

    private fun scrubAt(rawX: Float) {
        if (windowEnd - windowStart < 1) return
        val chartLeft = lastDrawnChartLeft
        val chartWidth = (width - chartLeft - chartPaddingRight).coerceAtLeast(1f)
        val fraction = ((rawX - chartLeft) / chartWidth).coerceIn(0f, 1f)
        val windowSize = windowEnd - windowStart
        val localIndex = (fraction * windowSize).toInt().coerceIn(0, windowSize)
        val index = windowStart + localIndex
        if (index != scrubbedIndex) {
            scrubbedIndex = index
            invalidate()
            fireScrubCallback()
            fireWindowChangedCallback()
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
        val chartWidth = (width - lastDrawnChartLeft - chartPaddingRight).coerceAtLeast(1f)
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
        fireScrubCallback()
        fireWindowChangedCallback()
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
            val chartWidth = (width - lastDrawnChartLeft - chartPaddingRight).coerceAtLeast(1f)
            val focalFraction = ((detector.focusX - lastDrawnChartLeft) / chartWidth).coerceIn(0f, 1f)
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
            fireScrubCallback()
            fireWindowChangedCallback()
            return true
        }
    }

    private inner class GestureListener : GestureDetector.SimpleOnGestureListener() {
        override fun onDoubleTap(e: MotionEvent): Boolean {
            windowStart = 0
            windowEnd = (points.size - 1).coerceAtLeast(0)
            invalidate()
            fireScrubCallback()
            fireWindowChangedCallback()
            return true
        }

        // A confirmed single tap (not the first tap of a double-tap
        // sequence) checks for a nearby marker dot - deliberately here
        // rather than in onTouchEvent's ACTION_UP handling, since that
        // already fires on every tap/drag-release for scrub purposes
        // and doesn't distinguish a genuine tap from a drag the way
        // GestureDetector's own tap-confirmation timing does.
        override fun onSingleTapConfirmed(e: MotionEvent): Boolean {
            val marker = findMarkerNear(e.x, e.y) ?: return false
            onMarkerTapped?.invoke(marker)
            return true
        }
    }

    /**
     * Finds the closest resolved marker within MARKER_TAP_RADIUS_PX of
     * a tap, using the SAME geometry the most recent onDraw actually
     * used (see the lastDrawn* fields' doc comment) - not a
     * re-derivation that could drift out of sync with where the dots
     * were actually drawn.
     */
    private fun findMarkerNear(touchX: Float, touchY: Float): TransactionMarker? {
        if (resolvedMarkers.isEmpty()) return null
        val windowSize = lastDrawnWindowEnd - lastDrawnWindowStart + 1
        if (windowSize <= 0) return null
        val chartWidth = (lastDrawnChartRight - lastDrawnChartLeft).coerceAtLeast(1f)
        val chartHeight = (lastDrawnChartBottom - lastDrawnChartTop).coerceAtLeast(1f)
        var closest: TransactionMarker? = null
        var closestDistSq = Float.MAX_VALUE
        val tapRadiusSq = MARKER_TAP_RADIUS_PX * MARKER_TAP_RADIUS_PX
        resolvedMarkers.forEach { (index, marker) ->
            val localIndex = index - lastDrawnWindowStart
            if (localIndex !in 0 until windowSize) return@forEach
            val point = points.getOrNull(index) ?: return@forEach
            val x = lastDrawnChartLeft + chartWidth * localIndex / (windowSize - 1).coerceAtLeast(1).toFloat()
            val y = lastDrawnChartTop + chartHeight * (1f - ((point.price - lastDrawnMinPrice) / lastDrawnPriceRange).toFloat())
            val dx = touchX - x
            val dy = touchY - y
            val distSq = dx * dx + dy * dy
            if (distSq <= tapRadiusSq && distSq < closestDistSq) {
                closestDistSq = distSq
                closest = marker
            }
        }
        return closest
    }
}
