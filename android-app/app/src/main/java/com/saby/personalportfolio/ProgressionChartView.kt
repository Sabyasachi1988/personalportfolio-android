package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import androidx.core.content.ContextCompat
import java.text.SimpleDateFormat
import java.util.Locale

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
 */
class ProgressionChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    /** Called whenever the scrubbed index changes, including on initial layout (defaults to the last point). */
    var onScrub: ((index: Int) -> Unit)? = null

    private var points: List<ProgressionPoint> = emptyList()
    private var scrubbedIndex: Int = -1

    private val density = context.resources.displayMetrics.density
    private val axisDateFormat = SimpleDateFormat("MMM ''yy", Locale.US)
    private val isoDateFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)

    private val investedPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 3f * density
        strokeCap = Paint.Cap.ROUND
        strokeJoin = Paint.Join.ROUND
        color = ContextCompat.getColor(context, R.color.colorNeutral)
    }
    private val valuePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 5f * density
        strokeCap = Paint.Cap.ROUND
        strokeJoin = Paint.Join.ROUND
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

    init {
        isClickable = true
    }

    fun setPoints(newPoints: List<ProgressionPoint>) {
        points = newPoints
        scrubbedIndex = (points.size - 1).coerceAtLeast(-1)
        valuePaint.color = currentSeriesColor()
        invalidate()
        if (scrubbedIndex >= 0) onScrub?.invoke(scrubbedIndex)
    }

    /** Programmatically move the scrubber, e.g. from an external slider/seek control. Clamped to valid range. */
    fun scrubTo(index: Int) {
        if (points.isEmpty()) return
        val clamped = index.coerceIn(0, points.size - 1)
        if (clamped == scrubbedIndex) return
        scrubbedIndex = clamped
        invalidate()
        onScrub?.invoke(scrubbedIndex)
    }

    private fun currentSeriesColor(): Int {
        val last = points.lastOrNull() ?: return gainColor
        return if (last.gain >= 0) gainColor else lossColor
    }

    private fun xForIndex(index: Int): Float {
        if (points.size <= 1) return edgeInset
        val usableWidth = width - 2 * edgeInset
        return edgeInset + usableWidth * index / (points.size - 1).toFloat()
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
        if (points.size < 2) return

        var minV = Float.MAX_VALUE
        var maxV = -Float.MAX_VALUE
        for (p in points) {
            minV = minOf(minV, p.invested.toFloat(), p.value.toFloat())
            maxV = maxOf(maxV, p.invested.toFloat(), p.value.toFloat())
        }
        if (minV > 0f) minV = 0f // always anchor at zero so the eye reads absolute growth, not a zoomed-in wiggle
        val headroom = (maxV - minV) * 0.08f
        maxV += headroom

        // Light horizontal gridlines at 25/50/75% of the value range, purely
        // as a reading aid - no labels on these, the numbers already live
        // in the detail card above.
        for (fraction in listOf(0.25f, 0.5f, 0.75f)) {
            val y = yForValue(minV + (maxV - minV) * fraction, minV, maxV)
            canvas.drawLine(edgeInset, y, width - edgeInset, y, gridPaint)
        }

        val investedPath = Path()
        val valuePath = Path()
        for ((index, p) in points.withIndex()) {
            val x = xForIndex(index)
            val yInvested = yForValue(p.invested.toFloat(), minV, maxV)
            val yValue = yForValue(p.value.toFloat(), minV, maxV)
            if (index == 0) {
                investedPath.moveTo(x, yInvested)
                valuePath.moveTo(x, yValue)
            } else {
                investedPath.lineTo(x, yInvested)
                valuePath.lineTo(x, yValue)
            }
        }
        canvas.drawPath(investedPath, investedPaint)
        canvas.drawPath(valuePath, valuePaint)

        drawAxisLabels(canvas)

        if (scrubbedIndex in points.indices) {
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
     * bottom band. Picks about one label per ~56dp of width (fewer on a
     * narrow phone, more on a tablet) so labels never overlap each
     * other, always including the first and last point so the full span
     * is legible even between ticks.
     */
    private fun drawAxisLabels(canvas: Canvas) {
        if (points.size < 2) return
        val tickY = height - chartBottomInset + 6f * density
        val textY = height - 8f * density

        val approxLabelWidth = 56f * density
        val maxLabels = (width / approxLabelWidth).toInt().coerceIn(2, 6)
        val step = ((points.size - 1).toFloat() / (maxLabels - 1).coerceAtLeast(1)).coerceAtLeast(1f)

        val indices = LinkedHashSet<Int>()
        var i = 0f
        while (i <= points.size - 1) {
            indices.add(Math.round(i))
            i += step
        }
        indices.add(points.size - 1)

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
        if (points.size <= 1) return
        val usableWidth = (width - 2 * edgeInset).coerceAtLeast(1f)
        val fraction = ((x - edgeInset) / usableWidth).coerceIn(0f, 1f)
        val index = Math.round(fraction * (points.size - 1))
        scrubTo(index)
    }

    override fun performClick(): Boolean {
        super.performClick()
        return true
    }
}
