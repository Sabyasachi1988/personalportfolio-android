package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

data class AllocationSlice(
    @SerializedName("Label") val label: String,
    @SerializedName("Value") val value: Double,
    @SerializedName("Percent") val percent: Double
)
