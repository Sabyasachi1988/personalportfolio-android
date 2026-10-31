package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/** Only what the UI needs of store.FundHistory: sync notes and how many months are held. */
data class FundHistorySummary(
    @SerializedName("AssetID") val assetId: String = "",
    @SerializedName("Pid") val pid: String = "",
    @SerializedName("Note") val note: String? = "",
    @SerializedName("NavNote") val navNote: String? = "",
    @SerializedName("Months") val months: List<FundMonthSummary>? = null
)

data class FundMonthSummary(@SerializedName("Month") val month: String = "")

/** Mirrors finance.CompositionPoint. All bucket fields are percent of the portfolio's value that month. */
data class CompositionPoint(
    @SerializedName("Month") val month: String = "",
    @SerializedName("Date") val date: String = "",
    @SerializedName("Value") val value: Double = 0.0,
    @SerializedName("Large") val large: Double = 0.0,
    @SerializedName("Mid") val mid: Double = 0.0,
    @SerializedName("Small") val small: Double = 0.0,
    @SerializedName("Cash") val cash: Double = 0.0,
    @SerializedName("Debt") val debt: Double = 0.0,
    @SerializedName("Foreign") val foreign: Double = 0.0,
    @SerializedName("Other") val other: Double = 0.0,
    @SerializedName("NoData") val noData: Double = 0.0,
    @SerializedName("Estimated") val estimated: Double = 0.0,
    @SerializedName("Sectors") val sectors: Map<String, Double>? = null,
    @SerializedName("OtherBy") val otherBy: Map<String, Double>? = null,
    @SerializedName("NoDataBy") val noDataBy: Map<String, Double>? = null
)
