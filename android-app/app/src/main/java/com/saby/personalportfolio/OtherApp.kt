package com.saby.personalportfolio

import android.content.Context
import android.content.Intent
import android.widget.Toast

/** Opens the sibling app installed on the same phone (the manifest lists it under <queries>). */
object OtherApp {
    const val STOCK_TRACKER = "com.saby.stocktracker"

    fun open(c: Context, pkg: String, label: String) {
        val i = c.packageManager.getLaunchIntentForPackage(pkg)
        if (i == null) {
            Toast.makeText(c, "$label is not installed on this phone", Toast.LENGTH_LONG).show()
            return
        }
        i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        c.startActivity(i)
    }
}
