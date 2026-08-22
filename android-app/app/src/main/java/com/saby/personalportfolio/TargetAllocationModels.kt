package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

data class TargetAllocation(
    @SerializedName("Large") val large: Double,
    @SerializedName("Mid") val mid: Double,
    @SerializedName("Small") val small: Double,
    @SerializedName("Cash") val cash: Double
)

// Matches ComputeAllocationDrift's {"hasTarget":bool,"drift":[...]} shape.
data class AllocationDriftResult(
    val hasTarget: Boolean,
    val drift: List<AllocationDriftSlice>?
)

data class AllocationDriftSlice(
    @SerializedName("Label") val label: String,
    @SerializedName("Actual") val actual: Double,
    @SerializedName("Target") val target: Double,
    @SerializedName("Drift") val drift: Double
)

// Just enough of the full Portfolio JSON to read the current target, for
// prefilling the edit screen.
data class PortfolioTargetSnapshot(
    @SerializedName("TargetAllocation") val targetAllocation: TargetAllocation?
)
