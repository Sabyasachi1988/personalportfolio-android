package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/** Minimal Asset shape for reading PreferredBenchmarkID after BenchmarkPicker persists a choice - not a full Asset model, just the two fields needed. */
data class AssetPreferredBenchmarkSnapshot(
    @SerializedName("Assets") val assets: List<AssetPreferredBenchmarkEntry>?
)

data class AssetPreferredBenchmarkEntry(
    @SerializedName("ID") val id: String,
    @SerializedName("PreferredBenchmarkID") val preferredBenchmarkId: String = ""
)

/** Mirrors bridge.NameListEntry - one asset or benchmark's real name and current nickname, for the Manage Names screen. */
data class NameListEntry(
    @SerializedName("SeriesID") val seriesId: String,
    @SerializedName("Name") val name: String,
    @SerializedName("Nickname") val nickname: String = "",
    @SerializedName("IsBenchmark") val isBenchmark: Boolean = false,
    @SerializedName("UsableAsBenchmark") val usableAsBenchmark: Boolean = false,
    @SerializedName("ISIN") val isin: String = ""
)
