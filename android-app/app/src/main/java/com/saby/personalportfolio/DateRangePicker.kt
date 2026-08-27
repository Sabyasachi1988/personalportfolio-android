package com.saby.personalportfolio

import android.app.DatePickerDialog
import android.content.Context
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Locale

/**
 * Numeric/calendar date-range entry - the manual counterpart to pinch-
 * zoom on the Returns and Compare charts (see their setWindowByDates
 * methods), for picking an exact range rather than gesturing one. Shows
 * two DatePickerDialogs in sequence (start, then end), both bounded to
 * the series' own actual date range so a picked date is always
 * resolvable rather than landing outside any available data.
 */
object DateRangePicker {
    private val storedFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)

    fun show(context: Context, minDate: String, maxDate: String, onPicked: (start: String, end: String) -> Unit) {
        val minCal = parse(minDate) ?: return
        val maxCal = parse(maxDate) ?: return

        DatePickerDialog(
            context,
            { _, y1, m1, d1 ->
                val startCal = Calendar.getInstance().apply { set(y1, m1, d1) }
                DatePickerDialog(
                    context,
                    { _, y2, m2, d2 ->
                        val endCal = Calendar.getInstance().apply { set(y2, m2, d2) }
                        onPicked(format(startCal), format(endCal))
                    },
                    maxCal.get(Calendar.YEAR), maxCal.get(Calendar.MONTH), maxCal.get(Calendar.DAY_OF_MONTH)
                ).apply {
                    datePicker.minDate = minCal.timeInMillis
                    datePicker.maxDate = maxCal.timeInMillis
                    setTitle("End date")
                }.show()
            },
            minCal.get(Calendar.YEAR), minCal.get(Calendar.MONTH), minCal.get(Calendar.DAY_OF_MONTH)
        ).apply {
            datePicker.minDate = minCal.timeInMillis
            datePicker.maxDate = maxCal.timeInMillis
            setTitle("Start date")
        }.show()
    }

    private fun parse(s: String): Calendar? = try {
        val d = storedFormat.parse(s) ?: return null
        Calendar.getInstance().apply { time = d }
    } catch (e: Exception) {
        null
    }

    private fun format(cal: Calendar): String = storedFormat.format(cal.time)
}
