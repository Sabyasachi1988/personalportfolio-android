package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/**
 * Mirrors bridge.DashboardResult field-for-field - see that Go struct's
 * doc comment for why it exists (one combined bridge call replacing 7
 * separate ones that each independently re-parsed the same portfolio
 * JSON, the confirmed remaining cause of Dashboard-specific lag).
 */
data class DashboardResult(
    @SerializedName("Holdings") val holdings: List<Holding>?,
    @SerializedName("xirr") val xirr: Double,
    @SerializedName("hasXIRR") val hasXIRR: Boolean,
    @SerializedName("RollingGains") val rollingGains: List<PeriodGain>?,
    @SerializedName("CalendarYearGain") val calendarYearGain: PeriodGain?,
    @SerializedName("MarketCapSlices") val marketCapSlices: List<AllocationSlice>?,
    @SerializedName("OriginSlices") val originSlices: List<AllocationSlice>?,
    @SerializedName("ClassSlices") val classSlices: List<AllocationSlice>?
)
