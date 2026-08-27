package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/**
 * Mirrors bridge.DashboardResult field-for-field - see that Go struct's
 * doc comment for why it exists (one combined bridge call replacing 7
 * separate ones that each independently re-parsed the same portfolio
 * JSON, the confirmed remaining cause of Dashboard-specific lag).
 */
/**
 * Mirrors bridge.DashboardHolding - a MINIMAL 3-field stand-in for the
 * full Holding class, matching what bridge.ComputeDashboard actually
 * sends (see that Go struct's doc comment for why: gomobile bind
 * couldn't generate a binding for a slice-of-struct field whose element
 * type itself had a nested slice field, which the full finance.Holding/
 * Kotlin Holding does via Tags). Deliberately NOT reusing the full
 * Holding class here even though Kotlin would compile fine either way -
 * Gson's unsafe field allocation leaves any JSON-absent, non-default
 * Kotlin field silently null despite its declared non-null type (see
 * store.Asset's GroupLabel/ETMoneyURL doc comment for the exact
 * confirmed-crash precedent this app already learned from once), so
 * reusing a 15-field class for a 3-field payload would leave 12 fields
 * primed to NPE the moment anything touches them, however unlikely
 * today - a dedicated minimal class makes that mistake impossible
 * rather than merely unlikely.
 */
data class DashboardHolding(
    @SerializedName("HasPrice") val hasPrice: Boolean = false,
    @SerializedName("NetInvested") val netInvested: Double = 0.0,
    @SerializedName("CurrentValue") val currentValue: Double = 0.0
)

/**
 * Mirrors bridge.DashboardResult field-for-field - see that Go struct's
 * doc comment for why it exists (one combined bridge call replacing 7
 * separate ones that each independently re-parsed the same portfolio
 * JSON, the confirmed remaining cause of Dashboard-specific lag).
 */
data class DashboardResult(
    @SerializedName("Holdings") val holdings: List<DashboardHolding>?,
    @SerializedName("xirr") val xirr: Double,
    @SerializedName("hasXIRR") val hasXIRR: Boolean,
    @SerializedName("RollingGains") val rollingGains: List<PeriodGain>?,
    @SerializedName("CalendarYearGain") val calendarYearGain: PeriodGain?,
    @SerializedName("MarketCapSlices") val marketCapSlices: List<AllocationSlice>?,
    @SerializedName("OriginSlices") val originSlices: List<AllocationSlice>?,
    @SerializedName("ClassSlices") val classSlices: List<AllocationSlice>?
)
