package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

data class AllocationSlice(
    @SerializedName("Label") val label: String,
    @SerializedName("Value") val value: Double,
    @SerializedName("Percent") val percent: Double
)

data class PortfolioClassTargetEntry(
    @SerializedName("Equity") val equity: Double,
    @SerializedName("Debt") val debt: Double,
    @SerializedName("Commodity") val commodity: Double,
    @SerializedName("Others") val others: Double
)
