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
        // went to background", which is the right moment to re-lock.
        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStop(owner: LifecycleOwner) {
                AppLockManager.isLocked = true
            }

            override fun onStart(owner: LifecycleOwner) {
                if (AppLockManager.isLocked) {
                    val intent = Intent(this@PersonalPortfolioApp, LockActivity::class.java)
                    intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                    startActivity(intent)
                }
            }
        })
    }
}
