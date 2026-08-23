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

    /**
     * If set, each legend row becomes tappable and invokes this with the
     * row's label - used by the expanded donut dialog (see
     * DonutExpansionDialog) to let a full-width vertical legend act as
     * an alternate hit target for "select this segment", alongside
     * tapping the donut ring itself. Null (the default) leaves rows
     * non-interactive, as before.
     */
    var onRowTapped: ((label: String) -> Unit)? = null

    init {
        orientation = VERTICAL
    }

    /**
     * Strips whatever leading words are common to every label in the
     * group before display. Real-world portfolios are often
     * single-AMC-heavy (e.g. several "Nippon India ..." funds), and a
     * fixed-width chip's end-ellipsis was truncating every single one
     * down to the identical, useless "Nippon Indi…" - the shared AMC
     * name, not the actually distinguishing part of the fund's name.
     * This is intentionally generic (no AMC names hardcoded): it just
     * finds the longest common leading word sequence across whatever
     * labels are actually being shown, so it does the right thing
     * whether the shared prefix is an AMC name, a common share class
     * suffix pattern, or nothing at all (single-fund or already-diverse
     * portfolios are left untouched).
     */
    private fun commonLeadingWordCount(labels: List<String>): Int {
        if (labels.size < 2) return 0
        val wordLists = labels.map { it.trim().split(Regex("\\s+")) }
        val shortest = wordLists.minOf { it.size }
        var count = 0
        while (count < shortest) {
            val word = wordLists[0][count].lowercase(Locale.ROOT)
            if (wordLists.all { it[count].lowercase(Locale.ROOT) == word }) {
                count++
            } else {
                break
            }
        }
        // Never strip EVERY word of the shortest label - that would
        // leave one chip blank while its siblings still show their
        // remaining distinguishing words.
        return count.coerceAtMost(shortest - 1)
    }

    private fun stripLeadingWords(text: String, count: Int): String {
        if (count <= 0) return text
        val words = text.trim().split(Regex("\\s+"))
        if (count >= words.size) return text
        return words.drop(count).joinToString(" ")
    }

    fun setSlices(slices: List<DonutChartView.Slice>) {
        removeAllViews()
        orientation = if (chipMode) HORIZONTAL else VERTICAL
        val colors = ChartColors.palette(context)
        val textColor = ContextCompat.getColor(context, R.color.colorOnSurface)
        val positive = slices.filter { it.percent > 0f }
        val density = resources.displayMetrics.density

        val shortenedLabels = positive.map { FundNameFormatter.shorten(it.label) }
        val skipWords = commonLeadingWordCount(shortenedLabels)

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
                val tapHandler = onRowTapped
                if (tapHandler != null) {
                    isClickable = true
                    isFocusable = true
                    val outValue = android.util.TypedValue()
                    context.theme.resolveAttribute(android.R.attr.selectableItemBackground, outValue, true)
                    foreground = ContextCompat.getDrawable(context, outValue.resourceId)
                    setOnClickListener { tapHandler(slice.label) }
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
                text = stripLeadingWords(shortenedLabels[index], skipWords)
                textSize = 13f
                setTextColor(textColor)
                maxLines = 1
                ellipsize = android.text.TextUtils.TruncateAt.END
                if (chipMode) {
                    // Fixed max width per chip so a long fund name can't
                    // make one chip dominate the row - with the shared
                    // AMC prefix already stripped above, what's left to
                    // show here is the part that actually distinguishes
                    // this fund from its siblings.
                    layoutParams = LayoutParams((130 * density).toInt(), LayoutParams.WRAP_CONTENT)
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
