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

data class StoredTransactionEntry(
    @SerializedName("ID") val id: String,
    @SerializedName("AssetID") val assetId: String,
    @SerializedName("Date") val date: String,
    @SerializedName("Type") val type: String,
    @SerializedName("Description") val description: String,
    @SerializedName("Amount") val amount: Double,
    @SerializedName("Units") val units: Double?
)

// Just enough of the full Portfolio JSON to read Assets, CapCompositions,
// and Transactions - Gson ignores the fields (Members, Accounts, Prices)
// it doesn't need for these screens.
data class PortfolioAssetsSnapshot(
    @SerializedName("Assets") val assets: List<AssetSummary>?,
    @SerializedName("CapCompositions") val capCompositions: List<CapCompositionEntry>?,
    @SerializedName("Transactions") val transactions: List<StoredTransactionEntry>?
)
