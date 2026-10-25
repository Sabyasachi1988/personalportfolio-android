package com.saby.personalportfolio

import androidx.appcompat.app.AppCompatActivity
import java.util.Locale

/**
 * The app-wide "Custom" period button for price charts (fund, index, comparison).
 * Opens the same dialog as the Progression screen and the stock tracker:
 * from/to dates, "last N days/weeks/months/years", and, when [prices] is given,
 * shortcuts from the series' highs and lows until today.
 */
object ChartCustomPeriod {
    private fun iso(day: Int): String {
        val (y, m, d) = RollingMath.civilFromDays(day)
        return String.format(Locale.US, "%04d-%02d-%02d", y, m, d)
    }

    fun show(a: AppCompatActivity, dates: List<String>, prices: DoubleArray?, onRange: (String, String) -> Unit) {
        if (dates.size < 2) return
        val days = IntArray(dates.size) { WindowMath.dayNumber(dates[it]) ?: 0 }
        val lastDay = days.last()
        val prefs = a.getSharedPreferences("chart_custom_prefs", AppCompatActivity.MODE_PRIVATE)
        val anchors = if (prices != null && prices.size == days.size && prices.all { it > 0.0 })
            PortfolioHighs.anchors(days, prices) else emptyList()
        CustomPeriodDialog.show(
            a, days.first(), lastDay,
            prefs.getInt("look_n", 1), (prefs.getString("look_unit", "Y") ?: "Y")[0],
            onDates = { from, to -> onRange(iso(from), iso(to)) },
            onLookback = { n, unit ->
                prefs.edit().putInt("look_n", n).putString("look_unit", unit.toString()).apply()
                val start = when (unit) {
                    'D' -> lastDay - n
                    'W' -> lastDay - 7 * n
                    'M' -> RollingMath.monthsBefore(lastDay, n)
                    else -> RollingMath.monthsBefore(lastDay, 12 * n)
                }
                onRange(iso(start), iso(lastDay))
            },
            anchors = anchors,
            onAnchor = { an -> onRange(iso(an.day), iso(lastDay)) }
        )
    }
}
