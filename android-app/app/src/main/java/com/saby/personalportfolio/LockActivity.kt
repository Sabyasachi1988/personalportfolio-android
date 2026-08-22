package com.saby.personalportfolio

import android.os.Bundle
import android.widget.Button
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat

class LockActivity : AppCompatActivity() {

    private lateinit var statusText: TextView
    private lateinit var biometricPrompt: BiometricPrompt
    private lateinit var promptInfo: BiometricPrompt.PromptInfo

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_lock)

        statusText = findViewById(R.id.lockStatusText)
        val unlockButton = findViewById<Button>(R.id.unlockButton)

        val biometricManager = BiometricManager.from(this)
        val canAuthenticate = biometricManager.canAuthenticate(
            BiometricManager.Authenticators.BIOMETRIC_STRONG or
                BiometricManager.Authenticators.DEVICE_CREDENTIAL
        )

        if (canAuthenticate != BiometricManager.BIOMETRIC_SUCCESS) {
            // No fingerprint/face AND no PIN/pattern/password enrolled at
            // all on this device - there is no credential to lock behind.
            // Skip locking entirely rather than trapping the person
            // outside their own app with no way to prove who they are.
            unlock()
            return
        }

        val executor = ContextCompat.getMainExecutor(this)
        biometricPrompt = BiometricPrompt(this, executor, object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                unlock()
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                // User cancelled, or backed out (e.g. pressed the phone's
                // back button) - stay locked and closeable, not a crash.
                statusText.text = "Authentication cancelled ($errString)"
            }

            override fun onAuthenticationFailed() {
                statusText.text = "Not recognized - try again"
            }
        })

        promptInfo = BiometricPrompt.PromptInfo.Builder()
            .setTitle("Unlock Personal Portfolio")
            .setAllowedAuthenticators(
                BiometricManager.Authenticators.BIOMETRIC_STRONG or
                    BiometricManager.Authenticators.DEVICE_CREDENTIAL
            )
            .build()

        statusText.text = "Verify it's you to continue"
        unlockButton.setOnClickListener { biometricPrompt.authenticate(promptInfo) }

        // Prompt immediately on open - the button is a fallback for
        // if the automatic prompt gets dismissed without a result.
        biometricPrompt.authenticate(promptInfo)
    }

    private fun unlock() {
        AppLockManager.isLocked = false
        finish()
    }
}
