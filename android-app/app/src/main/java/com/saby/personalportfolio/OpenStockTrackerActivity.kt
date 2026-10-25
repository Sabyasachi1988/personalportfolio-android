package com.saby.personalportfolio

import android.app.Activity
import android.os.Bundle

/** Invisible hop used by the launcher shortcut: opens the Stock Tracker app and closes itself. */
class OpenStockTrackerActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        OtherApp.open(this, OtherApp.STOCK_TRACKER, "Stock Tracker")
        finish()
    }
}
