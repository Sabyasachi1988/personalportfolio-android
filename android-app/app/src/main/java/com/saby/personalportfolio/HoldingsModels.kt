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
    @SerializedName("GroupLabel") val groupLabel: String = "",
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

// Mirrors internal/finance/holdings.go's GroupedHolding struct
// field-for-field - see GroupHoldingsByLabel's doc comment for what
// consolidation means here (a display-time aggregation only; the
// underlying assets/transactions this represents are untouched).
data class GroupedHolding(
    @SerializedName("DisplayName") val displayName: String,
    @SerializedName("IsGroup") val isGroup: Boolean,
    @SerializedName("AssetIDs") val assetIds: List<String> = emptyList(),
    @SerializedName("MemberID") val memberId: String,
    @SerializedName("MemberName") val memberName: String,
    @SerializedName("NetInvested") val netInvested: Double,
    @SerializedName("HasPrice") val hasPrice: Boolean,
    @SerializedName("CurrentValue") val currentValue: Double,
    @SerializedName("Gain") val gain: Double,
    @SerializedName("GainPercent") val gainPercent: Double,
    @SerializedName("XIRR") val xirr: Double,
    @SerializedName("HasXIRR") val hasXirr: Boolean
)
