package com.saby.personalportfolio

import android.content.Context

/**
 * Persistent lock settings, same SharedPreferences pattern as
 * ThemePreference. Three independent choices:
 *
 * - enabled: master on/off switch for the whole lock screen.
 * - graceSeconds: how long the app can sit backgrounded (app-switched
 *   away from, not force-closed) before the NEXT foreground triggers a
 *   re-lock. Previously there was no grace period at all - ANY
 *   backgrounding (even a one-second app-switch to check a
 *   notification) immediately re-locked, which is what was reported as
 *   irritating.
 * - allowWeakBiometric: whether BIOMETRIC_WEAK-classified methods (most
 *   phones' plain camera-based face unlock, unlike a proper depth-sensor
 *   system) are accepted alongside BIOMETRIC_STRONG (fingerprint on
 *   virtually every phone). Off by default - this is a real security/
 *   convenience tradeoff for an app holding full financial data, so it
 *   should be an explicit opt-in, not silently loosened.
 */
object LockPreference {
    private const val PREFS_NAME = "lock_prefs"
    private const val KEY_ENABLED = "enabled"
    private const val KEY_GRACE_SECONDS = "grace_seconds"
    private const val KEY_ALLOW_WEAK_BIOMETRIC = "allow_weak_biometric"

    // Options shown in Settings, seconds. 0 means "no grace period -
    // lock immediately", matching the exact previous (unconfigurable)
    // behavior for anyone who wants to keep it.
    val GRACE_PERIOD_OPTIONS = listOf(0, 30, 60, 300, 900)
    val GRACE_PERIOD_LABELS = listOf(
        "Immediately", "After 30 seconds", "After 1 minute", "After 5 minutes", "After 15 minutes"
    )

    fun isEnabled(context: Context): Boolean =
        prefs(context).getBoolean(KEY_ENABLED, true)

    fun setEnabled(context: Context, enabled: Boolean) {
        prefs(context).edit().putBoolean(KEY_ENABLED, enabled).apply()
    }

    fun graceSeconds(context: Context): Int =
        prefs(context).getInt(KEY_GRACE_SECONDS, 0)

    fun setGraceSeconds(context: Context, seconds: Int) {
        prefs(context).edit().putInt(KEY_GRACE_SECONDS, seconds).apply()
    }

    fun allowWeakBiometric(context: Context): Boolean =
        prefs(context).getBoolean(KEY_ALLOW_WEAK_BIOMETRIC, false)

    fun setAllowWeakBiometric(context: Context, allow: Boolean) {
        prefs(context).edit().putBoolean(KEY_ALLOW_WEAK_BIOMETRIC, allow).apply()
    }

    private fun prefs(context: Context) =
        context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
}
