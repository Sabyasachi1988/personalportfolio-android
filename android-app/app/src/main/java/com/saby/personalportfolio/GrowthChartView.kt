package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Path
import android.graphics.RectF
import android.util.AttributeSet
import android.view.MotionEvent
import android.view.View
import androidx.core.content.ContextCompat
import com.google.android.material.color.MaterialColors

/** Multi-line "growth of 100" chart with a touch crosshair and a value bubble. All series share the same x positions. */
class GrowthChartView @JvmOverloads constructor(context: Context, attrs: AttributeSet? = null) : View(context, attrs) {
    class Line(val name: String, val color: Int, val values: DoubleArray)

    private var labels: List<String> = emptyList()
    private var lines: List<Line> = emptyList()
    private var selected = -1
    private var touching = false

    private val d = resources.displayMetrics.density
    private val sp = resources.displayMetrics.scaledDensity
    private val surface = MaterialColors.getColor(this, com.google.android.material.R.attr.colorSurface, Color.WHITE)
    private val onSurface = MaterialColors.getColor(this, com.google.android.material.R.attr.colorOnSurface, Color.BLACK)
    private val grid = Paint().apply { color = ContextCompat.getColor(context, R.color.viz_grid); strokeWidth = 1f }
    private val axis = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = ContextCompat.getColor(context, R.color.viz_text_muted); textSize = 11f * sp }
    private val stroke = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE; strokeWidth = 2.2f * d; strokeJoin = Paint.Join.ROUND; strokeCap = Paint.Cap.ROUND }
    private val cursor = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; strokeWidth = 1.2f * d; alpha = 190 }
    private val dot = Paint(Paint.ANTI_ALIAS_FLAG)
    private val bubble = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = surface }
    private val bubbleLine = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE; color = ContextCompat.getColor(context, R.color.viz_grid); strokeWidth = 1f * d }
    private val tTitle = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; textSize = 12.5f * sp; isFakeBoldText = true }
    private val tText = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; textSize = 12f * sp }
    private val tVal = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = onSurface; textSize = 12f * sp; textAlign = Paint.Align.RIGHT; isFakeBoldText = true }

    private var lo = 0.0
    private var hi = 1.0
    private val padL get() = 40f * d
    private val padR get() = 8f * d
    private val padT get() = 8f * d
    private val padB get() = 22f * d

    fun setData(labels: List<String>, lines: List<Line>) {
        this.labels = labels; this.lines = lines
        selected = labels.size - 1
        var mn = Double.MAX_VALUE; var mx = -Double.MAX_VALUE
        lines.forEach { l -> l.values.forEach { mn = minOf(mn, it); mx = maxOf(mx, it) } }
        if (mn > mx) { mn = 0.0; mx = 1.0 }
        val span = (mx - mn).coerceAtLeast(1.0)
        lo = Math.floor((mn - span * 0.06) / 10) * 10
        hi = Math.ceil((mx + span * 0.06) / 10) * 10
        if (hi <= lo) hi = lo + 10
        invalidate()
    }

    private fun xAt(i: Int, n: Int): Float = if (n <= 1) padL else padL + (width - padL - padR) * i / (n - 1)
    private fun yAt(v: Double): Float = (height - padB - (height - padB - padT) * ((v - lo) / (hi - lo))).toFloat()

    override fun onDraw(c: Canvas) {
        super.onDraw(c)
        val n = labels.size
        for (q in 0..4) {
            val v = lo + (hi - lo) * q / 4
            val y = yAt(v)
            c.drawLine(padL, y, width - padR, y, grid)
            c.drawText("%.0f".format(v), 0f, y + axis.textSize / 3, axis)
        }
        if (n == 0) return
        for (l in lines) {
            val p = Path()
            for (i in 0 until n) { val x = xAt(i, n); val y = yAt(l.values[i]); if (i == 0) p.moveTo(x, y) else p.lineTo(x, y) }
            stroke.color = l.color
            c.drawPath(p, stroke)
        }
        val idxs = listOf(0, (n - 1) / 3, 2 * (n - 1) / 3, n - 1).distinct()
        for (i in idxs) {
            val t = labels[i]; val w = axis.measureText(t)
            c.drawText(t, (xAt(i, n) - w / 2).coerceIn(0f, width - w), height - 5f * d, axis)
        }
        if (selected in 0 until n) {
            val x = xAt(selected, n)
            c.drawLine(x, padT, x, height - padB, cursor)
            for (l in lines) {
                dot.color = surface; c.drawCircle(x, yAt(l.values[selected]), 5f * d, dot)
                dot.color = l.color; c.drawCircle(x, yAt(l.values[selected]), 3f * d, dot)
            }
            if (touching) drawBubble(c, x)
        }
    }

    private fun drawBubble(c: Canvas, cx: Float) {
        val rows = lines.map { it to it.values[selected] }.sortedByDescending { it.second }
        val pad = 10f * d; val rowH = 18f * d
        val maxName = width * 0.45f
        fun fit(t: String): String { var e = t.length; while (e > 3 && tText.measureText(t.substring(0, e) + (if (e < t.length) "…" else "")) > maxName) e--; return t.substring(0, e) + (if (e < t.length) "…" else "") }
        var w = tTitle.measureText(labels[selected])
        rows.forEach { (l, v) -> w = maxOf(w, 14f * d + tText.measureText(fit(l.name)) + 14f * d + tVal.measureText("%.1f".format(v))) }
        val bw = w + pad * 2; val bh = pad * 2 + 18f * d + rows.size * rowH
        val raw = if (cx + 14 * d + bw < width - 4 * d) cx + 14 * d else cx - 14 * d - bw
        val left = raw.coerceIn(4 * d, (width - bw - 4 * d).coerceAtLeast(4 * d))
        val r = RectF(left, padT + 2 * d, left + bw, padT + 2 * d + bh)
        c.drawRoundRect(r, 12 * d, 12 * d, bubble); c.drawRoundRect(r, 12 * d, 12 * d, bubbleLine)
        var y = r.top + pad + 12f * d
        c.drawText(labels[selected], r.left + pad, y, tTitle)
        y += 6f * d
        rows.forEach { (l, v) ->
            y += rowH
            dot.color = l.color; c.drawCircle(r.left + pad + 4f * d, y - 4f * d, 4f * d, dot)
            c.drawText(fit(l.name), r.left + pad + 14f * d, y, tText)
            c.drawText("%.1f".format(v), r.right - pad, y, tVal)
        }
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        val n = labels.size
        if (n == 0) return false
        parent?.requestDisallowInterceptTouchEvent(true)
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_MOVE -> {
                touching = true
                val frac = ((e.x - padL) / (width - padL - padR)).coerceIn(0f, 1f)
                selected = if (n == 1) 0 else Math.round(frac * (n - 1))
                invalidate()
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_CANCEL -> { touching = false; parent?.requestDisallowInterceptTouchEvent(false); invalidate() }
        }
        return true
    }
}
