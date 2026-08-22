package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

// Mirrors internal/finance/holdings.go's Holding struct field-for-field.
data class Holding(
    @SerializedName("AssetID") val assetId: String,
    @SerializedName("AssetName") val assetName: String,
    @SerializedName("ISIN") val isin: String,
    @SerializedName("AccountName") val accountName: String,
    @SerializedName("MemberID") val memberId: String,
    @SerializedName("MemberName") val memberName: String,
    @SerializedName("UnitsHeld") val unitsHeld: Double,
    @SerializedName("NetInvested") val netInvested: Double,
    @SerializedName("CurrentPrice") val currentPrice: Double,
    @SerializedName("HasPrice") val hasPrice: Boolean,
    @SerializedName("CurrentValue") val currentValue: Double,
    @SerializedName("Gain") val gain: Double,
    @SerializedName("GainPercent") val gainPercent: Double,
    @SerializedName("XIRR") val xirr: Double,
    @SerializedName("HasXIRR") val hasXirr: Boolean
)
