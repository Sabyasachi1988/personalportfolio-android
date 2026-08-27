package com.saby.personalportfolio

import java.text.SimpleDateFormat
import java.util.Locale

/**
 * Point-to-point annualized return (CAGR) between two prices on two
 * dates - used by both the single-fund Returns chart and the Compare
 * overlay chart to answer "from wherever this chart starts to where I'm
 * hovering, what's the annualized return". Always annualizes regardless
 * of span (Saby's explicit choice, even for sub-year windows, where an
 * annualized figure extrapolates a short period and can look extreme -
 * that's expected here, not a bug, since it was asked for deliberately
 * rather than following ComputeTrailingReturn's own <1Y-stays-simple
 * convention).
 */
object CagrCalculator {
    private val storedDateFormat = SimpleDateFormat("yyyy-MM-dd", Locale.US)

    /** Annualized % return from (startPrice, startDate) to (endPrice, endDate), or null if undefined (non-positive price, unparseable date, or zero/negative elapsed days). */
    fun compute(startPrice: Double, startDate: String, endPrice: Double, endDate: String): Double? {
        if (startPrice <= 0.0 || endPrice <= 0.0) return null
        val days = daysBetween(startDate, endDate) ?: return null
        if (days <= 0) return null
        val years = days / 365.0
        return (Math.pow(endPrice / startPrice, 1.0 / years) - 1.0) * 100.0
    }

    private fun daysBetween(startDate: String, endDate: String): Long? {
        return try {
            val d1 = storedDateFormat.parse(startDate) ?: return null
            val d2 = storedDateFormat.parse(endDate) ?: return null
            (d2.time - d1.time) / (1000L * 60 * 60 * 24)
        } catch (e: Exception) {
            null
        }
    }
}
