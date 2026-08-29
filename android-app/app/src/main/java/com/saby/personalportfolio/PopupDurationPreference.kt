package com.saby.personalportfolio

import android.content.Context

/**
 * How long the self-dismissing info popup (transaction markers on a
 * chart, Dashboard period-gain chips - see AutoDismissPopupView's own
 * doc comment) stays up before disappearing on its own. Previously
 * hardcoded to 800ms - a confirmed real ask to make this
 * person-adjustable instead. Same SharedPreferences pattern as
 * ThemePreference/LockPreference.
 *
 * Stored in milliseconds internally (what AutoDismissPopupView actually
 * needs), but the Settings UI presents it in seconds (including
 * fractional values like 1.5s) since that's a far more natural unit for
 * a person choosing "how long should this stay visible" than raw
 * milliseconds.
 */
object PopupDurationPreference {
    private const val PREFS_NAME = "popup_duration_prefs"
    private const val KEY_DURATION_MS = "duration_ms"

    // Options shown in Settings, in SECONDS - deliberately including
    // fractional values (0.5, 1.5) since a duration this short is
    // exactly where the difference between e.g. 1s and 1.5s is
    // meaningfully noticeable to someone reading a short popup.
    // AutoDismissPopupView.LINGER_DURATION_MS's own previous hardcoded
    // value (800ms = 0.8s) is kept as the default so nobody's existing
    // experience changes until they deliberately pick something else.
    val DURATION_OPTIONS_SECONDS = listOf(0.5, 0.8, 1.0, 1.5, 2.0, 3.0, 5.0)
    val DURATION_LABELS = listOf("0.5s", "0.8s (default)", "1s", "1.5s", "2s", "3s", "5s")

    fun durationMs(context: Context): Long =
        prefs(context).getLong(KEY_DURATION_MS, (0.8 * 1000).toLong())

    fun setDurationSeconds(context: Context, seconds: Double) {
        prefs(context).edit().putLong(KEY_DURATION_MS, (seconds * 1000).toLong()).apply()
    }

    private fun prefs(context: Context) =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
}
