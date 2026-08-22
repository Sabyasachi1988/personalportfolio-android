package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

// Only the fields the cap-composition screen actually needs, from
// internal/store/portfolio.go's Asset and CapComposition structs.

data class AssetSummary(
    @SerializedName("ID") val id: String,
    @SerializedName("Name") val name: String,
    @SerializedName("ISIN") val isin: String
)

data class CapCompositionEntry(
    @SerializedName("AssetID") val assetId: String,
    @SerializedName("Large") val large: Double,
    @SerializedName("Mid") val mid: Double,
    @SerializedName("Small") val small: Double,
    @SerializedName("Cash") val cash: Double,
    @SerializedName("AsOf") val asOf: String,
    @SerializedName("Source") val source: String
)

// Just enough of the full Portfolio JSON to read Assets and
// CapCompositions - Gson ignores the other fields (Members, Accounts,
// Transactions, Prices) it doesn't need for this screen.
data class PortfolioAssetsSnapshot(
    @SerializedName("Assets") val assets: List<AssetSummary>?,
    @SerializedName("CapCompositions") val capCompositions: List<CapCompositionEntry>?
)
