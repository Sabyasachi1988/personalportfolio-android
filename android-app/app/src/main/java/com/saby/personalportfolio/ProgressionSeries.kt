package com.saby.personalportfolio

/** Series the Progression chart can show besides Value and Invested. Pure functions, no Android types. */
object ProgressionSeries {

    /**
     * Drawdown (percent, 0 or negative) of the contribution-neutral growth
     * index over [from]..[to] inclusive: how far below its own running peak
     * the portfolio's investments are, with deposits and withdrawals removed
     * so adding money never hides a fall and withdrawing never looks like one.
     * The peak starts at the window's first point, so it measures the window
     * on screen. Same index as WindowMath's max drawdown.
     */
    fun drawdownPct(invested: DoubleArray, value: DoubleArray, from: Int, to: Int): DoubleArray {
        val out = DoubleArray((to - from + 1).coerceAtLeast(0))
        var index = 1.0
        var peak = 1.0
        for (i in from + 1..to) {
            val prev = value[i - 1]
            if (prev > 1.0) {
                val flow = invested[i] - invested[i - 1]
                index *= (value[i] - flow) / prev
            }
            if (index > peak) peak = index
            out[i - from] = (index / peak - 1.0) * 100.0
        }
        return out
    }

    /** "₹1.2 L", "-₹45 K", "₹2.3 Cr": short rupee label for chart axes (Indian lakh/crore grouping). */
    fun compactRupees(v: Double): String {
        val a = kotlin.math.abs(v)
        val sign = if (v < 0) "-" else ""
        return when {
            a >= 1e7 -> String.format(java.util.Locale.US, "%s₹%.2f Cr", sign, a / 1e7)
            a >= 1e5 -> String.format(java.util.Locale.US, "%s₹%.2f L", sign, a / 1e5)
            a >= 1e3 -> String.format(java.util.Locale.US, "%s₹%.1f K", sign, a / 1e3)
            else -> String.format(java.util.Locale.US, "%s₹%.0f", sign, a)
        }
    }
}
