package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/**
 * One weekly (or "today") checkpoint from Bridge.computeProgression - see
 * finance.ProgressionPoint's doc comment on the Go side. Invested/Value/
 * Gain are always in INR regardless of axis; INRPerCAD is that exact
 * date's own FX rate (not today's), present only when FX history covers
 * this date (HasINRPerCAD false otherwise - see UpdateHistoryActivity).
 */
data class ProgressionPoint(
    @SerializedName("Date") val date: String,
    @SerializedName("Invested") val invested: Double,
    @SerializedName("Value") val value: Double,
    @SerializedName("Gain") val gain: Double,
    @SerializedName("GainPercent") val gainPercent: Double,
    @SerializedName("XIRR") val xirr: Double,
    @SerializedName("HasXIRR") val hasXIRR: Boolean,
    @SerializedName("INRPerCAD") val inrPerCAD: Double,
    @SerializedName("HasINRPerCAD") val hasINRPerCAD: Boolean
)

/**
 * The four axes ComputeProgression accepts, matching
 * finance.ProgressionAxis's Go string constants exactly (these are
 * passed straight through to the bridge call, not just display labels).
 */
enum class ProgressionAxis(val bridgeValue: String, val label: String) {
    WHOLE_PORTFOLIO("WholePortfolio", "Whole Portfolio"),
    INDIAN_EQUITY("IndianEquity", "Indian Equity"),
    INTERNATIONAL_EQUITY("InternationalEquity", "International Equity"),
    COMBINED_EQUITY("CombinedEquity", "Combined Equity")
}

/**
 * Display-currency choice for the progression screen. NATIVE follows the
 * convention from the project's Phase 3/4 design notes - "home currency"
 * meaning whichever currency is the natural one for the selected axis:
 * INR for Indian Equity, CAD for International Equity, and INR (the
 * portfolio's own computation base - see ProgressionPoint's doc comment)
 * for Whole Portfolio / Combined Equity, since those mix both and have
 * no single natural currency of their own. This is an interpretation of
 * an ambiguous spec, not a confirmed-with-Saby rule - flagged here so
 * it's easy to find and revisit if that reading turns out wrong.
 */
enum class DisplayCurrency(val label: String) {
    INR("₹ INR"),
    CAD("$ CAD"),
    NATIVE("Native (by axis)")
}
