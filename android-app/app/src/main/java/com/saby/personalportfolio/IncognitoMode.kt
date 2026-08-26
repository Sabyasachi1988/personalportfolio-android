package com.saby.personalportfolio

import android.content.Context

/**
 * When enabled, IndianCurrencyFormatter masks every rupee amount it
 * formats instead of showing the real number - percentages are
 * untouched (they're formatted separately, inline, at each call site,
 * never through this formatter) so relative performance stays visible
 * while absolute scale doesn't. This was the deliberately chosen
 * simplest of three options discussed (full blackout / proportional
 * rebasing / percentages-survive-rupees-don't) specifically because
 * routing every rupee display through the one shared formatter this
 * app already uses everywhere means this single toggle correctly
 * affects every screen at once, with no separate calculation path that
 * could leak a real number by being missed.
 */
object IncognitoMode {
    private const val PREFS_NAME = "incognito_prefs"
    private const val KEY_ENABLED = "enabled"

    // In-memory cache, loaded once at app startup (see loadSaved) and
    // updated on toggle - avoids threading a Context through every
    // IndianCurrencyFormatter.format() call site (15+ across the app).
    @Volatile
    var isEnabled: Boolean = false
        private set

    fun setEnabled(context: Context, enabled: Boolean) {
        isEnabled = enabled
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            .edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    /** Call once, as early as possible (Application.onCreate), before any Activity is created. */
    fun loadSaved(context: Context) {
        val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        isEnabled = prefs.getBoolean(KEY_ENABLED, false)
    }
}
