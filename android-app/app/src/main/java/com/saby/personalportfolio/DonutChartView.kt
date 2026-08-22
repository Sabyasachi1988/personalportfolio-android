package com.saby.personalportfolio

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.RectF
import android.util.AttributeSet
import android.view.View
import androidx.core.content.ContextCompat

class DonutChartView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : View(context, attrs) {

    data class Slice(val label: String, val percent: Float)

    private var slices: List<Slice> = emptyList()
    private val arcPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE }
    private val emptyPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        style = Paint.Style.STROKE
        color = 0x22000000
    }
    private val rectF = RectF()

    private val sliceColors: List<Int> by lazy {
        listOf(
            ContextCompat.getColor(context, R.color.chartSlice1),
            ContextCompat.getColor(context, R.color.chartSlice2),
            ContextCompat.getColor(context, R.color.chartSlice3),
            ContextCompat.getColor(context, R.color.chartSlice4),
            ContextCompat.getColor(context, R.color.chartSlice5),
            ContextCompat.getColor(context, R.color.chartSlice6),
            ContextCompat.getColor(context, R.color.chartSlice7)
        )
    }

    fun setSlices(newSlices: List<Slice>) {
        // Only positive-percent slices are drawable; anything at 0%
        // would just be an invisible zero-length arc anyway.
        slices = newSlices.filter { it.percent > 0f }
        invalidate()
    }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)

        val strokeWidth = width.coerceAtMost(height) * 0.18f
        arcPaint.strokeWidth = strokeWidth
        emptyPaint.strokeWidth = strokeWidth

        val inset = strokeWidth / 2f + 4f
        rectF.set(inset, inset, width - inset, height - inset)

        if (slices.isEmpty()) {
            canvas.drawArc(rectF, 0f, 360f, false, emptyPaint)
            return
        }

        var startAngle = -90f
        for ((index, slice) in slices.withIndex()) {
            val sweep = (slice.percent / 100f) * 360f
            arcPaint.color = sliceColors[index % sliceColors.size]
            canvas.drawArc(rectF, startAngle, sweep, false, arcPaint)
            startAngle += sweep
        }
    }
}
