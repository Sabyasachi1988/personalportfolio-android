package com.saby.personalportfolio

/**
 * Tracks whether the app is currently "locked" behind biometric auth.
 * Deliberately in-memory only (not persisted to disk/SharedPreferences):
 * a fresh process start should always require unlocking, and returning
 * from the background should re-lock, same as a real device lock screen.
 */
object AppLockManager {
    // Starts true so the very first launch requires unlocking.
    @Volatile
    var isLocked: Boolean = true
}
