package com.saby.personalportfolio

import android.content.Intent
import android.os.Bundle
import android.view.View
import androidx.appcompat.app.AppCompatActivity

/**
 * The "more" screen behind the Dashboard's ⋮ button: everything that is not
 * one of the five bottom-bar destinations. Transactions moved here when
 * Returns took its place in the bar (the bar holds at most five items).
 */
class MoreActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_more)
        findViewById<View>(R.id.moreTransactionsCard).setOnClickListener {
            startActivity(Intent(this, TransactionsActivity::class.java))
        }
        findViewById<View>(R.id.moreImportCard).setOnClickListener {
            startActivity(Intent(this, ImportActivity::class.java))
        }
        findViewById<View>(R.id.moreSettingsCard).setOnClickListener {
            startActivity(Intent(this, SettingsActivity::class.java))
        }
    }
}
