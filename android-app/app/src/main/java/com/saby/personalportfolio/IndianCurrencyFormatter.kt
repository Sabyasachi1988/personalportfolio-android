package com.saby.personalportfolio

import java.text.NumberFormat
import java.util.Locale

/**
 * Formats rupee amounts with Indian digit grouping (lakh/crore: e.g.
 * ₹89,10,686.85) rather than Western grouping (₹8,910,686.85). Verified
 * against CLDR locale data before use - Locale("en", "IN") applies the
 * correct 2-2-3 grouping pattern natively, so this is a thin wrapper
 * around Java's own NumberFormat rather than hand-rolled digit-grouping
 * logic.
 */
object IndianCurrencyFormatter {
    private val indiaLocale = Locale("en", "IN")

    fun format(amount: Double, decimals: Int = 2): String {
        val nf = NumberFormat.getNumberInstance(indiaLocale)
        nf.minimumFractionDigits = decimals
        nf.maximumFractionDigits = decimals
        return "₹" + nf.format(amount)
    }

    /** Same as [format] but prefixes a "+" for non-negative amounts - for gain/loss lines. */
    fun formatSigned(amount: Double, decimals: Int = 2): String {
        val sign = if (amount >= 0) "+" else ""
        return sign + format(amount, decimals)
    }
}
