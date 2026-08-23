package com.saby.personalportfolio

import java.util.Locale

/** Result of converting an INR amount for display: the converted amount (null if the needed FX rate isn't cached yet for this point) and which currency code it's actually in. */
data class ConvertedAmount(val amount: Double?, val currencyCode: String)

object ProgressionCurrency {

    /** See DisplayCurrency.NATIVE's doc comment for why this mapping was chosen. */
    fun nativeCurrencyFor(axis: ProgressionAxis): String = when (axis) {
        ProgressionAxis.INTERNATIONAL_EQUITY -> "CAD"
        else -> "INR"
    }

    /**
     * Converts an INR amount (every ProgressionPoint field is INR - see
     * ProgressionPoint's doc comment) to the requested display currency,
     * using that SPECIFIC point's own INRPerCAD rate, never today's rate -
     * consistent with how the Go side computed the series in the first
     * place. Returns a null amount (not a guess) when CAD is needed but
     * this point predates the FX history that's been fetched.
     */
    fun convert(amountINR: Double, display: DisplayCurrency, axis: ProgressionAxis, point: ProgressionPoint): ConvertedAmount {
        val target = if (display == DisplayCurrency.NATIVE) nativeCurrencyFor(axis) else display.name
        if (target == "INR") return ConvertedAmount(amountINR, "INR")
        if (!point.hasINRPerCAD || point.inrPerCAD <= 0.0) return ConvertedAmount(null, "CAD")
        return ConvertedAmount(amountINR / point.inrPerCAD, "CAD")
    }

    fun format(converted: ConvertedAmount): String {
        val amount = converted.amount ?: return "— (no FX rate for this date yet)"
        return when (converted.currencyCode) {
            "CAD" -> "C$" + String.format(Locale.US, "%,.2f", amount)
            else -> IndianCurrencyFormatter.format(amount)
        }
    }

    fun formatSigned(converted: ConvertedAmount): String {
        val amount = converted.amount ?: return "— (no FX rate for this date yet)"
        val sign = if (amount >= 0) "+" else ""
        return when (converted.currencyCode) {
            "CAD" -> sign + "C$" + String.format(Locale.US, "%,.2f", amount)
            else -> IndianCurrencyFormatter.formatSigned(amount)
        }
    }
}
