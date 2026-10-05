package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import androidx.core.content.ContextCompat
import java.text.SimpleDateFormat
import java.util.Locale
import kotlin.math.abs

/**
 * Line chart of a rolling return over time: one point per window, placed by
 * the date the window ENDS on. Green above the 0% line, red below it, split
 * exactly where the line crosses zero. Touch or drag to read any window; the
 * host Activity shows the value. The 0% line is always in view.
 */
class RollingChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    /** Called with the index (into the arrays last passed to [setData]) of the window under the finger. */
    var onScrub: ((index: Int) -> Unit)? = null

    private var dates: List<String> = emptyList()
    private var days = IntArray(0)
    private var values = DoubleArray(0)
    private var scrubbed = -1

    private val density = context.resources.displayMetrics.density
    private val isoFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)
    private val monthFormat = SimpleDateFormat("MMM ''yy", Locale.US)
    private val dayFormat = SimpleDateFormat("d MMM", Locale.US)

    private val gainColor = ContextCompat.getColor(context, R.color.colorGain)
    private val lossColor = ContextCompat.getColor(context, R.color.colorLoss)
    private val onSurface = ContextCompat.getColor(context, R.color.colorOnSurface)
    private val neutral = ContextCompat.getColor(context, R.color.colorNeutral)

    private val linePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 3.5f * density
        strokeCap = Paint.Cap.ROUND
        strokeJoin = Paint.Join.ROUND
    }
    private val fillPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
    private val zeroPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1.2f * density
        color = onSurface
        alpha = 110
    }
    private val scrubLinePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1.5f * density
        color = onSurface
        alpha = 100
    }
    private val dotPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
    private val labelPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.FILL
        color = neutral
        textSize = 11f * density
    }
    private val tickPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        strokeWidth = 1.5f * density
        color = neutral
        alpha = 140
    }

    private val sideInset = 12f * density
    private val topInset = 18f * density
    private val axisBand = 28f * density
    private val touchSlop = ViewConfiguration.get(context).scaledTouchSlop
    private var downX = 0f
    private var downY = 0f

    init { isClickable = true }

    /** [dates] are "yyyy-MM-dd", ascending; [percents] the rolling return for the window ending on each. */
    fun setData(dates: List<String>, percents: DoubleArray) {
        this.dates = dates
        this.values = percents
        this.days = IntArray(dates.size) { WindowMath.dayNumber(dates[it]) ?: 0 }
        scrubbed = dates.size - 1
        invalidate()
        if (scrubbed >= 0) onScrub?.invoke(scrubbed)
    }

    private fun spanDays(): Int = if (days.size < 2) 0 else days.last() - days.first()

    private fun xFor(i: Int): Float {
        val w = width - 2 * sideInset
        val span = spanDays()
        return if (span <= 0) sideInset + w / 2f else sideInset + w * (days[i] - days.first()) / span.toFloat()
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        val n = values.size
        if (n == 0) return

        var minV = minOf(values.min(), 0.0)
        var maxV = maxOf(values.max(), 0.0)
        if (maxV - minV < 1e-9) { maxV += 1.0; minV -= 1.0 }
        val pad = (maxV - minV) * 0.10
        maxV += pad
        minV -= pad

        val baselineBottom = height - axisBand
        val usable = baselineBottom - topInset
        fun yFor(v: Double): Float = (baselineBottom - (v - minV) / (maxV - minV) * usable).toFloat()
        val zeroY = yFor(0.0)

        canvas.drawLine(sideInset, zeroY, width - sideInset, zeroY, zeroPaint)

        val gainPath = Path(); val lossPath = Path()
        val gainFill = Path(); val lossFill = Path()
        fun quad(p: Path, x0: Float, y0: Float, x1: Float, y1: Float) {
            p.moveTo(x0, zeroY); p.lineTo(x0, y0); p.lineTo(x1, y1); p.lineTo(x1, zeroY); p.close()
        }
        if (n >= 2) {
            for (i in 1 until n) {
                val x0 = xFor(i - 1); val y0 = yFor(values[i - 1])
                val x1 = xFor(i); val y1 = yFor(values[i])
                val a = values[i - 1]; val b = values[i]
                val aPos = a >= 0; val bPos = b >= 0
                if (aPos == bPos) {
                    val line = if (aPos) gainPath else lossPath
                    line.moveTo(x0, y0); line.lineTo(x1, y1)
                    quad(if (aPos) gainFill else lossFill, x0, y0, x1, y1)
                } else {
                    val t = (a / (a - b)).toFloat().coerceIn(0f, 1f)
                    val cx = x0 + t * (x1 - x0)
                    (if (aPos) gainPath else lossPath).apply { moveTo(x0, y0); lineTo(cx, zeroY) }
                    (if (bPos) gainPath else lossPath).apply { moveTo(cx, zeroY); lineTo(x1, y1) }
                    quad(if (aPos) gainFill else lossFill, x0, y0, cx, zeroY)
                    quad(if (bPos) gainFill else lossFill, cx, zeroY, x1, y1)
                }
            }
            fillPaint.color = (gainColor and 0x00FFFFFF) or 0x2A000000
            canvas.drawPath(gainFill, fillPaint)
            fillPaint.color = (lossColor and 0x00FFFFFF) or 0x2A000000
            canvas.drawPath(lossFill, fillPaint)
            linePaint.color = lossColor
            canvas.drawPath(lossPath, linePaint)
            linePaint.color = gainColor
            canvas.drawPath(gainPath, linePaint)
        } else {
            dotPaint.color = if (values[0] >= 0) gainColor else lossColor
            canvas.drawCircle(xFor(0), yFor(values[0]), 5f * density, dotPaint)
        }

        // Value labels: only the extremes, and the zero line.
        labelPaint.textAlign = Paint.Align.LEFT
        val top = values.max()
        val bottom = values.min()
        if (top > 0) canvas.drawText(fmt(top), sideInset + 4f * density, yFor(top) - 5f * density, labelPaint)
        if (bottom < 0) canvas.drawText(fmt(bottom), sideInset + 4f * density, yFor(bottom) + 14f * density, labelPaint)
        labelPaint.textAlign = Paint.Align.RIGHT
        canvas.drawText("0%", width - sideInset - 2f * density, zeroY - 4f * density, labelPaint)
        labelPaint.textAlign = Paint.Align.LEFT

        drawTimeAxis(canvas, baselineBottom)

        if (scrubbed in 0 until n) {
            val x = xFor(scrubbed)
            canvas.drawLine(x, topInset, x, baselineBottom, scrubLinePaint)
            dotPaint.color = if (values[scrubbed] >= 0) gainColor else lossColor
            canvas.drawCircle(x, yFor(values[scrubbed]), 5.5f * density, dotPaint)
        }
    }

    private fun fmt(v: Double): String = String.format(Locale.US, "%+.1f%%", v)

    /** Up to five evenly spaced date labels; day+month on narrow spans (they can't repeat), month+year on wide ones. */
    private fun drawTimeAxis(canvas: Canvas, baselineBottom: Float) {
        val n = dates.size
        if (n == 0) return
        val withDay = spanDays() <= 200
        val tickY = baselineBottom + 6f * density
        val textY = height - 8f * density
        val labelCount = if (n == 1) 1 else 5
        val chosen = LinkedHashSet<Int>()
        for (k in 0 until labelCount) {
            val target = if (labelCount == 1) days[0] else days.first() + (spanDays() * k) / (labelCount - 1)
            chosen.add(nearestIndexToDay(target))
        }
        for (i in chosen) {
            val x = xFor(i)
            canvas.drawLine(x, baselineBottom, x, tickY, tickPaint)
            val parsed = try { isoFormat.parse(dates[i]) } catch (e: Exception) { null }
            val label = if (parsed == null) dates[i] else (if (withDay) dayFormat else monthFormat).format(parsed)
            val w = labelPaint.measureText(label)
            canvas.drawText(label, (x - w / 2f).coerceIn(0f, width - w), textY, labelPaint)
        }
    }

    private fun nearestIndexToDay(day: Int): Int {
        var best = 0
        var bestDiff = Int.MAX_VALUE
        for (i in days.indices) {
            val d = abs(days[i] - day)
            if (d < bestDiff) { bestDiff = d; best = i }
        }
        return best
    }

    private fun scrubToX(x: Float) {
        if (days.isEmpty()) return
        val w = (width - 2 * sideInset).coerceAtLeast(1f)
        val frac = ((x - sideInset) / w).coerceIn(0f, 1f)
        val target = days.first() + (spanDays() * frac).toInt()
        val i = nearestIndexToDay(target)
        if (i != scrubbed) {
            scrubbed = i
            invalidate()
            onScrub?.invoke(i)
        }
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (values.isEmpty()) return super.onTouchEvent(event)
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                downX = event.x; downY = event.y
                scrubToX(event.x)
                return true
            }
            MotionEvent.ACTION_MOVE -> {
                // Keep the page scrollable until the finger is clearly moving sideways.
                if (abs(event.x - downX) > touchSlop && abs(event.x - downX) > abs(event.y - downY)) {
                    parent?.requestDisallowInterceptTouchEvent(true)
                }
                scrubToX(event.x)
                return true
            }
            MotionEvent.ACTION_UP -> {
                performClick()
                return true
            }
            MotionEvent.ACTION_CANCEL -> {
                parent?.requestDisallowInterceptTouchEvent(false)
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
