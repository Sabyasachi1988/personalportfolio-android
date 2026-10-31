package com.saby.personalportfolio

import android.animation.ValueAnimator
import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Path
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import android.view.animation.DecelerateInterpolator
import androidx.core.content.ContextCompat

/**
 * 100%-stacked area chart over months: each series is a share (percent) of the whole, stacked bottom-up.
 * Touch and drag to scrub a month; [onScrub] reports the month index (or -1 on release is not used: the last
 * scrubbed month stays selected). Colors are supplied per series; the view never invents data.
 */
class StackedAreaChartView @JvmOverloads constructor(
    context: Context, attrs: AttributeSet? = null
) : View(context, attrs) {

    class Series(val name: String, val color: Int, val values: DoubleArray)

    private var labels: List<String> = emptyList()
    private var series: List<Series> = emptyList()
    private var selected = -1
    var onScrub: ((Int) -> Unit)? = null

    private val textColor = ContextCompat.getColor(context, R.color.colorNeutral)
    private val fill = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
    private val edge = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE; strokeWidth = 1.5f; color = 0x66FFFFFF }
    private val grid = Paint().apply { color = 0x33808080; strokeWidth = 1f }
    private val label = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = textColor; textSize = 11f * resources.displayMetrics.scaledDensity }
    private val cursor = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = 0xFF000000.toInt(); strokeWidth = 2f }
    private var reveal = 1f
    private var animator: ValueAnimator? = null

    fun setData(labels: List<String>, series: List<Series>) {
        this.labels = labels
        this.series = series
        selected = if (labels.isEmpty()) -1 else labels.size - 1
        animator?.cancel()
        animator = ValueAnimator.ofFloat(0f, 1f).apply {
            duration = 650
            interpolator = DecelerateInterpolator()
            addUpdateListener { reveal = it.animatedValue as Float; invalidate() }
            start()
        }
        invalidate()
    }

    fun selectedIndex() = selected

    private val padL get() = 40f * resources.displayMetrics.density
    private val padR get() = 8f * resources.displayMetrics.density
    private val padT get() = 8f * resources.displayMetrics.density
    private val padB get() = 22f * resources.displayMetrics.density

    private fun xAt(i: Int, n: Int): Float {
        val w = width - padL - padR
        return if (n <= 1) padL + w / 2 else padL + w * i / (n - 1)
    }

    override fun onDraw(c: Canvas) {
        super.onDraw(c)
        val n = labels.size
        val top = padT
        val bottom = height - padB
        val h = bottom - top
        // gridlines + y labels
        for (p in intArrayOf(0, 25, 50, 75, 100)) {
            val y = bottom - h * p / 100f
            c.drawLine(padL, y, width - padR, y, grid)
            c.drawText("$p%", 4f, y + label.textSize / 3, label)
        }
        if (n == 0 || series.isEmpty()) return
        c.save()
        c.clipRect(0f, 0f, padL + (width - padL) * reveal, height.toFloat())
        val base = DoubleArray(n)
        for (s in series) {
            val path = Path()
            for (i in 0 until n) {
                val y = bottom - h * ((base[i] + s.values[i]) / 100.0).toFloat().coerceIn(0f, 1f)
                if (i == 0) path.moveTo(xAt(i, n), y) else path.lineTo(xAt(i, n), y)
            }
            for (i in n - 1 downTo 0) {
                path.lineTo(xAt(i, n), bottom - h * (base[i] / 100.0).toFloat().coerceIn(0f, 1f))
            }
            path.close()
            fill.color = s.color
            c.drawPath(path, fill)
            c.drawPath(path, edge)
            for (i in 0 until n) base[i] += s.values[i]
        }
        c.restore()
        // x labels: first, last and up to 3 between, skipping any that would crowd
        val every = maxOf(1, (n - 1) / 4)
        var i = 0
        while (i < n) {
            val t = labels[i]
            val tw = label.measureText(t)
            val x = (xAt(i, n) - tw / 2).coerceIn(0f, width - tw)
            c.drawText(t, x, height - 4f, label)
            i += every
        }
        if (selected in 0 until n) {
            val x = xAt(selected, n)
            c.drawLine(x, top, x, bottom, cursor)
        }
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        val n = labels.size
        if (n == 0) return false
        parent?.requestDisallowInterceptTouchEvent(true)
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE -> {
                val w = width - padL - padR
                val frac = ((e.x - padL) / w).coerceIn(0f, 1f)
                val idx = if (n == 1) 0 else Math.round(frac * (n - 1))
                if (idx != selected) {
                    selected = idx
                    onScrub?.invoke(idx)
                    invalidate()
                }
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> parent?.requestDisallowInterceptTouchEvent(false)
        }
        return true
    }
}
