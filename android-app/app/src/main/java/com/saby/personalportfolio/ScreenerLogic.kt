package com.saby.personalportfolio

/** Screener criteria. A null field means "no limit". Drawdown/volatility limits are entered as positive numbers. */
data class ScreenerFilters(
    val min1y: Double? = null, val min3y: Double? = null, val min5y: Double? = null,
    val maxVol: Double? = null, val maxDrawdown: Double? = null, val minSharpe: Double? = null,
    val minAge: Double? = null,
    val minLarge: Double? = null, val minMid: Double? = null, val maxSmall: Double? = null,
    val onlyWithCapMix: Boolean = false
) {
    val activeCount: Int get() = listOfNotNull(min1y, min3y, min5y, maxVol, maxDrawdown, minSharpe, minAge, minLarge, minMid, maxSmall).size +
        (if (onlyWithCapMix) 1 else 0)
}

enum class ScreenerSort(val label: String) {
    NAME("Name"), R1("1Y return"), R3("3Y return"), R5("5Y return"), SHARPE("Sharpe ratio"),
    VOL("Volatility (lowest)"), DD("Max drawdown (smallest)"), AGE("Fund age (oldest)")
}

object ScreenerLogic {
    fun apply(all: List<ScreenerFund>, category: String?, query: String, f: ScreenerFilters, sort: ScreenerSort): List<ScreenerFund> {
        val words = query.lowercase().split(Regex("\\s+")).filter { it.isNotBlank() }
        val out = all.filter { fund ->
            val row = fund.row
            (category == null || fund.category == category) &&
                words.all { w -> row.name.lowercase().contains(w) || row.amc.lowercase().contains(w) } &&
                atLeast(fund.ret("1y"), f.min1y) && atLeast(fund.ret("3y"), f.min3y) && atLeast(fund.ret("5y"), f.min5y) &&
                atMost(row.vol, f.maxVol) &&
                (f.maxDrawdown == null || (row.mdd != null && -row.mdd <= f.maxDrawdown)) &&
                atLeast(row.sharpe, f.minSharpe) && (f.minAge == null || row.age >= f.minAge) &&
                atLeast(fund.large, f.minLarge) && atLeast(fund.mid, f.minMid) && atMost(fund.small, f.maxSmall) &&
                (!f.onlyWithCapMix || row.cap != null)
        }
        // funds missing the sorted metric go last
        return when (sort) {
            ScreenerSort.NAME -> out.sortedBy { it.shortName.lowercase() }
            ScreenerSort.R1 -> out.sortedWith(desc { it.ret("1y") })
            ScreenerSort.R3 -> out.sortedWith(desc { it.ret("3y") })
            ScreenerSort.R5 -> out.sortedWith(desc { it.ret("5y") })
            ScreenerSort.SHARPE -> out.sortedWith(desc { it.row.sharpe })
            ScreenerSort.VOL -> out.sortedWith(asc { it.row.vol })
            ScreenerSort.DD -> out.sortedWith(desc { it.row.mdd }) // mdd is negative: closer to 0 is smaller
            ScreenerSort.AGE -> out.sortedByDescending { it.row.age }
        }
    }

    private fun atLeast(v: Double?, min: Double?) = min == null || (v != null && v >= min)
    private fun atMost(v: Double?, max: Double?) = max == null || (v != null && v <= max)
    private fun desc(sel: (ScreenerFund) -> Double?) = Comparator<ScreenerFund> { a, b ->
        val x = sel(a); val y = sel(b)
        when { x == null && y == null -> 0; x == null -> 1; y == null -> -1; else -> y.compareTo(x) }
    }
    private fun asc(sel: (ScreenerFund) -> Double?) = Comparator<ScreenerFund> { a, b ->
        val x = sel(a); val y = sel(b)
        when { x == null && y == null -> 0; x == null -> 1; y == null -> -1; else -> x.compareTo(y) }
    }
}
