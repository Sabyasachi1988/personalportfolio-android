package com.saby.personalportfolio

/**
 * Highs and lows of the portfolio's growth index, for the Custom period
 * dialog's "from a high or low until today" cards. Ported from the stock
 * tracker's Highs.kt; the series here is the contribution-neutral growth
 * index (deposits and withdrawals removed) rather than a price, because the
 * portfolio's raw value hits new highs every time money is added.
 */
object PortfolioHighs {

    /** Growth index, 1.0 at the first point; deposits and withdrawals removed (same index as the window summary's drawdown). */
    fun growthIndex(invested: DoubleArray, value: DoubleArray): DoubleArray {
        val idx = DoubleArray(value.size)
        if (idx.isEmpty()) return idx
        idx[0] = 1.0
        for (i in 1 until value.size) {
            val prev = value[i - 1]
            val flow = invested[i] - invested[i - 1]
            idx[i] = if (prev > 1.0) idx[i - 1] * ((value[i] - flow) / prev) else idx[i - 1]
        }
        return idx
    }

    class Info(val highDay: Int, val high: Double, val lowDay: Int, val low: Double, val last: Double, val lastDay: Int, val startDay: Int)

    /** [months] <= 0 means the whole history. Null when there is no data. */
    fun info(days: IntArray, px: DoubleArray, months: Int): Info? {
        val n = px.size
        if (n < 1) return null
        val lastDay = days[n - 1]
        val start = if (months <= 0) days[0] else RollingMath.monthsBefore(lastDay, months)
        var hi = Double.NEGATIVE_INFINITY; var hiDay = lastDay
        var lo = Double.POSITIVE_INFINITY; var loDay = lastDay
        var i = n - 1
        while (i >= 0 && days[i] >= start) {
            val v = px[i]
            if (v > hi) { hi = v; hiDay = days[i] } // scanning backwards with ">" keeps the most recent of equal highs
            if (v > 0 && v < lo) { lo = v; loDay = days[i] }
            i--
        }
        if (hi <= 0.0 || lo == Double.POSITIVE_INFINITY) return null
        return Info(hiDay, hi, loDay, lo, px[n - 1], lastDay, start)
    }

    /** A ready-made window from [day] (a high or a low) to the latest date. [pct] is where the portfolio is now relative to [level]. */
    class Anchor(val title: String, val day: Int, val level: Double, val pct: Double, val isHigh: Boolean)

    /** Highs and lows worth zooming to, biggest window first. Same-day anchors are merged; anchors within the last few days are left out. */
    fun anchors(days: IntArray, px: DoubleArray): List<Anchor> {
        if (px.size < 2) return emptyList()
        val lastDay = days[px.size - 1]
        class Raw(val names: ArrayList<String>, val day: Int, val level: Double, val last: Double, val high: Boolean)
        val raw = ArrayList<Raw>()
        fun add(name: String, day: Int, level: Double, last: Double, high: Boolean) {
            if (lastDay - day < 4) return
            raw.firstOrNull { it.day == day && it.high == high }?.let { it.names.add(name); return }
            raw.add(Raw(arrayListOf(name), day, level, last, high))
        }
        for ((m, name) in listOf(0 to "All-time", 12 to "52-week", 6 to "6-month", 3 to "3-month", 1 to "1-month"))
            info(days, px, m)?.let { add(name, it.highDay, it.high, it.last, true) }
        for ((m, name) in listOf(0 to "All-time", 12 to "52-week", 6 to "6-month"))
            info(days, px, m)?.let { add(name, it.lowDay, it.low, it.last, false) }
        return raw.map {
            Anchor(it.names.joinToString(" / ") + if (it.high) " high" else " low", it.day, it.level, (it.last / it.level - 1.0) * 100.0, it.high)
        }
    }
}
