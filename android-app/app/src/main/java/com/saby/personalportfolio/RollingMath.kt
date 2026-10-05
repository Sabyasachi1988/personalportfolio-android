package com.saby.personalportfolio

import kotlin.math.abs
import kotlin.math.pow

/** One rolling window: it ends at point [endIndex] of the series, and [percent] is the return over it. */
data class RollingPoint(val endIndex: Int, val percent: Double)

/**
 * Result of [RollingMath.compute] for one window length.
 * [seriesStartIndex] is the first point the growth index is defined at, or -1
 * if the portfolio never reached [RollingMath.MIN_START_VALUE].
 */
data class RollingResult(
    val months: Int,
    val annualised: Boolean,
    val points: List<RollingPoint>,
    val seriesStartIndex: Int,
    val latest: Double?,
    val median: Double?,
    val worst: RollingPoint?,
    val best: RollingPoint?,
    val positiveSharePct: Double?,
    /** Plain average of every window's return - the headline "rolling return", as fund factsheets quote it. */
    val mean: Double? = null
)

/**
 * Portfolio rolling returns, time-weighted.
 *
 * A portfolio, unlike a fund, has no price series of its own: its value moves
 * when you add or withdraw money as well as when markets move. So this builds
 * a GROWTH INDEX from the weekly Invested/Value checkpoints that removes the
 * effect of deposits and withdrawals - the same idea as a fund's NAV - and
 * then treats that index exactly like a price series: for every window of the
 * chosen length ending at a checkpoint, the return is index(end)/index(start).
 *
 * Per week, with flow = change in Invested (net money added that week):
 *     r = (Value_now - Value_prev - flow) / (Value_prev + flow / 2)
 * i.e. the week's market gain over the money that was working during it,
 * assuming the flow arrived mid-week (Modified Dietz). Weekly checkpoints
 * don't say which day a deposit landed, so mid-week is the neutral
 * assumption; the error is small unless a deposit is large relative to the
 * balance, which is why the index only starts once the value first reaches
 * [MIN_START_VALUE] (the first weeks, with a tiny balance and big deposits,
 * can't be measured reliably).
 *
 * Windows of a year or more are annualised (CAGR) over their ACTUAL elapsed
 * days - a weekly checkpoint can sit a few days before the exact calendar
 * date, so the window is measured honestly rather than rounded. Shorter
 * windows are plain total returns, never annualised (same convention as the
 * fund Returns screen).
 */
object RollingMath {

    const val MIN_START_VALUE = 100000.0

    /** Growth index, 1.0 at the first point with Value >= [MIN_START_VALUE]; NaN before it. */
    fun growthIndex(invested: DoubleArray, value: DoubleArray): DoubleArray {
        val n = value.size
        val idx = DoubleArray(n) { Double.NaN }
        val start = value.indexOfFirst { it >= MIN_START_VALUE }
        if (start < 0) return idx
        idx[start] = 1.0
        for (i in start + 1 until n) {
            val prev = value[i - 1]
            val flow = invested[i] - invested[i - 1]
            val base = prev + flow / 2.0
            val r = if (base > 1.0) ((value[i] - prev - flow) / base).coerceAtLeast(-0.999) else 0.0
            idx[i] = idx[i - 1] * (1.0 + r)
        }
        return idx
    }

    /** Time-weighted rolling return: what the investments did, with deposits and withdrawals removed. */
    fun compute(days: IntArray, invested: DoubleArray, value: DoubleArray, months: Int): RollingResult {
        val idx = growthIndex(invested, value)
        val annualise = months >= 12
        return assemble(days, value, months) { i, j ->
            val elapsed = days[j] - days[i]
            val ratio = idx[j] / idx[i]
            if (annualise) (ratio.pow(365.25 / elapsed) - 1.0) * 100.0 else (ratio - 1.0) * 100.0
        }
    }

    /**
     * Money-weighted rolling return: what YOUR money earned, given when you put it in.
     * For each window the opening value counts as money invested on day one, each
     * deposit/withdrawal counts on its own date, and the closing value is what came
     * back. Windows of a year or more give the annualised rate (XIRR); shorter windows
     * give the total return on the money at work (Modified Dietz) - an annualised rate
     * over a few weeks is meaningless. Both come from [WindowMath], so they agree with
     * the Progression window card.
     */
    fun computeMoneyWeighted(days: IntArray, invested: DoubleArray, value: DoubleArray, months: Int): RollingResult {
        val annualise = months >= 12
        return assemble(days, value, months) { i, j ->
            val s = WindowMath.compute(days, invested, value, i, j)
            if (s == null) Double.NaN
            else if (annualise) (s.xirrPct ?: Double.NaN) else (s.returnPct ?: Double.NaN)
        }
    }

