package com.saby.personalportfolio

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

class ImportActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    private lateinit var statusText: TextView
    private lateinit var transactionsList: RecyclerView
    private lateinit var commitButton: Button
    private lateinit var memberNameInput: android.widget.EditText

    private var lastImportedRows: List<StagedRow> = emptyList()

    private val pickPdf = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            importFile(uri, isCsv = false)
        }
    }

    private val pickCsv = registerForActivityResult(ActivityResultContracts.OpenDocument()) { uri ->
        if (uri != null) {
            importFile(uri, isCsv = true)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_import)

        statusText = findViewById(R.id.statusText)
        transactionsList = findViewById(R.id.transactionsList)
        transactionsList.layoutManager = LinearLayoutManager(this)
        commitButton = findViewById(R.id.commitButton)
        memberNameInput = findViewById(R.id.memberNameInput)

        findViewById<Button>(R.id.importButton).setOnClickListener {
            pickPdf.launch(arrayOf("application/pdf"))
        }
        findViewById<Button>(R.id.importCsvButton).setOnClickListener {
            // "text/comma-separated-values" is included because some
            // Android file providers (notably Google Drive's) report CSV
            // files under that older MIME type instead of "text/csv",
            // and a few report the generic "text/plain" or even
            // "application/octet-stream" if the provider doesn't
            // recognize the extension at all - without those, the
            // system picker can grey out or hide an otherwise-valid CSV.
            pickCsv.launch(arrayOf("text/csv", "text/comma-separated-values", "text/plain", "application/octet-stream"))
        }
        commitButton.setOnClickListener { commitImportedRows() }

        commitButton.isEnabled = false
    }

    private fun importFile(uri: Uri, isCsv: Boolean) {
        statusText.text = if (isCsv) "Reading and parsing CSV…" else "Reading and parsing PDF…"
        commitButton.isEnabled = false

        backgroundExecutor.execute {
            try {
                val fileBytes = contentResolver.openInputStream(uri)?.use { it.readBytes() }
                    ?: throw IllegalStateException("Could not open the selected file")

                val resultJson = if (isCsv) Bridge.importCSV(fileBytes) else Bridge.importCAS(fileBytes)
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

                val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { failCommit("Failed to load existing portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val rowsJson = gson.toJson(newRows)
                val memberName = memberNameInput.text.toString().trim().ifBlank { "Me" }
                val commitResultJson = Bridge.commitStagedRows(currentPortfolioJson, rowsJson, memberName)
                if (isBridgeError(commitResultJson)) {
                    mainThread.post { failCommit("Failed to link transactions: $commitResultJson") }
                    return@execute
                }

                val commitResult = try {
                    gson.fromJson(commitResultJson, CommitStagedRowsResult::class.java)
                } catch (e: Exception) {
                    mainThread.post { failCommit("Commit returned unexpected data: ${e.message}") }
                    return@execute
                }
                val updatedPortfolioJson = gson.toJson(commitResult.portfolio)

                val saveResult = Bridge.savePortfolio(portfolioPath, updatedPortfolioJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { failCommit("Failed to save portfolio: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    statusText.text = if (commitResult.skippedDuplicates > 0) {
                        "Added ${commitResult.committed} transaction(s). Skipped ${commitResult.skippedDuplicates} already in your portfolio (this statement overlaps with a previous import)."
                    } else {
                        "Added ${commitResult.committed} transaction(s) to your portfolio."
                    }
                    Toast.makeText(this, "Saved — go back to see it on your Dashboard.", Toast.LENGTH_LONG).show()
                }
            } catch (e: Exception) {
                mainThread.post { failCommit("Failed to commit: ${e.message}") }
            }
        }
    }

    private fun isBridgeError(json: String): Boolean {
        return json.trimStart().startsWith("{\"error\"")
    }

    private fun failCommit(message: String) {
        statusText.text = message
        commitButton.isEnabled = true
    }
}

private data class CommitStagedRowsResult(
    val committed: Int,
    val skippedDuplicates: Int,
    val portfolio: com.google.gson.JsonObject
)
