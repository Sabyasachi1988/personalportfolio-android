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

    /**
     * When true, renders as a row of compact horizontally-scrollable
     * chips instead of a full-width vertical list - set this and wrap
     * the view in a HorizontalScrollView BEFORE calling setSlices().
     * Used by the Holdings screen's per-fund header, where a long
     * vertical legend (one row per holding) was pushing the actual
     * holdings list far down the screen, unnecessarily duplicating
     * information the cards right below it already show. The Dashboard
     * and Allocation screens keep the default vertical list, since
     * there the legend IS the primary way to read exact percentages per
     * category.
     */
    var chipMode: Boolean = false

    init {
        orientation = VERTICAL
    }

    fun setSlices(slices: List<DonutChartView.Slice>) {
        removeAllViews()
        orientation = if (chipMode) HORIZONTAL else VERTICAL
        val colors = ChartColors.palette(context)
        val textColor = ContextCompat.getColor(context, R.color.colorOnSurface)
        val positive = slices.filter { it.percent > 0f }
        val density = resources.displayMetrics.density

        for ((index, slice) in positive.withIndex()) {
            val row = LinearLayout(context).apply {
                orientation = HORIZONTAL
                if (chipMode) {
                    setPadding((10 * density).toInt(), (6 * density).toInt(), (10 * density).toInt(), (6 * density).toInt())
                    background = GradientDrawable().apply {
                        shape = GradientDrawable.RECTANGLE
                        cornerRadius = 20f * density
                        setColor(ContextCompat.getColor(context, R.color.colorSurfaceVariant))
                    }
                    layoutParams = LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT).apply {
                        marginEnd = (8 * density).toInt()
                    }
                } else {
                    setPadding(0, 8, 0, 8)
                }
            }

            val swatch = android.view.View(context).apply {
                val size = (14 * density).toInt()
                layoutParams = LayoutParams(size, size).apply {
                    gravity = android.view.Gravity.CENTER_VERTICAL
                    marginEnd = (8 * density).toInt()
                }
                background = GradientDrawable().apply {
                    shape = GradientDrawable.OVAL
                    setColor(slice.color ?: colors[index % colors.size])
                }
            }

            val label = TextView(context).apply {
                text = FundNameFormatter.shorten(slice.label)
                textSize = 13f
                setTextColor(textColor)
                maxLines = 1
                ellipsize = android.text.TextUtils.TruncateAt.END
                if (chipMode) {
                    // Fixed max width per chip so a long fund name can't
                    // make one chip dominate the row - it's already
                    // fully readable in the holding card right below.
                    layoutParams = LayoutParams((90 * density).toInt(), LayoutParams.WRAP_CONTENT)
                } else {
                    layoutParams = LayoutParams(0, LayoutParams.WRAP_CONTENT, 1f)
                }
            }

            val percentText = TextView(context).apply {
                text = String.format(Locale.getDefault(), "%.1f%%", slice.percent)
                textSize = 13f
                setTextColor(textColor)
                if (chipMode) {
                    setPadding((6 * density).toInt(), 0, 0, 0)
                }
            }

            row.addView(swatch)
            row.addView(label)
            row.addView(percentText)
            addView(row)
        }
    }
}
