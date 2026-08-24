package com.saby.personalportfolio

import android.app.Application
import android.content.Intent
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner

class PersonalPortfolioApp : Application() {

    override fun onCreate() {
        super.onCreate()
        ThemePreference.applySavedMode(this)

        // ProcessLifecycleOwner fires ON_STOP when EVERY activity in the
        // app has left the foreground (not just one screen rotating or
        // navigating to another in-app screen) - exactly "the whole app
        // went to background". Whether that actually re-locks depends on
        // LockPreference's settings (enabled + grace period) - a brief
        // app-switch and back used to force a fresh unlock every single
        // time with no way to configure that, which is what was reported
        // as irritating.
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStop(owner: LifecycleOwner) {
                AppLockManager.backgroundedAtMillis = System.currentTimeMillis()
            }

            override fun onStart(owner: LifecycleOwner) {
                val app = this@PersonalPortfolioApp
                if (!LockPreference.isEnabled(app)) {
                    AppLockManager.isLocked = false
                    return
                }
                if (!AppLockManager.isLocked && AppLockManager.backgroundedAtMillis != 0L) {
                    val graceMs = LockPreference.graceSeconds(app) * 1000L
                    val elapsed = System.currentTimeMillis() - AppLockManager.backgroundedAtMillis
                    if (elapsed >= graceMs) {
                        AppLockManager.isLocked = true
                    }
                }
                if (AppLockManager.isLocked) {
                    val intent = Intent(app, LockActivity::class.java)
                    intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                    app.startActivity(intent)
                }
            }
        })
    }
}
