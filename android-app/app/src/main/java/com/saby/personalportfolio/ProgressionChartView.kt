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
 *
 * Once zoomed, a SINGLE finger drag pans the window (no need for a
 * second finger just to move around what's already zoomed in) - see
 * onTouchEvent's doc comment. A single-finger TAP still scrubs to that
 * point either way.
 */
class ProgressionChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    /** Called whenever the scrubbed index changes, including on initial layout (defaults to the last point). */
    var onScrub: ((index: Int) -> Unit)? = null

    /** Called whenever the zoom window changes, with true if currently zoomed in (not showing the full range). */
    var onZoomChanged: ((zoomed: Boolean) -> Unit)? = null

    /**
     * Called (once per gesture, not per-frame) when a pinch-out is
     * attempted while ALREADY showing the full extent of whatever
     * dataset is currently loaded (windowStart==0 AND
     * windowEnd==points.size-1) - there's nowhere further this chart's
     * OWN data can reveal, since it was only ever given a bounded daily
     * window (see ProgressionActivity's daily-zoom-on-demand fetching).
     * Continuing to pinch out at that point used to just silently do
     * nothing - clamped, but with no visible feedback that anything
     * happened, which read as the gesture being stuck/broken. The
     * hosting Activity is expected to swap in a wider dataset (its
     * weekly spine) in response, so zooming out keeps working smoothly
     * past the edge of the currently-loaded daily window.
     */
    var onZoomOutBeyondBounds: (() -> Unit)? = null

    /**
     * Called whenever the visible window's DATE RANGE changes (on
     * setPoints, resetZoom, and every pinch/pan update) - lets the
     * hosting Activity react to how narrow a span is actually showing,
     * e.g. to swap in daily-resolution data once zoomed past some
     * threshold (see ProgressionActivity's daily-overlay handling).
     * Fires with empty dates when there are no points at all.
     */
    var onWindowChanged: ((startDate: String, endDate: String, spanDays: Int) -> Unit)? = null

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

    private val investedColor = ContextCompat.getColor(context, R.color.colorProgressionInvested)

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
        notifyWindowChanged()
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
            notifyWindowChanged()
            onScrub?.invoke(scrubbedIndex)
        }
    }

    /**
     * Days between the window's start and end point dates (parsed as
     * plain "yyyy-MM-dd" - always well-formed here, since every date
     * comes from the bridge in that format). Falls back to the INDEX
     * span if parsing ever fails, which is still a reasonable proxy for
     * "how narrow is this view" even though it may not be exactly in
     * days for a weekly series.
     */
    private fun notifyWindowChanged() {
        val callback = onWindowChanged ?: return
        if (points.isEmpty()) {
            callback("", "", 0)
            return
        }
        val startDate = points[windowStart].date
        val endDate = points[windowEnd].date
        val spanDays = try {
            val start = isoDateFormat.parse(startDate)
            val end = isoDateFormat.parse(endDate)
            if (start != null && end != null) {
                ((end.time - start.time) / (1000L * 60 * 60 * 24)).toInt().coerceAtLeast(0)
            } else {
                windowEnd - windowStart
            }
        } catch (e: Exception) {
            windowEnd - windowStart
        }
        callback(startDate, endDate, spanDays)
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

    private val touchSlop = android.view.ViewConfiguration.get(context).scaledTouchSlop

    // Single-finger gesture tracking, for distinguishing a tap (scrub to
    // that point - the original behavior) from a drag (pan the visible
    // window, when zoomed - see onTouchEvent's doc comment). Reset at
    // the start of every single-finger gesture.
    private var downX = 0f
    private var downWindowStart = 0
    private var downWindowEnd = 0
    private var isPanningGesture = false

    /**
     * Single-finger behavior depends on zoom state:
     *
     * - NOT zoomed (showing the full loaded range): unchanged from
     *   before - continuous scrub-as-you-drag, exactly as it always
     *   worked. There's nothing meaningful to pan to at full zoom-out
     *   anyway.
     * - ZOOMED: a genuine drag (past touch-slop) now PANS the visible
     *   window with one finger, instead of requiring a two-finger drag
     *   (which necessarily also risks zooming if the two fingers don't
     *   move in perfect lockstep - reported directly as an issue). A
     *   quick TAP (movement stays under touch-slop) still scrubs to
     *   that point, resolved on release so a tap-that-becomes-a-drag
     *   doesn't flash a scrub position it's about to abandon.
     *
     * Two-finger pinch/pan (ScaleGestureDetector) is unaffected either
     * way - this only changes what a SINGLE finger does.
     */
    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (points.isEmpty()) return super.onTouchEvent(event)

        scaleDetector.onTouchEvent(event)
        gestureDetector.onTouchEvent(event)

        // While a pinch is actively in progress (2+ fingers down), don't
        // also treat the gesture as a single-finger scrub/pan - the two
        // interpretations of the same touch stream would otherwise fight
        // each other every frame.
        if (scaleDetector.isInProgress || event.pointerCount > 1) {
            isPanningGesture = false
            return true
        }

        when (event.action) {
            MotionEvent.ACTION_DOWN -> {
                downX = event.x
                downWindowStart = windowStart
                downWindowEnd = windowEnd
                isPanningGesture = false
                if (!isZoomed()) {
                    scrubToX(event.x)
                }
                return true
            }
            MotionEvent.ACTION_MOVE -> {
                if (isZoomed()) {
                    val totalDeltaX = event.x - downX
                    if (isPanningGesture || kotlin.math.abs(totalDeltaX) > touchSlop) {
                        isPanningGesture = true
                        panSingleFingerTo(totalDeltaX)
                    }
                    // else: still within touch-slop - could still become
                    // either a tap or a drag, so nothing happens yet
                    // (deliberately not scrubbing mid-uncertainty here,
                    // unlike the not-zoomed case below).
                } else {
                    scrubToX(event.x)
                }
                return true
            }
            MotionEvent.ACTION_UP -> {
                if (!isPanningGesture) {
                    scrubToX(event.x)
                }
                isPanningGesture = false
                performClick()
                return true
            }
        }
        return super.onTouchEvent(event)
    }

    /**
     * Pans the window by `totalDeltaX` pixels of single-finger movement
     * SINCE ACTION_DOWN (an absolute offset from the gesture's starting
     * window, not an incremental per-frame delta like the two-finger
     * pan uses) - simpler and avoids any incremental-accumulation drift
     * over a long drag. Same direction convention as the two-finger
     * pan: dragging right reveals earlier/left content.
     */
    private fun panSingleFingerTo(totalDeltaX: Float) {
        val span = (downWindowEnd - downWindowStart).coerceAtLeast(1)
        val usableWidth = (width - 2 * edgeInset).coerceAtLeast(1f)
        val indexPerPixel = span / usableWidth
        val indexDelta = -totalDeltaX * indexPerPixel

        var newStart = downWindowStart + indexDelta
        var newEnd = downWindowEnd + indexDelta
        val maxSpan = (points.size - 1).toFloat()
        if (newStart < 0f) {
            newEnd -= newStart
            newStart = 0f
        }
        if (newEnd > maxSpan) {
            newStart -= (newEnd - maxSpan)
            newEnd = maxSpan
        }
        windowStart = newStart.roundToInt().coerceIn(0, maxSpan.toInt())
        windowEnd = newEnd.roundToInt().coerceIn(windowStart, maxSpan.toInt())
        scrubbedIndex = scrubbedIndex.coerceIn(windowStart, windowEnd)
        invalidate()
        notifyWindowChanged()
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
        private var lastFocusXPixels = 0f
        private var panWindowStartF = 0f
        private var panWindowEndF = 0f
        // Captured once at the start of each gesture (not re-checked
        // per-frame) - see onZoomOutBeyondBounds' doc comment for why
        // "started already at full bounds" is the right thing to test,
        // rather than re-deriving "at bounds" from the live, possibly
        // momentarily-out-of-range pan/zoom math mid-frame.
        private var startedAtFullBounds = false
        private var firedZoomOutBeyondBounds = false

        override fun onScaleBegin(detector: ScaleGestureDetector): Boolean {
            panWindowStartF = windowStart.toFloat()
            panWindowEndF = windowEnd.toFloat()
            lastFocusXPixels = detector.focusX
            val maxSpanInt = points.size - 1
            startedAtFullBounds = points.size > 1 && windowStart <= 0 && windowEnd >= maxSpanInt
            firedZoomOutBeyondBounds = false
            return true
        }

        override fun onScale(detector: ScaleGestureDetector): Boolean {
            if (points.size < 2) return true

            // Already showing everything this chart has loaded, and the
            // person is still pinching outward - hand off to the
            // Activity to load something wider, rather than silently
            // clamping and doing nothing (which is what "stuck" looked
            // like). Fires once per gesture; once the Activity swaps
            // `points` out from under this view (via setPoints), the
            // rest of this gesture's frames become moot anyway.
            if (startedAtFullBounds && !firedZoomOutBeyondBounds && detector.scaleFactor < 1f) {
                firedZoomOutBeyondBounds = true
                onZoomOutBeyondBounds?.invoke()
                // The callback may have swapped `points` out from under
                // this view (e.g. falling back to the weekly spine via
                // setPoints) - re-sync the pan/zoom reference frame to
                // whatever the CURRENT state now is, so if the fingers
                // stay down and this same physical gesture continues,
                // it continues smoothly against the new dataset instead
                // of computing against now-meaningless offsets sized for
                // the old one.
                panWindowStartF = windowStart.toFloat()
                panWindowEndF = windowEnd.toFloat()
                lastFocusXPixels = detector.focusX
                startedAtFullBounds = points.size > 1 && windowStart <= 0 && windowEnd >= points.size - 1
                return true
            }

            val wasZoomed = isZoomed()
            val maxSpan = (points.size - 1).toFloat()
            val usableWidth = (width - 2 * edgeInset).coerceAtLeast(1f)

            // Pan: translate the window by however far the two-finger
            // focus point has moved since the last frame, in
            // index-space - this is what lets a drag reposition an
            // already-zoomed window (e.g. zoomed into Jan-Mar, then drag
            // to see Apr-Jun at the same zoom level) rather than only
            // ever being able to zoom back out to the full range.
            val deltaXPixels = detector.focusX - lastFocusXPixels
            lastFocusXPixels = detector.focusX
            val currentSpanF = (panWindowEndF - panWindowStartF).coerceAtLeast(1f)
            val indexPerPixel = currentSpanF / usableWidth
            val panDelta = -deltaXPixels * indexPerPixel
            panWindowStartF += panDelta
            panWindowEndF += panDelta

            // Zoom: rescale the span around the current focus point (a
            // pinch with scaleFactor == 1.0, i.e. a pure two-finger drag
            // with no pinching, leaves the span unchanged here - only
            // the pan translation above applies).
            val focusFraction = ((detector.focusX - edgeInset) / usableWidth).coerceIn(0f, 1f)
            val focusIndexF = panWindowStartF + focusFraction * (panWindowEndF - panWindowStartF)
            val newSpanF = (currentSpanF / detector.scaleFactor)
                .coerceIn(minWindowSpan.toFloat().coerceAtMost(maxSpan), maxSpan)
            panWindowStartF = focusIndexF - focusFraction * newSpanF
            panWindowEndF = panWindowStartF + newSpanF

            // Clamp to valid bounds, shifting the whole window rather
            // than squashing its width when it hits an edge.
            if (panWindowStartF < 0f) {
                panWindowEndF -= panWindowStartF
                panWindowStartF = 0f
            }
            if (panWindowEndF > maxSpan) {
                panWindowStartF -= (panWindowEndF - maxSpan)
                panWindowEndF = maxSpan
            }
            panWindowStartF = panWindowStartF.coerceAtLeast(0f)
            panWindowEndF = panWindowEndF.coerceAtMost(maxSpan)

            windowStart = panWindowStartF.roundToInt().coerceIn(0, maxSpan.toInt())
            windowEnd = panWindowEndF.roundToInt().coerceIn(windowStart, maxSpan.toInt())
            scrubbedIndex = scrubbedIndex.coerceIn(windowStart, windowEnd)
            invalidate()

            val nowZoomed = isZoomed()
            if (nowZoomed != wasZoomed) onZoomChanged?.invoke(nowZoomed)
            notifyWindowChanged()
            return true
        }
    }
}
