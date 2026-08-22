package com.saby.personalportfolio

import android.content.Context
import android.graphics.drawable.GradientDrawable
import android.util.AttributeSet
import android.widget.LinearLayout
import android.widget.TextView
import androidx.core.content.ContextCompat
import java.util.Locale

class DonutLegendView @JvmOverloads constructor(
    context: Context,
    attrs: AttributeSet? = null
) : LinearLayout(context, attrs) {

    init {
        orientation = VERTICAL
    }

    fun setSlices(slices: List<DonutChartView.Slice>) {
        removeAllViews()
        val colors = ChartColors.palette(context)
        val textColor = ContextCompat.getColor(context, R.color.colorOnSurface)
        val positive = slices.filter { it.percent > 0f }

        for ((index, slice) in positive.withIndex()) {
            val row = LinearLayout(context).apply {
                orientation = HORIZONTAL
                setPadding(0, 8, 0, 8)
            }

            val swatch = android.view.View(context).apply {
                val size = (14 * resources.displayMetrics.density).toInt()
                layoutParams = LayoutParams(size, size).apply {
                    gravity = android.view.Gravity.CENTER_VERTICAL
                    marginEnd = (10 * resources.displayMetrics.density).toInt()
                }
                background = GradientDrawable().apply {
                    shape = GradientDrawable.OVAL
                    setColor(slice.color ?: colors[index % colors.size])
                }
            }

            val label = TextView(context).apply {
                layoutParams = LayoutParams(0, LayoutParams.WRAP_CONTENT, 1f)
                text = FundNameFormatter.shorten(slice.label)
                textSize = 13f
                setTextColor(textColor)
                maxLines = 1
                ellipsize = android.text.TextUtils.TruncateAt.END
            }

            val percentText = TextView(context).apply {
                text = String.format(Locale.getDefault(), "%.1f%%", slice.percent)
                textSize = 13f
                setTextColor(textColor)
            }

            row.addView(swatch)
            row.addView(label)
            row.addView(percentText)
            addView(row)
        }
    }
}
