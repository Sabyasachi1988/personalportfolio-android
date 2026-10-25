package com.saby.personalportfolio

import android.text.InputType
import android.view.Gravity
import android.view.ViewGroup
import android.widget.ArrayAdapter
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.Spinner
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import com.google.android.material.bottomsheet.BottomSheetDialog
import com.google.android.material.card.MaterialCardView
import com.google.android.material.datepicker.CalendarConstraints
import com.google.android.material.datepicker.MaterialDatePicker
import com.google.android.material.dialog.MaterialAlertDialogBuilder
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone

/**
 * "Custom" period chooser, ported from the stock tracker app: a bottom sheet
 * of cards - optional "from a high or low until today" windows, then "Pick
 * start and end dates" (a calendar range picker) and "Look back from the
 * latest date" (the last N days, weeks, months or years).
 *
 * Days are epoch days (days since 1970-01-01), the same unit [WindowMath] uses.
 */
object CustomPeriodDialog {
    private const val DAY_MS = 86_400_000L
    private val UNITS = listOf("Days", "Weeks", "Months", "Years")

    /**
     * @param firstDay earliest selectable date, [lastDay] latest
     * @param onDates the person picked an explicit start and end
     * @param onLookback the last N days/weeks/months/years ending at [lastDay]; unit is one of D, W, M, Y
     * @param anchors optional "from a high or low until today" windows; [onAnchor] gets the one picked
     */
    fun show(
        a: AppCompatActivity, firstDay: Int, lastDay: Int, lookN: Int, lookUnit: Char,
        onDates: (Int, Int) -> Unit,
        onLookback: (Int, Char) -> Unit,
        anchors: List<PortfolioHighs.Anchor> = emptyList(),
        onAnchor: (PortfolioHighs.Anchor) -> Unit = {},
    ) {
        val d = a.resources.displayMetrics.density
        fun dp(v: Int) = (v * d).toInt()
        val neutral = ContextCompat.getColor(a, R.color.colorNeutral)
        val gain = ContextCompat.getColor(a, R.color.colorGain)
        val loss = ContextCompat.getColor(a, R.color.colorLoss)

        val sheet = BottomSheetDialog(a)
        val col = LinearLayout(a).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(8), dp(20), dp(24))
        }
        fun text(t: String, sp: Float, color: Int? = null, bold: Boolean = false) = TextView(a).apply {
            text = t
            textSize = sp
            if (color != null) setTextColor(color)
            if (bold) setTypeface(typeface, android.graphics.Typeface.BOLD)
        }
        fun card(): MaterialCardView = MaterialCardView(a).apply {
            radius = dp(16).toFloat()
            cardElevation = dp(1).toFloat()
            isClickable = true
            isFocusable = true
        }
        fun add(v: android.view.View, top: Int) {
            col.addView(v, LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT).apply { topMargin = dp(top) })
        }
        fun heading(t: String, top: Int) =
            add(text(t, 12.5f, neutral, bold = true).apply { letterSpacing = 0.04f }, top)
        fun choose(then: () -> Unit) { sheet.dismiss(); then() }

        add(text("Custom period", 20f, bold = true), 0)

        if (anchors.isNotEmpty()) {
            heading("FROM A HIGH OR LOW UNTIL TODAY", 14)
            for (an in anchors) {
                val c = card()
                val inner = LinearLayout(a).apply { orientation = LinearLayout.VERTICAL; setPadding(dp(14), dp(10), dp(14), dp(10)) }
                val top = LinearLayout(a).apply { orientation = LinearLayout.HORIZONTAL }
                top.addView(text(an.title, 15f, bold = true), LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
                top.addView(text(String.format(Locale.US, "%+.2f%% now", an.pct), 13.5f, if (an.isHigh && an.pct < -0.05) loss else gain, bold = true))
                inner.addView(top)
                val span = lastDay - an.day
                val len = if (span >= 730) String.format(Locale.US, "%.1f years", span / 365.25) else if (span >= 60) "${span / 30} months" else "$span days"
                inner.addView(text("${pretty(an.day)}  ·  $len to today", 12.5f, neutral))
                c.addView(inner)
                c.setOnClickListener { choose { onAnchor(an) } }
                add(c, 8)
            }
        }

        heading("YOUR OWN WINDOW", if (anchors.isNotEmpty()) 20 else 14)
        fun action(icon: String, title: String, sub: String, then: () -> Unit) {
            val c = card()
            val row = LinearLayout(a).apply { orientation = LinearLayout.HORIZONTAL; gravity = Gravity.CENTER_VERTICAL; setPadding(dp(14), dp(12), dp(14), dp(12)) }
            row.addView(text(icon, 22f), LinearLayout.LayoutParams(dp(36), ViewGroup.LayoutParams.WRAP_CONTENT))
            val t = LinearLayout(a).apply { orientation = LinearLayout.VERTICAL }
            t.addView(text(title, 15f, bold = true))
            t.addView(text(sub, 12.5f, neutral))
            row.addView(t, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
            c.addView(row)
            c.setOnClickListener { choose(then) }
            add(c, 8)
        }
        action("📅", "Pick start and end dates", "Choose any two dates on a calendar") { pickDates(a, firstDay, lastDay, onDates) }
        action("⏪", "Look back from the latest date", "The last N days, weeks, months or years") { askLookback(a, lookN, lookUnit, onLookback) }

        sheet.setContentView(ScrollView(a).apply { addView(col) })
        sheet.show()
    }

    private fun pickDates(a: AppCompatActivity, firstDay: Int, lastDay: Int, onDates: (Int, Int) -> Unit) {
        val picker = MaterialDatePicker.Builder.dateRangePicker()
            .setTitleText("Show the portfolio between")
            .setSelection(androidx.core.util.Pair(maxOf(firstDay, lastDay - 365).toLong() * DAY_MS, lastDay.toLong() * DAY_MS))
            .setCalendarConstraints(CalendarConstraints.Builder().setStart(firstDay.toLong() * DAY_MS).setEnd(lastDay.toLong() * DAY_MS).build())
            .build()
        picker.addOnPositiveButtonClickListener { sel ->
            onDates((sel.first / DAY_MS).toInt(), minOf((sel.second / DAY_MS).toInt(), lastDay))
        }
        picker.show(a.supportFragmentManager, "range")
    }

    private fun askLookback(a: AppCompatActivity, lookN: Int, lookUnit: Char, onLookback: (Int, Char) -> Unit) {
        val d = a.resources.displayMetrics.density
        val box = LinearLayout(a).apply {
            orientation = LinearLayout.HORIZONTAL
            setPadding((24 * d).toInt(), (8 * d).toInt(), (24 * d).toInt(), 0)
        }
        val num = EditText(a).apply {
            inputType = InputType.TYPE_CLASS_NUMBER
            setText(lookN.toString())
            layoutParams = LinearLayout.LayoutParams((90 * d).toInt(), ViewGroup.LayoutParams.WRAP_CONTENT)
        }
        val spin = Spinner(a).apply {
            adapter = ArrayAdapter(a, android.R.layout.simple_spinner_dropdown_item, UNITS)
            setSelection("DWMY".indexOf(lookUnit).coerceAtLeast(0))
        }
        box.addView(num); box.addView(spin)
        MaterialAlertDialogBuilder(a)
            .setTitle("Look back from the latest data")
            .setView(box)
            .setPositiveButton("Apply") { _, _ ->
                val n = num.text.toString().toIntOrNull() ?: 0
                if (n in 1..9999) onLookback(n, "DWMY"[spin.selectedItemPosition])
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun pretty(day: Int): String {
        val f = SimpleDateFormat("d MMM yyyy", Locale.US).apply { timeZone = TimeZone.getTimeZone("UTC") }
        return f.format(Date(day.toLong() * DAY_MS))
    }
}