    /** Shared windowing: for each end point j, finds the start checkpoint [months] back and asks [pctFor] for the return over (i, j). */
    private fun assemble(days: IntArray, value: DoubleArray, months: Int, pctFor: (Int, Int) -> Double): RollingResult {
        val n = days.size
        val start = value.indexOfFirst { it >= MIN_START_VALUE }
        val annualise = months >= 12
        if (n < 2 || start < 0 || months < 1) {
            return RollingResult(months, annualise, emptyList(), start, null, null, null, null, null)
        }
        val points = ArrayList<RollingPoint>()
        var i = start
        for (j in start + 1 until n) {
            val target = monthsBefore(days[j], months)
            if (target < days[start]) continue // not enough history behind this point yet
            // i = last checkpoint on or before the target date (monotone as j grows).
            while (i + 1 <= j && days[i + 1] <= target) i++
            if (days[j] - days[i] <= 0) continue
            val pct = pctFor(i, j)
            if (pct.isNaN() || pct.isInfinite()) continue
            points.add(RollingPoint(j, pct))
        }
        if (points.isEmpty()) {
            return RollingResult(months, annualise, emptyList(), start, null, null, null, null, null)
        }
        val sorted = points.map { it.percent }.sorted()
        val median = if (sorted.size % 2 == 1) sorted[sorted.size / 2]
        else (sorted[sorted.size / 2 - 1] + sorted[sorted.size / 2]) / 2.0
        return RollingResult(
            months, annualise, points, start,
            latest = points.last().percent,
            median = median,
            worst = points.minByOrNull { it.percent },
            best = points.maxByOrNull { it.percent },
            positiveSharePct = points.count { it.percent > 0 } * 100.0 / points.size,
            mean = sorted.average()
        )
    }

    /** The day-number [months] calendar months before [day]; the day-of-month is clamped (31 Mar - 1 month = 28/29 Feb). */
    /** Whole calendar months from day [from] to day [to] (rounded down, never negative). */
    fun monthsBetween(from: Int, to: Int): Int {
        val (y0, m0, d0) = civilFromDays(from)
        val (y1, m1, d1) = civilFromDays(to)
        var m = (y1 - y0) * 12 + (m1 - m0)
        if (d1 < d0) m -= 1
        return m.coerceAtLeast(0)
    }

    fun monthsBefore(day: Int, months: Int): Int {
        val (y, m, d) = civilFromDays(day)
        val total = y * 12 + (m - 1) - months
        val ny = total.floorDiv(12)
        val nm = total.mod(12) + 1
        return daysFromCivil(ny, nm, minOf(d, daysInMonth(ny, nm)))
    }

    private fun isLeap(y: Int) = (y % 4 == 0 && y % 100 != 0) || y % 400 == 0

    private fun daysInMonth(y: Int, m: Int): Int = when (m) {
        1, 3, 5, 7, 8, 10, 12 -> 31
        4, 6, 9, 11 -> 30
        else -> if (isLeap(y)) 29 else 28
    }

    // Howard Hinnant's civil-date algorithms (days since 1970-01-01); no java.time because minSdk is 24.
    fun daysFromCivil(year: Int, month: Int, day: Int): Int {
        val y = if (month <= 2) year - 1 else year
        val era = (if (y >= 0) y else y - 399) / 400
        val yoe = y - era * 400
        val doy = (153 * (if (month > 2) month - 3 else month + 9) + 2) / 5 + day - 1
        val doe = yoe * 365 + yoe / 4 - yoe / 100 + doy
        return era * 146097 + doe - 719468
    }

    fun civilFromDays(dayNumber: Int): Triple<Int, Int, Int> {
        val z = dayNumber + 719468
        val era = (if (z >= 0) z else z - 146096) / 146097
        val doe = z - era * 146097
        val yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365
        val y = yoe + era * 400
        val doy = doe - (365 * yoe + yoe / 4 - yoe / 100)
        val mp = (5 * doy + 2) / 153
        val d = doy - (153 * mp + 2) / 5 + 1
        val m = if (mp < 10) mp + 3 else mp - 9
        return Triple(if (m <= 2) y + 1 else y, m, d)
    }
}
