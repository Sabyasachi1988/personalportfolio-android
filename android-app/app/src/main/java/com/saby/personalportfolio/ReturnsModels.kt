package com.saby.personalportfolio

import com.google.gson.annotations.SerializedName

/** Mirrors store.Benchmark - see its Go doc comment. */
data class Benchmark(
    @SerializedName("ID") val id: String,
    @SerializedName("Name") val name: String,
    @SerializedName("YahooTicker") val yahooTicker: String,
    @SerializedName("NiftyTRIIndexName") val niftyTRIIndexName: String = "",
    @SerializedName("Nickname") val nickname: String = ""
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
    @SerializedName("IsAdditional") val isAdditional: Boolean = false,
    @SerializedName("Day") val day: TrailingReturn,
    @SerializedName("Month") val month: TrailingReturn,
    @SerializedName("OneYearTrailing") val oneYearTrailing: TrailingReturn,
    @SerializedName("OneYearRolling") val oneYearRolling: RollingReturnStats,
    @SerializedName("ThreeYearTrailing") val threeYearTrailing: TrailingReturn,
    @SerializedName("ThreeYearRolling") val threeYearRolling: RollingReturnStats,
    @SerializedName("FiveYearTrailing") val fiveYearTrailing: TrailingReturn,
    @SerializedName("FiveYearRolling") val fiveYearRolling: RollingReturnStats,
    @SerializedName("TenYearTrailing") val tenYearTrailing: TrailingReturn,
    @SerializedName("TenYearRolling") val tenYearRolling: RollingReturnStats
)

/**
 * A narrow slice of the full portfolio JSON - just enough to populate
 * the Manage Benchmarks screen (which benchmarks exist, and whether
 * each already has price history) without deserializing everything
 * else. Gson ignores JSON fields with no matching property, so reading
 * the full portfolio JSON through this class is safe and simple.
 */
/** Assets+Prices from the raw portfolio JSON, reusing AssetSummary (see CapCompositionModels.kt) - only what the Additional Funds screen needs. */
data class AdditionalFundsSnapshot(
    @SerializedName("Assets") val assets: List<AssetSummary>?,
    @SerializedName("Prices") val prices: List<PricePoint>?
)

/** Mirrors bridge.ResolveFundNameByISIN's success shape ({"name":"..."}). */
data class IsinNameResolution(
    @SerializedName("name") val name: String
)

/** Mirrors priceapi.MfapiSchemeMatch - one fund-name search hit when adding an Additional Fund by name. */
data class MfapiSchemeMatch(
    @SerializedName("Name") val name: String,
    @SerializedName("ISIN") val isin: String
)

/** Partial deserialization of the raw portfolio JSON's Benchmarks+Prices - only the fields the Benchmarks screen actually needs. */
data class PortfolioBenchmarksSnapshot(
    @SerializedName("Benchmarks") val benchmarks: List<Benchmark>?,
    @SerializedName("Prices") val prices: List<PricePoint>?
)

/** Mirrors bridge.FundMetricsResult - Beta/Info Ratio/Capture/Sharpe/Sortino/Max Drawdown plus which benchmark was used. */
data class FundMetricsResult(
    @SerializedName("Beta") val beta: Double,
    @SerializedName("BetaHasData") val betaHasData: Boolean,
    @SerializedName("InformationRatio") val informationRatio: Double,
    @SerializedName("InfoRatioHasData") val infoRatioHasData: Boolean,
    @SerializedName("UpCapture") val upCapture: Double,
    @SerializedName("UpCaptureHasData") val upCaptureHasData: Boolean,
    @SerializedName("DownCapture") val downCapture: Double,
    @SerializedName("DownCaptureHasData") val downCaptureHasData: Boolean,
    @SerializedName("MaxDrawdown") val maxDrawdown: Double,
    @SerializedName("MaxDrawdownHasData") val maxDrawdownHasData: Boolean,
    @SerializedName("SharpeRatio") val sharpeRatio: Double = 0.0,
    @SerializedName("SharpeHasData") val sharpeHasData: Boolean = false,
    @SerializedName("SortinoRatio") val sortinoRatio: Double = 0.0,
    @SerializedName("SortinoHasData") val sortinoHasData: Boolean = false,
    @SerializedName("StandardDeviation") val standardDeviation: Double = 0.0,
    @SerializedName("StdDevHasData") val stdDevHasData: Boolean = false,
    @SerializedName("Alpha") val alpha: Double = 0.0,
    @SerializedName("AlphaHasData") val alphaHasData: Boolean = false,
    @SerializedName("BenchmarkID") val benchmarkId: String?,
    @SerializedName("BenchmarkName") val benchmarkName: String?,
    @SerializedName("AutoSelected") val autoSelected: Boolean
)

/** Mirrors bridge.CustomPeriodReturnResult - a person-typed tenure's trailing + rolling figures. */
data class CustomPeriodReturnResult(
    @SerializedName("Trailing") val trailing: TrailingReturn,
    @SerializedName("Rolling") val rolling: RollingReturnStats
)

/** Mirrors bridge.MultiSeriesHistoryItem - one series' identity plus its raw price points, for the overlay comparison chart. */
data class MultiSeriesHistoryItem(
    @SerializedName("SeriesID") val seriesId: String,
    @SerializedName("Name") val name: String,
    @SerializedName("IsBenchmark") val isBenchmark: Boolean,
    @SerializedName("Points") val points: List<PricePoint>?
)

/** Mirrors bridge.TransactionMarker - one buy/sell point overlaid on a fund's price history chart. */
data class TransactionMarker(
    @SerializedName("Date") val date: String,
    @SerializedName("IsBuy") val isBuy: Boolean,
    @SerializedName("Units") val units: Double,
    @SerializedName("Price") val price: Double,
    @SerializedName("Amount") val amount: Double,
    @SerializedName("Description") val description: String = "",
    @SerializedName("Member") val member: String = ""
)
