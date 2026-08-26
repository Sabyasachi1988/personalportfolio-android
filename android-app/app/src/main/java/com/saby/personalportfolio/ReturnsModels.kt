package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/** Mirrors store.Benchmark - see its Go doc comment. */
data class Benchmark(
    @SerializedName("ID") val id: String,
    @SerializedName("Name") val name: String,
    @SerializedName("YahooTicker") val yahooTicker: String
)

/** Mirrors store.PriceRecord - used for the tap-to-graph price history drill-down. */
data class PricePoint(
    @SerializedName("AssetID") val seriesId: String,
    @SerializedName("Date") val date: String,
    @SerializedName("Price") val price: Double,
    @SerializedName("Source") val source: String = ""
)

/** Mirrors finance.TrailingReturn - see its Go doc comment. */
data class TrailingReturn(
    @SerializedName("Label") val label: String,
    @SerializedName("Percent") val percent: Double,
    @SerializedName("HasData") val hasData: Boolean
)

/** Mirrors finance.RollingReturnStats - see its Go doc comment. */
data class RollingReturnStats(
    @SerializedName("Label") val label: String,
    @SerializedName("Median") val median: Double,
    @SerializedName("Min") val min: Double,
    @SerializedName("Max") val max: Double,
    @SerializedName("HasData") val hasData: Boolean
)

/** Mirrors bridge.ReturnsTableRow - one row of the Returns screen's table. */
data class ReturnsTableRow(
    @SerializedName("SeriesID") val seriesId: String,
    @SerializedName("Name") val name: String,
    @SerializedName("IsBenchmark") val isBenchmark: Boolean,
    @SerializedName("Day") val day: TrailingReturn,
    @SerializedName("Month") val month: TrailingReturn,
    @SerializedName("OneYear") val oneYear: RollingReturnStats,
    @SerializedName("ThreeYear") val threeYear: RollingReturnStats,
    @SerializedName("FiveYear") val fiveYear: RollingReturnStats,
    @SerializedName("TenYear") val tenYear: RollingReturnStats
)

/**
 * A narrow slice of the full portfolio JSON - just enough to populate
 * the Manage Benchmarks screen (which benchmarks exist, and whether
 * each already has price history) without deserializing everything
 * else. Gson ignores JSON fields with no matching property, so reading
 * the full portfolio JSON through this class is safe and simple.
 */
data class PortfolioBenchmarksSnapshot(
    @SerializedName("Benchmarks") val benchmarks: List<Benchmark>?,
    @SerializedName("Prices") val prices: List<PricePoint>?
)
