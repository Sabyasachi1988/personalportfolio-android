package com.saby.personalportfolio

import android.app.AlertDialog
import android.net.Uri
import android.os.Bundle
import android.widget.RadioButton
import android.widget.RadioGroup
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.JsonParser
import java.io.File
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

class SettingsActivity : AppCompatActivity() {

    private lateinit var statusText: TextView

    private val createBackupFile = registerForActivityResult(ActivityResultContracts.CreateDocument("application/json")) { uri ->
        if (uri != null) exportTo(uri)
    }

    private val pickBackupFile = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) confirmAndImportFrom(uri)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_settings)

        statusText = findViewById(R.id.settingsStatusText)
        setupThemeToggle()

        findViewById<android.widget.Button>(R.id.exportButton).setOnClickListener {
            val stamp = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())
            createBackupFile.launch("personalportfolio-backup-$stamp.json")
        }

        findViewById<android.widget.Button>(R.id.importButton).setOnClickListener {
            pickBackupFile.launch(arrayOf("application/json"))
        }
    }

    private fun setupThemeToggle() {
        val group = findViewById<RadioGroup>(R.id.themeRadioGroup)
        val systemRadio = findViewById<RadioButton>(R.id.themeSystemRadio)
        val lightRadio = findViewById<RadioButton>(R.id.themeLightRadio)
        val darkRadio = findViewById<RadioButton>(R.id.themeDarkRadio)

        when (ThemePreference.getSavedMode(this)) {
            ThemePreference.MODE_LIGHT -> lightRadio.isChecked = true
            ThemePreference.MODE_DARK -> darkRadio.isChecked = true
            else -> systemRadio.isChecked = true
        }

        group.setOnCheckedChangeListener { _, checkedId ->
            val mode = when (checkedId) {
                R.id.themeLightRadio -> ThemePreference.MODE_LIGHT
                R.id.themeDarkRadio -> ThemePreference.MODE_DARK
                else -> ThemePreference.MODE_SYSTEM
            }
            ThemePreference.setMode(this, mode)
        }
    }

    private fun exportTo(uri: Uri) {
        try {
            val portfolioFile = File(PortfolioStorage.filePath(this))
            if (!portfolioFile.exists()) {
                statusText.text = "Nothing to export yet - no portfolio data has been created."
                return
            }
            contentResolver.openOutputStream(uri)?.use { out ->
                portfolioFile.inputStream().use { input -> input.copyTo(out) }
            }
            statusText.text = "Backup exported successfully."
            Toast.makeText(this, "Backup saved", Toast.LENGTH_SHORT).show()
        } catch (e: Exception) {
            statusText.text = "Export failed: ${e.message}"
        }
    }

    private fun confirmAndImportFrom(uri: Uri) {
        // Validate BEFORE asking to confirm the destructive overwrite -
        // no point scaring the person with an "this will replace
        // everything" warning for a file that turns out to not even be
        // valid JSON.
        val text = try {
            contentResolver.openInputStream(uri)?.bufferedReader()?.use { it.readText() }
        } catch (e: Exception) {
            statusText.text = "Could not read that file: ${e.message}"
            return
        }
        if (text == null) {
            statusText.text = "Could not read that file."
            return
        }
        try {
            JsonParser.parseString(text)
        } catch (e: Exception) {
            statusText.text = "That file doesn't look like a valid backup (not valid JSON)."
            return
        }

        AlertDialog.Builder(this)
            .setTitle("Replace everything with this backup?")
            .setMessage("This overwrites all current holdings, transactions, and settings with what's in the backup file. Anything added since that backup was made will be lost. This can't be undone.")
            .setPositiveButton("Restore") { _, _ -> writeAsCurrentPortfolio(text) }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun writeAsCurrentPortfolio(json: String) {
        val saveResult = com.ledger.bridge.Bridge.savePortfolio(PortfolioStorage.filePath(this), json)
        if (saveResult.trimStart().startsWith("{\"error\"")) {
            statusText.text = "Restore failed: $saveResult"
            return
        }
        statusText.text = "Restored from backup."
        Toast.makeText(this, "Restored", Toast.LENGTH_SHORT).show()
    }
}
