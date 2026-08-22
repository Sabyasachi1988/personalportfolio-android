package com.saby.personalportfolio

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Button
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.ledger.bridge.Bridge
import java.util.concurrent.Executors

class MainActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    private lateinit var statusText: TextView
    private lateinit var transactionsList: RecyclerView
    private lateinit var commitButton: Button
    private lateinit var viewHoldingsButton: Button
    private lateinit var memberNameInput: android.widget.EditText

    // The most recently imported rows, kept in memory so the "Add to
    // Portfolio" button has something to commit without re-parsing the PDF.
    private var lastImportedRows: List<StagedRow> = emptyList()

    private val pickPdf = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            importCasFile(uri)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)

        statusText = findViewById(R.id.statusText)
        transactionsList = findViewById(R.id.transactionsList)
        transactionsList.layoutManager = LinearLayoutManager(this)
        commitButton = findViewById(R.id.commitButton)
        viewHoldingsButton = findViewById(R.id.viewHoldingsButton)
        memberNameInput = findViewById(R.id.memberNameInput)

        findViewById<Button>(R.id.importButton).setOnClickListener {
            pickPdf.launch(arrayOf("application/pdf"))
        }
        commitButton.setOnClickListener { commitImportedRows() }
        viewHoldingsButton.setOnClickListener {
            startActivity(Intent(this, HoldingsActivity::class.java))
        }
        findViewById<Button>(R.id.settingsButton).setOnClickListener {
            startActivity(Intent(this, SettingsActivity::class.java))
        }

        commitButton.isEnabled = false
    }

    private fun importCasFile(uri: Uri) {
        statusText.text = "Reading and parsing PDF…"
        commitButton.isEnabled = false

        backgroundExecutor.execute {
            try {
                val pdfBytes = contentResolver.openInputStream(uri)?.use { it.readBytes() }
                    ?: throw IllegalStateException("Could not open the selected file")

                val resultJson = Bridge.importCAS(pdfBytes)
                val result = gson.fromJson(resultJson, ImportCASResult::class.java)

                mainThread.post { showResult(result) }
            } catch (e: Exception) {
                mainThread.post {
                    statusText.text = "Failed to read/import file: ${e.message}"
                    Toast.makeText(this, "Import failed", Toast.LENGTH_SHORT).show()
                }
            }
        }
    }

    private fun showResult(result: ImportCASResult) {
        if (result.error != null) {
            statusText.text = "Import error: ${result.error}"
            transactionsList.adapter = TransactionAdapter(emptyList())
            lastImportedRows = emptyList()
            commitButton.isEnabled = false
            return
        }

        val staged = result.staged.orEmpty()
        val manualReview = result.manualReview.orEmpty()
        lastImportedRows = staged

        statusText.text = buildString {
            append("Format: ${result.format}\n")
            append("${staged.size} transaction(s) parsed")
            if (manualReview.isNotEmpty()) {
                append(", ${manualReview.size} line(s) need manual review")
            }
        }

        transactionsList.adapter = TransactionAdapter(staged)
        commitButton.isEnabled = staged.any { it.status == "NEW" }
    }

    private fun commitImportedRows() {
        val newRows = lastImportedRows.filter { it.status == "NEW" }
        if (newRows.isEmpty()) {
            Toast.makeText(this, "No NEW rows to add", Toast.LENGTH_SHORT).show()
            return
        }

        commitButton.isEnabled = false
        statusText.text = "Adding ${newRows.size} transaction(s) to your portfolio…"

        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)

                val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { failCommit("Failed to load existing portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val rowsJson = gson.toJson(newRows)
                val memberName = memberNameInput.text.toString().trim().ifBlank { "Me" }
                val updatedPortfolioJson = Bridge.commitStagedRows(currentPortfolioJson, rowsJson, memberName)
                if (isBridgeError(updatedPortfolioJson)) {
                    mainThread.post { failCommit("Failed to link transactions: $updatedPortfolioJson") }
                    return@execute
                }

                // Only ever save a portfolio JSON we've confirmed is real,
                // never an error object - an unguarded save here could
                // silently clobber a good portfolio file with an empty one.
                val saveResult = Bridge.savePortfolio(portfolioPath, updatedPortfolioJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { failCommit("Failed to save portfolio: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    statusText.text = "Added ${newRows.size} transaction(s) to your portfolio."
                    Toast.makeText(this, "Saved. Tap 'View Holdings' to see it.", Toast.LENGTH_LONG).show()
                }
            } catch (e: Exception) {
                mainThread.post { failCommit("Failed to commit: ${e.message}") }
            }
        }
    }

    // The bridge's error responses are always a JSON object with only an
    // "error" key (see bridge.go) - a real portfolio/result JSON never has
    // this shape, so this check is safe and cheap without needing a full
    // parse.
    private fun isBridgeError(json: String): Boolean {
        return json.trimStart().startsWith("{\"error\"")
    }

    private fun failCommit(message: String) {
        statusText.text = message
        commitButton.isEnabled = true
    }
}
