package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/** Mirrors bridge.UpdateAllHistoryResult - the combined, concurrent history-update result. */
data class UpdateAllHistoryResult(
    @SerializedName("NavSucceeded") val navSucceeded: Int = 0,
    @SerializedName("NavTotal") val navTotal: Int = 0,
    @SerializedName("NavFailures") val navFailures: List<String>? = null,
    @SerializedName("PriceSucceeded") val priceSucceeded: Int = 0,
    @SerializedName("PriceTotal") val priceTotal: Int = 0,
    @SerializedName("PriceFailures") val priceFailures: List<String>? = null,
    @SerializedName("FxSucceeded") val fxSucceeded: Int = 0,
    @SerializedName("FxTotal") val fxTotal: Int = 0,
    @SerializedName("FxFailures") val fxFailures: List<String>? = null,
    @SerializedName("BenchmarkSucceeded") val benchmarkSucceeded: Int = 0,
    @SerializedName("BenchmarkTotal") val benchmarkTotal: Int = 0,
    @SerializedName("BenchmarkFailures") val benchmarkFailures: List<String>? = null,
    @SerializedName("PortfolioJSON") val portfolioJson: String = ""
)
