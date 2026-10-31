package com.saby.personalportfolio

import android.content.Context
import com.google.gson.Gson
import com.google.gson.annotations.SerializedName
import java.io.File

/** One row of the downloaded all-funds table (built monthly by the fund-data repo; see build/screener.py there). */
data class ScreenerRow(
    @SerializedName("c") val code: Int = 0,
    @SerializedName("isin") val isin: String = "",
    @SerializedName("name") val name: String = "",
    @SerializedName("cat") val cat: String = "",
    @SerializedName("amc") val amc: String = "",
    @SerializedName("pid") val pid: String = "",
    @SerializedName("nav") val nav: Double = 0.0,
    @SerializedName("d") val date: String = "",
    @SerializedName("age") val age: Double = 0.0,
    @SerializedName("r") val r: Map<String, Double?>? = null,
    @SerializedName("roll") val roll: Map<String, List<Double>?>? = null,
    @SerializedName("vol") val vol: Double? = null,
    @SerializedName("mdd") val mdd: Double? = null,
    @SerializedName("sharpe") val sharpe: Double? = null,
    @SerializedName("cap") val cap: List<Double>? = null
)

data class ScreenerBundle(
    @SerializedName("asof") val asof: String = "",
    @SerializedName("rf") val rf: Double = 0.065,
    @SerializedName("rows") val rows: List<ScreenerRow>? = null
)

/** A row plus the display fields worked out once at load. */
class ScreenerFund(val row: ScreenerRow, val category: String, val shortName: String) {
    fun ret(key: String): Double? = row.r?.get(key)
    val large: Double? get() = row.cap?.getOrNull(0)
    val mid: Double? get() = row.cap?.getOrNull(1)
    val small: Double? get() = row.cap?.getOrNull(2)
}

object ScreenerStore {
    private val gson = Gson()
    @Volatile private var cache: List<ScreenerFund>? = null
    @Volatile private var cacheKey = ""
    @Volatile var asof: String = ""
        private set

    fun dir(context: Context): File = File(context.applicationContext.filesDir, "screener")
    fun isDownloaded(context: Context) = File(dir(context), "screener.json").exists()

    /** AMFI category names come in several spellings for the same thing; fold them to one set. */
    fun normCategory(raw: String): String {
        val s = raw.trim()
        val l = s.lowercase()
        return when {
            s.isEmpty() -> "Other"
            l.startsWith("index funds") -> "Index Funds"
            l.startsWith("fund of funds scheme (domestic)") || l.startsWith("fof domestic") -> "FoF Domestic"
            l.startsWith("fof overseas") || l.startsWith("fund of funds scheme (overseas)") -> "FoF Overseas"
            l.contains("sectoral") || l.contains("thematic") -> "Sectoral / Thematic"
            else -> s.replace(Regex("^(Income/Debt Oriented Schemes|Equity Schemes?|Debt Schemes?|Hybrid Schemes?|Solution Oriented Schemes?|Other Schemes?)\\s*-\\s*"), "")
                .replace(Regex("^Hybrid\\s*-\\s*"), "").trim().ifEmpty { "Other" }
        }
    }

    fun shortName(name: String): String = name
        .replace(Regex("\\s*-\\s*Direct Plan\\s*-?\\s*", RegexOption.IGNORE_CASE), " ")
        .replace(Regex("\\bDirect( Plan)?\\b", RegexOption.IGNORE_CASE), "")
        .replace(Regex("\\b(Growth|Option)\\b", RegexOption.IGNORE_CASE), "")
        .replace(Regex("\\s+-\\s*$"), "")
        .replace(Regex("\\s{2,}"), " ").trim().trimEnd('-').trim()

    @Synchronized
    fun load(context: Context): List<ScreenerFund> {
        val f = File(dir(context), "screener.json")
        if (!f.exists()) return emptyList()
        val key = "${f.lastModified()}:${f.length()}"
        cache?.let { if (key == cacheKey) return it }
        val bundle = try { f.reader().use { gson.fromJson(it, ScreenerBundle::class.java) } } catch (e: Exception) { null }
        val list = bundle?.rows.orEmpty().map { ScreenerFund(it, normCategory(it.cat), shortName(it.name)) }
        cache = list; cacheKey = key; asof = bundle?.asof.orEmpty()
        return list
    }
}
