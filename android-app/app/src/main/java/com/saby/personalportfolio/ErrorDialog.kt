package com.saby.personalportfolio

import android.app.Activity
import android.content.ClipData
import android.content.ClipboardManager
import android.widget.Toast
import androidx.appcompat.app.AlertDialog

/**
 * Shows the full text of an error in a scrollable, copyable dialog -
 * a plain Toast ellipsizes anything past ~2 lines with no way to read
 * or copy the rest, which is exactly what made a real diagnosis (e.g.
 * a wrapped root-cause message, or a raw server response body some
 * fetch functions include on failure) invisible on-device in more than
 * one screen. First built inline in BenchmarksActivity, extracted here
 * so it doesn't get re-duplicated a third time.
 */
object ErrorDialog {
    fun show(activity: Activity, title: String, message: String) {
        AlertDialog.Builder(activity)
            .setTitle(title)
            .setMessage(message)
            .setPositiveButton("OK", null)
            .setNeutralButton("Copy") { _, _ ->
                val clipboard = activity.getSystemService(ClipboardManager::class.java)
                clipboard?.setPrimaryClip(ClipData.newPlainText(title, message))
                Toast.makeText(activity, "Copied", Toast.LENGTH_SHORT).show()
            }
            .show()
    }
}
