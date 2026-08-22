package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

// These mirror internal/casimport/model.go and internal/store/types.go
// field-for-field. Go's encoding/json capitalizes exported field names by
// default (no json tags were set on these structs), so the JSON keys are
// exactly the Go field names: "Txn", "Status", "Date", "Amount", etc.

data class ImportCASResult(
    val format: String?,
    val staged: List<StagedRow>?,
    val manualReview: List<ManualReviewLine>?,
    val error: String?
)

data class StagedRow(
    @SerializedName("Txn") val txn: Transaction,
    @SerializedName("Status") val status: String,
    @SerializedName("SourcePage") val sourcePage: Int,
    @SerializedName("SourceFolio") val sourceFolio: String
)

data class ManualReviewLine(
    @SerializedName("Page") val page: Int,
    @SerializedName("Folio") val folio: String,
    @SerializedName("Text") val text: String,
    @SerializedName("Reason") val reason: String
)

data class Transaction(
    @SerializedName("Date") val date: String,
    @SerializedName("Description") val description: String,
    @SerializedName("Amount") val amount: Double,
    @SerializedName("Units") val units: Double?,
    @SerializedName("Price") val price: Double?,
    @SerializedName("Balance") val balance: Double?,
    @SerializedName("Type") val type: String,
    @SerializedName("AMC") val amc: String,
    @SerializedName("Folio") val folio: String,
    @SerializedName("Scheme") val scheme: String,
    @SerializedName("ISIN") val isin: String
)
