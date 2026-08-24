package com.saby.personalportfolio

/**
 * Tracks whether the app is currently "locked" behind biometric auth.
 * Deliberately in-memory only (not persisted to disk/SharedPreferences):
 * a fresh process start should always require unlocking (isLocked starts
 * true). Whether returning from the background re-locks depends on
 * LockPreference's grace period and enabled settings - see
 * PersonalPortfolioApp's lifecycle observer for the actual decision.
 */
object AppLockManager {
    // Starts true so the very first launch requires unlocking.
    @Volatile
    var isLocked: Boolean = true

    // Wall-clock time (System.currentTimeMillis()) of the most recent
    // onStop (whole app left the foreground) - 0 means "never
    // backgrounded yet this process". Used to measure elapsed time
    // against LockPreference's grace period on the next onStart, rather
    // than re-locking on every single app-switch regardless of duration.
    @Volatile
    var backgroundedAtMillis: Long = 0L
}
