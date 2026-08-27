package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

// Only the fields the cap-composition screen actually needs, from
// internal/store/portfolio.go's Asset and CapComposition structs.

data class AssetSummary(
    @SerializedName("ID") val id: String,
    @SerializedName("AccountID") val accountId: String = "",
    @SerializedName("Name") val name: String,
    @SerializedName("ISIN") val isin: String,
    @SerializedName("Symbol") val symbol: String = "",
    @SerializedName("Type") val type: String = "",
    @SerializedName("ETMoneyURL") val etMoneyUrl: String = "",
    @SerializedName("GroupLabel") val groupLabel: String = "",
    // Tags/PrimaryTag are always present (never a missing/null key) on
    // the Go side by the time this reaches Kotlin - see
    // store.Asset.Tags' doc comment and Load()'s normalization. The `=
    // emptyList()` default here is a safety net for older cached JSON
    // only, not something this screen relies on for correctness.
    @SerializedName("Tags") val tags: List<String> = emptyList(),
    @SerializedName("PrimaryTag") val primaryTag: String = "",
    @SerializedName("Nickname") val nickname: String = "",
    @SerializedName("AssetClass") val assetClass: String = "",
    @SerializedName("AssetClassOverride") val assetClassOverride: String = ""
)

data class AccountSummary(
    @SerializedName("ID") val id: String,
    @SerializedName("MemberID") val memberId: String,
    @SerializedName("Name") val name: String,
    @SerializedName("Currency") val currency: String
)

// Just enough of the full Portfolio JSON for the manual (non-CAS)
// holdings entry screen: members (to attach a new account to), accounts
// (to attach a new asset to, and to pick from for a new transaction),
// and assets (to pick from for a new transaction).
data class PortfolioManualEntrySnapshot(
    @SerializedName("Members") val members: List<Member>?,
    @SerializedName("Accounts") val accounts: List<AccountSummary>?,
    @SerializedName("Assets") val assets: List<AssetSummary>?
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

// Mirrors priceapi.CapCompositionResult (Go) - the raw fetched
// percentages from an ETMoney fund page, before the person reviews and
// saves them (see FetchCapCompositionFromETMoney's doc comment: this is
// a best-effort parse, not guaranteed correct).
data class ETMoneyFetchResult(
    @SerializedName("Large") val large: Double,
    @SerializedName("Mid") val mid: Double,
    @SerializedName("Small") val small: Double,
    @SerializedName("Cash") val cash: Double,
    @SerializedName("MatchedSum") val matchedSum: Double
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

data class EquityOriginEntry(
    @SerializedName("AssetID") val assetId: String,
    @SerializedName("Indian") val indian: Double,
    @SerializedName("International") val international: Double,
    @SerializedName("AsOf") val asOf: String,
    @SerializedName("Source") val source: String
)

// Just enough of the full Portfolio JSON for the equity-origin entry
// screen: the asset list plus any existing entries.
data class PortfolioEquityOriginSnapshot(
    @SerializedName("Assets") val assets: List<AssetSummary>?,
    @SerializedName("EquityOriginCompositions") val equityOriginCompositions: List<EquityOriginEntry>?
)
