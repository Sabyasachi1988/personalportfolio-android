package com.saby.personalportfolio

import android.content.Context
import androidx.appcompat.app.AppCompatDelegate

/**
 * Explicit day/night control via AppCompatDelegate, rather than relying
 * purely on the OS's implicit -night resource qualifier resolution
 * (which in practice didn't reliably follow the system setting). This
 * also makes an in-app manual toggle possible.
 */
object ThemePreference {
    private const val PREFS_NAME = "theme_prefs"
    private const val KEY_MODE = "night_mode"

    const val MODE_SYSTEM = AppCompatDelegate.MODE_NIGHT_FOLLOW_SYSTEM
    const val MODE_LIGHT = AppCompatDelegate.MODE_NIGHT_NO
    const val MODE_DARK = AppCompatDelegate.MODE_NIGHT_YES

    fun getSavedMode(context: Context): Int {
        val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
        return prefs.getInt(KEY_MODE, MODE_SYSTEM)
    }

    fun setMode(context: Context, mode: Int) {
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
            .edit().putInt(KEY_MODE, mode).apply()
        AppCompatDelegate.setDefaultNightMode(mode)
    }

    /** Call once, as early as possible (Application.onCreate), before any Activity is created. */
    fun applySavedMode(context: Context) {
        AppCompatDelegate.setDefaultNightMode(getSavedMode(context))
    }
}
