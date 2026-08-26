package com.saby.personalportfolio

import java.text.NumberFormat
import java.util.Locale

/**
 * Formats rupee amounts with Indian digit grouping (lakh/crore: e.g.
 * ₹89,10,686) rather than Western grouping (₹8,910,686). Verified
 * against CLDR locale data before use - Locale("en", "IN") applies the
 * correct 2-2-3 grouping pattern natively, so this is a thin wrapper
 * around Java's own NumberFormat rather than hand-rolled digit-grouping
 * logic.
 *
 * Defaults to 0 decimal places - every call site in this app uses this
 * for a TOTAL or AMOUNT (portfolio value, gain, invested, a transaction
 * amount), never a per-unit price/NAV, which needs its own real
 * precision and is never routed through this formatter. Paisa-level
 * precision on a summary figure like "portfolio value" or "today's
 * gain" is noise, not information - nobody is making a decision off 50
 * paise of movement on a multi-lakh figure. Pass decimals explicitly
 * for the rare case that genuinely needs it.
 */
object IndianCurrencyFormatter {
    private val indiaLocale = Locale("en", "IN")

    // Fixed-length, not proportional to the real amount's digit count -
    // a mask whose length varied with the actual number (e.g. via
    // "•".repeat(digits)) would itself leak the portfolio's order of
    // magnitude (lakhs vs crores) even with every digit hidden. See
    // IncognitoMode's doc comment for why this lives here rather than
    // at each of the 15+ call sites.
    private const val MASK = "₹••••••"

    fun format(amount: Double, decimals: Int = 0): String {
        if (IncognitoMode.isEnabled) return MASK
        val nf = NumberFormat.getNumberInstance(indiaLocale)
        nf.minimumFractionDigits = decimals
        nf.maximumFractionDigits = decimals
        return "₹" + nf.format(amount)
    }

    /** Same as [format] but prefixes a "+" for non-negative amounts - for gain/loss lines. */
    fun formatSigned(amount: Double, decimals: Int = 0): String {
        if (IncognitoMode.isEnabled) return (if (amount >= 0) "+" else "-") + MASK
        val sign = if (amount >= 0) "+" else ""
        return sign + format(amount, decimals)
    }
}

/**
 * Formats a per-unit price - a fund's NAV or a benchmark index's level -
 * with the same Indian digit grouping as [IndianCurrencyFormatter], but
 * DELIBERATELY NEVER masked by incognito mode, and deliberately a
 * separate object rather than a call site of IndianCurrencyFormatter
 * (whose own doc comment already says per-unit price/NAV should never
 * be routed through it, for exactly this reason).
 *
 * A NAV or an index level is publicly available information anyone can
 * look up - it's not a fact about Saby's own holdings the way a rupee
 * total or a unit count is (see IncognitoMode/HoldingsAdapter.
 * unitsDisplay's doc comments on why THOSE are masked: units x a public
 * price reveals a private total). A fund's price by itself reveals
 * nothing about what Saby owns, so masking it here was a real,
 * confirmed regression - it forced deactivating incognito just to read
 * a Returns/Compare chart, which shows no portfolio-specific figures at
 * all.
 */
object PricePerUnitFormatter {
    private val indiaLocale = Locale("en", "IN")

    fun format(price: Double, decimals: Int = 2): String {
        val nf = NumberFormat.getNumberInstance(indiaLocale)
        nf.minimumFractionDigits = decimals
        nf.maximumFractionDigits = decimals
        return "₹" + nf.format(price)
    }
}
