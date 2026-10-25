package com.saby.personalportfolio

import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.floor
import kotlin.math.log10
import kotlin.math.pow

/**
 * Axis maths for the fund and index price charts, ported from the stock
 * tracker's ChartView: a log axis when the window spans 3x or more (Auto),
 * with 1-2-5 style ticks, or "nice" round ticks on a linear axis.
 */
object ChartScale {
    /** Auto switches to a log axis when the window's highest value is at least this many times its lowest. */
    const val AUTO_LOG_RATIO = 3.0

    /** Round tick values (1, 2, 2.5, 5 x 10^k) between [lo] and [hi], about [n] of them. */
    fun niceTicks(lo: Double, hi: Double, n: Int): List<Double> {
        if (!(hi > lo)) return emptyList()
        val raw = (hi - lo) / n
        val mag = 10.0.pow(floor(log10(raw)))
        val f = raw / mag
        val step = mag * when { f <= 1.0 -> 1.0; f <= 2.0 -> 2.0; f <= 2.5 -> 2.5; f <= 5.0 -> 5.0; else -> 10.0 }
        val out = ArrayList<Double>()
        var v = ceil(lo / step) * step
        while (v <= hi + step * 1e-9) { out.add(v); v += step }
        return out
    }

    /**
     * Ticks at 1, 2, 5 x 10^k for a log axis, falling back to finer steps when
     * the window is narrow. [yOf] maps a value to its screen y so the labels
     * can be kept at least [minGapPx] apart.
     */
    fun logTicks(lo: Double, hi: Double, minGapPx: Float, yOf: (Double) -> Float): List<Double> {
        fun spaced(t: List<Double>) = t.size >= 2 && t.zipWithNext { a, b -> abs(yOf(a) - yOf(b)) }.all { it >= minGapPx }
        fun build(mult: DoubleArray): List<Double> {
            val out = ArrayList<Double>()
            var e = floor(log10(lo)).toInt()
            while (10.0.pow(e.toDouble()) <= hi) {
                for (m in mult) { val v = m * 10.0.pow(e.toDouble()); if (v >= lo && v <= hi) out.add(v) }
                e++
            }
            return out
        }
        val sets = listOf(
            doubleArrayOf(1.0, 1.5, 2.0, 2.5, 3.0, 4.0, 5.0, 6.0, 8.0),
            doubleArrayOf(1.0, 2.0, 3.0, 5.0, 7.0),
            doubleArrayOf(1.0, 2.0, 5.0),
            doubleArrayOf(1.0)
        )
        for (m in sets) { val t = build(m); if (spaced(t)) return t }
        val t = build(doubleArrayOf(1.0, 2.0, 5.0)).filterIndexed { i, _ -> i % 2 == 0 }
        return if (t.size >= 2) t else build(doubleArrayOf(1.0, 2.0, 5.0))
    }

    /** Decimal places that keep neighbouring ticks distinguishable. */
    fun decimalsFor(step: Double): Int = if (step >= 1.0) 0 else ceil(-log10(step)).toInt().coerceIn(0, 4)

    fun mean(values: DoubleArray, from: Int, to: Int): Double {
        var s = 0.0
        for (i in from..to) s += values[i]
        return s / (to - from + 1)
    }

    fun median(values: DoubleArray, from: Int, to: Int): Double {
        val sorted = values.copyOfRange(from, to + 1).also { it.sort() }
        return if (sorted.size % 2 == 1) sorted[sorted.size / 2] else (sorted[sorted.size / 2 - 1] + sorted[sorted.size / 2]) / 2
    }
}
