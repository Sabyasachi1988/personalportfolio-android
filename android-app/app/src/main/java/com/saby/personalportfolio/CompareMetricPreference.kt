package com.saby.personalportfolio

import android.content.Context

/**
 * Which rows show in the Compare screen's quantitative table - a
 * confirmed real ask: the person wants to pick their own subset from
 * the FULL available set (including the rolling min-max range, not
 * shown by default today) and have that choice persist across app
 * restarts rather than resetting to the built-in default every time.
 * Same SharedPreferences pattern as PopupDurationPreference/
 * ThemePreference/LockPreference.
 *
 * Stores ONE combined set of metric IDs spanning both the Returns and
 * Risk Parameters sections - CompareMetricCatalog's own id->label/
 * section mapping is what splits them back apart for rendering.
 */
object CompareMetricPreference {
    private const val PREFS_NAME = "compare_metric_prefs"
    private const val KEY_SELECTED_IDS = "selected_ids"
    private const val DELIMITER = ","

    /**
     * The built-in default - exactly what the table showed before this
     * customization feature existed, so nobody's existing view changes
     * until they deliberately customize it. Deliberately excludes the
     * rolling min-max range rows (CompareMetricCatalog.ALL has them
     * too, as a pick-able option, just not selected by default).
     */
    val DEFAULT_SELECTED_IDS: Set<String> = setOf(
        "day", "month", "y1_trailing", "y3_trailing", "y5_trailing", "y10_trailing",
        "y1_rolling", "y3_rolling", "y5_rolling", "y10_rolling",
        "beta", "alpha", "info_ratio", "std_dev", "up_capture", "down_capture", "max_drawdown", "sharpe", "sortino"
    )

    fun getSelectedIds(context: Context): Set<String> {
        val stored = prefs(context).getString(KEY_SELECTED_IDS, null) ?: return DEFAULT_SELECTED_IDS
        val ids = stored.split(DELIMITER).filter { it.isNotBlank() }.toSet()
        return ids.ifEmpty { DEFAULT_SELECTED_IDS } // never persist/return an empty table - degrade to default rather than show nothing
    }

    fun setSelectedIds(context: Context, ids: Set<String>) {
        prefs(context).edit().putString(KEY_SELECTED_IDS, ids.joinToString(DELIMITER)).apply()
    }

    private fun prefs(context: Context) =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
}

/**
 * The full catalog of every metric the Compare table CAN show - the
 * "whole set of data we have for comparison purposes" the person can
 * pick from, not just what's currently visible. Order here is the
 * canonical display order within each section; picking a subset never
 * reorders the remaining rows.
 */
object CompareMetricCatalog {
    data class Metric(val id: String, val label: String, val section: Section)
    enum class Section { RETURNS, RISK }

    val ALL: List<Metric> = listOf(
        Metric("day", "Day", Section.RETURNS),
        Metric("month", "1 Month", Section.RETURNS),
        Metric("y1_trailing", "1Y Trailing", Section.RETURNS),
        Metric("y3_trailing", "3Y Trailing", Section.RETURNS),
        Metric("y5_trailing", "5Y Trailing", Section.RETURNS),
        Metric("y10_trailing", "10Y Trailing", Section.RETURNS),
        Metric("y1_rolling", "1Y Rolling", Section.RETURNS),
        Metric("y3_rolling", "3Y Rolling", Section.RETURNS),
        Metric("y5_rolling", "5Y Rolling", Section.RETURNS),
        Metric("y10_rolling", "10Y Rolling", Section.RETURNS),
        Metric("y1_range", "1Y Rolling Range", Section.RETURNS),
        Metric("y3_range", "3Y Rolling Range", Section.RETURNS),
        Metric("y5_range", "5Y Rolling Range", Section.RETURNS),
        Metric("y10_range", "10Y Rolling Range", Section.RETURNS),
        Metric("beta", "Beta", Section.RISK),
        Metric("alpha", "Alpha", Section.RISK),
        Metric("info_ratio", "Information Ratio", Section.RISK),
        Metric("std_dev", "Std. Deviation", Section.RISK),
        Metric("up_capture", "Up Capture", Section.RISK),
        Metric("down_capture", "Down Capture", Section.RISK),
        Metric("max_drawdown", "Max Drawdown*", Section.RISK),
        Metric("sharpe", "Sharpe Ratio", Section.RISK),
        Metric("sortino", "Sortino Ratio", Section.RISK)
    )
}
