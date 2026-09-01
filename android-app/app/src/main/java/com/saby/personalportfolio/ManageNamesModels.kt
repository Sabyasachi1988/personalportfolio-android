package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/** Mirrors bridge.NameListEntry - one asset or benchmark's real name and current nickname, for the Manage Names screen. */
data class NameListEntry(
    @SerializedName("SeriesID") val seriesId: String,
    @SerializedName("Name") val name: String,
    @SerializedName("Nickname") val nickname: String = "",
    @SerializedName("IsBenchmark") val isBenchmark: Boolean = false,
    @SerializedName("UsableAsBenchmark") val usableAsBenchmark: Boolean = false,
    @SerializedName("ISIN") val isin: String = ""
)
