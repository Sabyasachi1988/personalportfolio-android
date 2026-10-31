package com.saby.personalportfolio

import android.animation.ValueAnimator
import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapShader
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import android.graphics.Shader
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import android.view.animation.DecelerateInterpolator
import androidx.core.content.ContextCompat
import com.google.android.material.color.MaterialColors

/**
 * 100%-stacked area chart over months. Each series is a share (percent) of the whole, stacked bottom-up, separated by a
 * 2dp surface-coloured gap (no strokes around marks). Touch/drag scrubs a month: a hairline cursor plus a floating
 * tooltip card list that month's values. Series flagged [Series.hatched] draw as diagonal hatching ("no data").
 */
class StackedAreaChartView @JvmOverloads constructor(
    context: Context, attrs: AttributeSet? = null
) : View(context, attrs) {

    class Series(val name: String, val color: Int, val values: DoubleArray, val hatched: Boolean = false)

    private var labels: List<String> = emptyList()
    private var series: List<Series> = emptyList()
    private var selected = -1
    private var touching = false
    var onScrub: ((Int) -> Unit)? = null

    private val d = resources.displayMetrics.density
    private val sp = resources.displayMetrics.scaledDensity
    private val surface = MaterialColors.getColor(this, com.google.android.material.R.attr.colorSurface, Color.WHITE)
    private val onSurface = MaterialColors.getColor(this, com.google.android.material.R.attr.colorOnSurface, Color.BLACK)
    private val muted = ContextCompat.getColor(context, R.color.viz_text_muted)
    private val gridColor = ContextCompat.getColor(context, R.color.viz_grid)

    private val fill = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.FILL }
    private val gap = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE; strokeWidth = 2f * d; color = surface; strokeJoin = Paint.Join.ROUND }
    private val grid = Paint().apply { color = gridColor; strokeWidth = 1f }
    private val axisText = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = muted; textSize = 11f * sp }
    private val cursor = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; strokeWidth = 1.5f * d; alpha = 200 }
    private val dot = Paint(Paint.ANTI_ALIAS_FLAG)
    private val bubbleFill = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = surface }
    private val bubbleLine = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE; color = gridColor; strokeWidth = 1f * d }
    private val bubbleTitle = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; textSize = 13f * sp; isFakeBoldText = true }
    private val bubbleText = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; textSize = 12f * sp }
    private val bubbleVal = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; textSize = 12f * sp; textAlign = Paint.Align.RIGHT; isFakeBoldText = true }
    private val hatchPaint: Paint by lazy { makeHatch() }

    private var reveal = 1f
    private var animator: ValueAnimator? = null


    private fun makeHatch(): Paint {
        val s = (8 * d).toInt().coerceAtLeast(6)
        val bmp = Bitmap.createBitmap(s, s, Bitmap.Config.ARGB_8888)
        val c = Canvas(bmp)
        val p = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = ContextCompat.getColor(context, R.color.viz_neutral_soft); strokeWidth = 1.5f * d }
        c.drawLine(0f, s.toFloat(), s.toFloat(), 0f, p)
        c.drawLine(-s / 2f, s / 2f, s / 2f, -s / 2f, p)
        c.drawLine(s / 2f, s * 1.5f, s * 1.5f, s / 2f, p)
        return Paint(Paint.ANTI_ALIAS_FLAG).apply { shader = BitmapShader(bmp, Shader.TileMode.REPEAT, Shader.TileMode.REPEAT) }
    }

    private var yMax = 100.0

    /** [yMax] is the axis maximum in the same unit as the values (100 for a 100%-stacked chart). */
    fun setData(labels: List<String>, series: List<Series>, animate: Boolean = true, yMax: Double = 100.0) {
        this.yMax = yMax.coerceAtLeast(1.0)
        this.labels = labels
        this.series = series
        selected = if (labels.isEmpty()) -1 else labels.size - 1
        animator?.cancel()
        if (animate) {
            animator = ValueAnimator.ofFloat(0f, 1f).apply {
                duration = 700
                interpolator = DecelerateInterpolator(1.6f)
                addUpdateListener { reveal = it.animatedValue as Float; invalidate() }
                start()
            }
        } else reveal = 1f
        invalidate()
    }

    private val padL get() = 34f * d
    private val padR get() = 6f * d
    private val padT get() = 6f * d
    private val padB get() = 22f * d

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
        for (q in 0..4) {
            val y = bottom - h * q / 4f
            val v = yMax * q / 4
            c.drawLine(padL, y, width - padR, y, grid)
            c.drawText(if (v == Math.floor(v)) "${v.toInt()}%" else "%.1f%%".format(v), 0f, y + axisText.textSize / 3, axisText)
        }
        if (n == 0 || series.isEmpty()) return
        c.save()
        c.clipRect(0f, 0f, padL + (width - padL) * reveal, height.toFloat())
        val base = DoubleArray(n)
        for (s in series) {
            val path = Path()
            for (i in 0 until n) {
                val y = bottom - h * ((base[i] + s.values[i]) / yMax).toFloat().coerceIn(0f, 1f)
                if (i == 0) path.moveTo(xAt(i, n), y) else path.lineTo(xAt(i, n), y)
            }
            for (i in n - 1 downTo 0) path.lineTo(xAt(i, n), bottom - h * (base[i] / yMax).toFloat().coerceIn(0f, 1f))
            path.close()
            if (s.hatched) c.drawPath(path, hatchPaint) else { fill.color = s.color; c.drawPath(path, fill) }
            c.drawPath(path, gap)
            for (i in 0 until n) base[i] += s.values[i]
        }
        c.restore()
        // x axis: first, last, and evenly spaced labels that do not collide
        val minGap = axisText.measureText("Sep 26") + 14f * d
        var lastRight = -1e9f
        val idxs = (0 until n).filter { it == 0 || it == n - 1 || (it % maxOf(1, (n - 1) / 4) == 0) }
        for (i in idxs) {
            val t = labels[i]
            val tw = axisText.measureText(t)
            val x = (xAt(i, n) - tw / 2).coerceIn(0f, width - tw)
            if (x < lastRight + minGap - tw && i != n - 1) continue
            if (i == n - 1 && x < lastRight + 8 * d) continue
            c.drawText(t, x, height - 5f * d, axisText)
            lastRight = x + tw
        }
        if (selected in 0 until n) {
            val x = xAt(selected, n)
            c.drawLine(x, top, x, bottom, cursor)
            // marker dots on every stack boundary with 2dp surface ring
            var acc = 0.0
            for (s in series) {
                acc += s.values[selected]
                val y = bottom - h * (acc / yMax).toFloat().coerceIn(0f, 1f)
                dot.color = surface; c.drawCircle(x, y, 5f * d, dot)
                dot.color = if (s.hatched) onSurface else s.color; c.drawCircle(x, y, 3f * d, dot)
            }
            if (touching) drawBubble(c, x, top)
        }
    }

    private fun drawBubble(c: Canvas, cx: Float, top: Float) {
        val rows = series.mapIndexed { i, s -> s to s.values[selected] }.filter { it.second >= 0.5 }.sortedByDescending { it.second }.take(7)
        val title = labels[selected]
        val rowH = 18f * d
        val pad = 10f * d
        val maxW = width * 0.78f - pad * 2
        fun fit(t: String, room: Float): String {
            if (bubbleText.measureText(t) <= room) return t
            var e = t.length
            while (e > 3 && bubbleText.measureText(t.substring(0, e) + "…") > room) e--
            return t.substring(0, e).trimEnd() + "…"
        }
        var w = bubbleTitle.measureText(title)
        rows.forEach { (s, v) -> w = maxOf(w, 14f * d + minOf(bubbleText.measureText(s.name), maxW * 0.62f) + 14f * d + bubbleVal.measureText("%.1f%%".format(v))) }
        val bw = minOf(w, maxW) + pad * 2
        val bh = pad * 2 + 18f * d + rows.size * rowH
        val rawLeft = if (cx + 14 * d + bw < width - 4 * d) cx + 14 * d else cx - 14 * d - bw
        val left = rawLeft.coerceIn(4 * d, (width - bw - 4 * d).coerceAtLeast(4 * d))
        val r = RectF(left, top + 4 * d, left + bw, top + 4 * d + bh)
        c.drawRoundRect(r, 12 * d, 12 * d, bubbleFill)
        c.drawRoundRect(r, 12 * d, 12 * d, bubbleLine)
        var y = r.top + pad + 12f * d
        c.drawText(title, r.left + pad, y, bubbleTitle)
        y += 6f * d
        rows.forEach { (s, v) ->
            y += rowH
            dot.color = if (s.hatched) ContextCompat.getColor(context, R.color.viz_neutral) else s.color
            c.drawCircle(r.left + pad + 4f * d, y - 4f * d, 4f * d, dot)
            c.drawText(fit(s.name, bw - pad * 2 - 14f * d - 14f * d - bubbleVal.measureText("%.1f%%".format(v))), r.left + pad + 14f * d, y, bubbleText)
            c.drawText("%.1f%%".format(v), r.right - pad, y, bubbleVal)
        }
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        val n = labels.size
        if (n == 0) return false
        parent?.requestDisallowInterceptTouchEvent(true)
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE -> {
                touching = true
                val w = width - padL - padR
                val frac = ((e.x - padL) / w).coerceIn(0f, 1f)
                val idx = if (n == 1) 0 else Math.round(frac * (n - 1))
                if (idx != selected) { selected = idx; onScrub?.invoke(idx) }
                invalidate()
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> {
                touching = false
                parent?.requestDisallowInterceptTouchEvent(false)
                invalidate()
            }
        }
        return true
    }
}
