package com.saby.personalportfolio

import android.content.Context
import android.graphics.Color
import android.graphics.drawable.GradientDrawable
import android.util.AttributeSet
import android.widget.LinearLayout
import android.widget.TextView
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
                    setColor(colors[index % colors.size])
                }
            }

            val label = TextView(context).apply {
                layoutParams = LayoutParams(0, LayoutParams.WRAP_CONTENT, 1f)
                text = slice.label
                textSize = 13f
                setTextColor(currentTextColorOrDefault())
            }

            val percentText = TextView(context).apply {
                text = String.format(Locale.getDefault(), "%.1f%%", slice.percent)
                textSize = 13f
                setTextColor(currentTextColorOrDefault())
            }

            row.addView(swatch)
            row.addView(label)
            row.addView(percentText)
            addView(row)
        }
    }

    // Falls back to a mid-grey if no theme-aware text color is easily
    // available in this context - avoids hardcoding pure black, which
    // would be unreadable in dark mode.
    private fun currentTextColorOrDefault(): Int {
        val typedValue = android.util.TypedValue()
        val resolved = context.theme.resolveAttribute(android.R.attr.textColorPrimary, typedValue, true)
        return if (resolved) typedValue.data else Color.GRAY
    }
}
