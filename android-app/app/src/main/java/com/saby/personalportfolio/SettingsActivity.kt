package com.saby.personalportfolio

import android.app.AlertDialog
import android.net.Uri
import android.os.Bundle
import android.widget.ArrayAdapter
import android.widget.EditText
import android.widget.RadioButton
import android.widget.RadioGroup
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import com.google.android.material.switchmaterial.SwitchMaterial
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
        setupLockSettings()
        setupPopupDurationSetting()

        findViewById<android.widget.Button>(R.id.manageMembersButton).setOnClickListener {
            startActivity(android.content.Intent(this, MembersActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageForeignHoldingsButton).setOnClickListener {
            startActivity(android.content.Intent(this, ManualHoldingsActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageFundGroupsButton).setOnClickListener {
            startActivity(android.content.Intent(this, FundGroupsActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageTagsButton).setOnClickListener {
            startActivity(android.content.Intent(this, TagsActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageBenchmarksButton).setOnClickListener {
            startActivity(android.content.Intent(this, BenchmarksActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.fixAssetSymbolButton).setOnClickListener {
            startActivity(android.content.Intent(this, FixAssetSymbolActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.updateHistoryButton).setOnClickListener {
            startActivity(android.content.Intent(this, UpdateHistoryActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageAssetClassButton).setOnClickListener {
            startActivity(android.content.Intent(this, CapCompositionActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageTargetAllocationButton).setOnClickListener {
            startActivity(android.content.Intent(this, TargetAllocationActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageEquityOriginButton).setOnClickListener {
            startActivity(android.content.Intent(this, EquityOriginCompositionActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.managePortfolioClassTargetButton).setOnClickListener {
            startActivity(android.content.Intent(this, PortfolioClassTargetActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.manageNamesButton).setOnClickListener {
            startActivity(android.content.Intent(this, ManageNamesActivity::class.java))
        }

        findViewById<android.widget.Button>(R.id.exportButton).setOnClickListener {
            val stamp = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())
            createBackupFile.launch("personalportfolio-backup-$stamp.json")
        }

        findViewById<android.widget.Button>(R.id.importButton).setOnClickListener {
            pickBackupFile.launch(arrayOf("application/json"))
        }

        findViewById<android.widget.Button>(R.id.wipeAllDataButton).setOnClickListener {
            confirmWipeAllData()
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

    /**
     * Grace period + strictness settings persisted via LockPreference,
     * consumed by PersonalPortfolioApp's lifecycle observer and
     * LockActivity respectively - see those files' doc comments.
     */
    private fun setupLockSettings() {
        val enabledSwitch = findViewById<SwitchMaterial>(R.id.lockEnabledSwitch)
        val graceSpinner = findViewById<Spinner>(R.id.lockGraceSpinner)
        val strictnessGroup = findViewById<RadioGroup>(R.id.lockBiometricStrictnessGroup)
        val strictRadio = findViewById<RadioButton>(R.id.lockStrictRadio)
        val weakRadio = findViewById<RadioButton>(R.id.lockWeakRadio)

        enabledSwitch.isChecked = LockPreference.isEnabled(this)
        enabledSwitch.setOnCheckedChangeListener { _, checked ->
            LockPreference.setEnabled(this, checked)
        }

        graceSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, LockPreference.GRACE_PERIOD_LABELS)
        val currentGraceIndex = LockPreference.GRACE_PERIOD_OPTIONS.indexOf(LockPreference.graceSeconds(this)).coerceAtLeast(0)
        graceSpinner.setSelection(currentGraceIndex)
        graceSpinner.onItemSelectedListener = object : android.widget.AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: android.widget.AdapterView<*>?, view: android.view.View?, position: Int, id: Long) {
                LockPreference.setGraceSeconds(this@SettingsActivity, LockPreference.GRACE_PERIOD_OPTIONS.getOrElse(position) { 0 })
            }
            override fun onNothingSelected(parent: android.widget.AdapterView<*>?) {}
        }

        if (LockPreference.allowWeakBiometric(this)) weakRadio.isChecked = true else strictRadio.isChecked = true
        strictnessGroup.setOnCheckedChangeListener { _, checkedId ->
            LockPreference.setAllowWeakBiometric(this, checkedId == R.id.lockWeakRadio)
        }
    }

    /**
     * How long the self-dismissing chart-marker/period-gain popup stays
     * visible - see PopupDurationPreference's own doc comment. A free-
     * text seconds field (any decimal, e.g. 1.75), not a fixed set of
     * choices - a confirmed real ask not to be confined to preset
     * options. The quick-pick chips below it are tap-to-FILL shortcuts
     * into that same field, not a separate constrained control.
     */
    private fun setupPopupDurationSetting() {
        val input = findViewById<android.widget.EditText>(R.id.popupDurationInput)
        val saveButton = findViewById<TextView>(R.id.popupDurationSaveButton)
        val quickPicksGroup = findViewById<com.google.android.material.chip.ChipGroup>(R.id.popupDurationQuickPicks)

        val currentSeconds = PopupDurationPreference.durationMs(this) / 1000.0
        input.setText(formatSeconds(currentSeconds))

        PopupDurationPreference.DURATION_OPTIONS_SECONDS.forEachIndexed { i, seconds ->
            val chip = com.google.android.material.chip.Chip(this)
            chip.text = PopupDurationPreference.DURATION_LABELS[i]
            chip.isClickable = true
            chip.setOnClickListener { input.setText(formatSeconds(seconds)) }
            quickPicksGroup.addView(chip)
        }

        saveButton.setOnClickListener {
            val seconds = input.text?.toString()?.trim()?.toDoubleOrNull()
            if (seconds == null || seconds <= 0) {
                input.error = "Enter a positive number of seconds, e.g. 1.75"
                return@setOnClickListener
            }
            input.error = null
            PopupDurationPreference.setDurationSeconds(this, seconds)
            Toast.makeText(this, "Popup duration set to ${formatSeconds(seconds)}s", Toast.LENGTH_SHORT).show()
        }
    }

    private fun formatSeconds(seconds: Double): String {
        // No trailing ".0" for a whole number, but keeps real decimals
        // (1.75 stays 1.75) - purely cosmetic, doesn't affect what's
        // actually saved.
        return if (seconds == seconds.toLong().toDouble()) {
            seconds.toLong().toString()
        } else {
            seconds.toString()
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

    /**
     * Two-step gate before anything destructive happens: type "DELETE"
     * to prove this isn't an accidental tap, THEN a biometric/device
     * credential check to prove it's actually the phone's owner acting
     * (not, say, a child who picked up an unlocked phone). Either step
     * alone would be weaker than the Restore-from-backup confirmation
     * above deserves, given this has no in-app undo.
     */
    private fun confirmWipeAllData() {
        val dialogView = layoutInflater.inflate(R.layout.dialog_confirm_wipe, null)
        val confirmInput = dialogView.findViewById<EditText>(R.id.wipeConfirmInput)

        val dialog = AlertDialog.Builder(this)
            .setTitle("Wipe all portfolio data?")
            .setView(dialogView)
            .setPositiveButton("Continue", null) // set below, after creation, so it can validate before dismissing
            .setNegativeButton("Cancel", null)
            .create()

        dialog.setOnShowListener {
            val positiveButton = dialog.getButton(AlertDialog.BUTTON_POSITIVE)
            positiveButton.setOnClickListener {
                if (confirmInput.text.toString().trim() == "DELETE") {
                    dialog.dismiss()
                    verifyIdentityThenWipe()
                } else {
                    confirmInput.error = "Type DELETE exactly, in capitals"
                }
            }
        }
        dialog.show()
    }

    private fun verifyIdentityThenWipe() {
        val biometricManager = BiometricManager.from(this)
        val canAuthenticate = biometricManager.canAuthenticate(
            BiometricManager.Authenticators.BIOMETRIC_STRONG or
                BiometricManager.Authenticators.DEVICE_CREDENTIAL
        )
        if (canAuthenticate != BiometricManager.BIOMETRIC_SUCCESS) {
            // No fingerprint/face AND no PIN/pattern/password enrolled -
            // same situation LockActivity handles by skipping the check
            // entirely rather than blocking on a credential that can't
            // exist. The typed "DELETE" confirmation above still applies.
            performWipe()
            return
        }

        val executor = ContextCompat.getMainExecutor(this)
        val prompt = BiometricPrompt(this, executor, object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                performWipe()
            }
            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                statusText.text = "Wipe cancelled - authentication not completed ($errString)"
            }
            override fun onAuthenticationFailed() {
                // Keep prompting - BiometricPrompt itself stays open and
                // lets the person retry, this is just a failed attempt,
                // not a final cancellation.
            }
        })
        val promptInfo = BiometricPrompt.PromptInfo.Builder()
            .setTitle("Verify it's you")
            .setSubtitle("Confirm to permanently wipe all portfolio data")
            .setAllowedAuthenticators(
                BiometricManager.Authenticators.BIOMETRIC_STRONG or
                    BiometricManager.Authenticators.DEVICE_CREDENTIAL
            )
            .build()
        prompt.authenticate(promptInfo)
    }

    private fun performWipe() {
        val portfolioPath = PortfolioStorage.filePath(this)
        // Passing an empty string (not "{}") to SavePortfolio leaves it
        // at Go's zero-value store.Portfolio{} - see SavePortfolio's doc
        // comment. Since the file currently exists, store.Save's own
        // backupBeforeWrite runs first, so the pre-wipe state is kept in
        // the app's backups/ folder even though there's no in-app UI to
        // browse and restore from it today.
        val saveResult = com.ledger.bridge.Bridge.savePortfolio(portfolioPath, "")
        if (saveResult.trimStart().startsWith("{\"error\"")) {
            statusText.text = "Wipe failed: $saveResult"
            return
        }
        statusText.text = "All portfolio data wiped. Starting from scratch."
        Toast.makeText(this, "Portfolio wiped", Toast.LENGTH_LONG).show()
    }
}
