package com.saby.personalportfolio

import kotlin.math.abs
import kotlin.math.pow

/**
 * Figures for the stretch of the Progression chart currently on screen.
 * Everything is derived from the already-loaded points (Invested and Value
 * per date, always INR), so it can update live while panning with no
 * bridge call.
 *
 * The point of this class is to separate MARKET gain from MONEY ADDED:
 * Value rising because the person bought more is not performance.
 *
 *  - marketGain  = (end Value - start Value) - net money added in the window
 *  - returnPct   = marketGain / money-at-work, where money-at-work is the
 *                  start Value plus each contribution weighted by the share
 *                  of the window it was actually invested for (Modified
 *                  Dietz). Dividing by the start Value alone would make a
 *                  window that starts small look absurdly good.
 *  - xirrPct     = annualised money-weighted rate, only for windows of a
 *                  year or more: annualising a few weeks gives meaningless
 *                  numbers. Cash flows are taken from changes in Invested
 *                  at each point's date, so on weekly data a flow can be
 *                  off by up to six days - negligible over a year.
 *  - maxDrawdownPct = deepest peak-to-trough fall of a growth index that
 *                  strips out contributions step by step (a plain fall in
 *                  Value would be hidden by money being added, or inflated
 *                  by money being withdrawn).
 *
 * Any figure that can't be computed sensibly is null rather than a guess.
 */
data class WindowSummary(
    val startDay: Int,
    val endDay: Int,
    val startValue: Double,
    val endValue: Double,
    val netInvested: Double,
    val marketGain: Double,
    val returnPct: Double?,
    val xirrPct: Double?,
    val maxDrawdownPct: Double?
) {
    val spanDays: Int get() = endDay - startDay
}

object WindowMath {

    const val MIN_XIRR_SPAN_DAYS = 365

    /** Days since 1970-01-01 for a "yyyy-MM-dd" date, or null if malformed. No java.time: minSdk is 24. */
    fun dayNumber(isoDate: String): Int? {
        val parts = isoDate.split("-")
        if (parts.size != 3) return null
        val y = parts[0].toIntOrNull() ?: return null
        val m = parts[1].toIntOrNull() ?: return null
        val d = parts[2].toIntOrNull() ?: return null
        if (m !in 1..12 || d !in 1..31) return null
        // Howard Hinnant's days_from_civil.
        val yy = if (m <= 2) y - 1 else y
        val era = (if (yy >= 0) yy else yy - 399) / 400
        val yoe = yy - era * 400
        val doy = (153 * (if (m > 2) m - 3 else m + 9) + 2) / 5 + d - 1
        val doe = yoe * 365 + yoe / 4 - yoe / 100 + doy
        return era * 146097 + doe - 719468
    }

    /**
     * [days], [invested], [value] are parallel arrays over the whole loaded
     * series (ascending by day); [from]..[to] is the visible window, inclusive.
     */
    fun compute(days: IntArray, invested: DoubleArray, value: DoubleArray, from: Int, to: Int): WindowSummary? {
        if (from < 0 || to >= days.size || to <= from) return null
        val span = days[to] - days[from]
        if (span <= 0) return null

        val netInvested = invested[to] - invested[from]
        val marketGain = (value[to] - value[from]) - netInvested

        // Modified Dietz.
        var moneyAtWork = value[from]
        for (i in from + 1..to) {
            val flow = invested[i] - invested[i - 1]
            moneyAtWork += flow * (days[to] - days[i]).toDouble() / span
        }
        val returnPct = if (moneyAtWork > 1.0) marketGain / moneyAtWork * 100.0 else null

        // Contribution-neutral growth index -> max drawdown.
        var index = 1.0
        var peak = 1.0
        var worst = 0.0
        var steps = 0
        for (i in from + 1..to) {
            val prev = value[i - 1]
            if (prev <= 1.0) continue
            val flow = invested[i] - invested[i - 1]
            index *= (value[i] - flow) / prev
            steps++
            if (index > peak) peak = index
            val dd = index / peak - 1.0
            if (dd < worst) worst = dd
        }
        val maxDrawdownPct = if (steps > 0) worst * 100.0 else null

        val xirrPct = if (span >= MIN_XIRR_SPAN_DAYS) windowXirr(days, invested, value, from, to) else null

        return WindowSummary(
            days[from], days[to], value[from], value[to], netInvested, marketGain,
            returnPct, xirrPct, maxDrawdownPct
        )
    }

    /** Annualised money-weighted return over the window, in percent, or null if there is no sensible root. */
    private fun windowXirr(days: IntArray, invested: DoubleArray, value: DoubleArray, from: Int, to: Int): Double? {
        val t0 = days[from]
        val amounts = ArrayList<Double>()
        val years = ArrayList<Double>()
        // Opening balance is treated as money put in at the start.
        amounts.add(-value[from]); years.add(0.0)
        for (i in from + 1..to) {
            val flow = invested[i] - invested[i - 1]
            if (abs(flow) > 0.005) {
                amounts.add(-flow); years.add((days[i] - t0) / 365.0)
            }
        }
        amounts.add(value[to]); years.add((days[to] - t0) / 365.0)

        fun npv(rate: Double): Double {
            var sum = 0.0
            for (k in amounts.indices) sum += amounts[k] * (1.0 + rate).pow(-years[k])
            return sum
        }
        var lo = -0.99
        var hi = 10.0
        var fLo = npv(lo)
        val fHi = npv(hi)
        if (fLo.isNaN() || fHi.isNaN() || fLo * fHi > 0) return null
        repeat(200) {
            val mid = (lo + hi) / 2
            val fMid = npv(mid)
            if (fMid.isNaN()) return null
            if (fLo * fMid <= 0) { hi = mid } else { lo = mid; fLo = fMid }
        }
        return (lo + hi) / 2 * 100.0
    }
}
