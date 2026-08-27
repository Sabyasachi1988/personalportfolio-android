package com.saby.personalportfolio

import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.ledger.bridge.Bridge
import java.util.concurrent.Executors

/**
 * CAS PDF / CSV import screen. Whose portfolio a statement belongs to
 * is picked from a dropdown of REAL, EXISTING members - never free
 * text, and never defaulted to "Me". This replaced an EditText that
 * defaulted to "Me" and matched whatever was typed against existing
 * members by exact name, silently creating a brand-new member on any
 * mismatch - a confirmed real risk: a typo ("Mom" vs "Mother") would
 * spawn a phantom duplicate family member nobody actually meant to
 * create. See bridge.CommitStagedRows' own doc comment for the other
 * half of this fix (it now requires a real member ID and errors rather
 * than auto-creating on a mismatch, so this isn't relying on the UI
 * alone to prevent it).
 */
class ImportActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())

    private lateinit var statusText: TextView
    private lateinit var transactionsList: RecyclerView
    private lateinit var commitButton: Button
    private lateinit var memberSpinner: Spinner
    private lateinit var memberSpinnerHint: TextView

    private var lastImportedRows: List<StagedRow> = emptyList()
    // Index 0 is always the "— Select member —" placeholder (not a real
    // choice); indices 1.. map 1:1 with members - same convention
    // MainActivity's own member spinner already uses, just without an
    // "All (family)" option here (a statement belongs to exactly one
    // person, never "family" as a whole).
    private var members: List<Member> = emptyList()

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
        memberSpinner = findViewById(R.id.memberSpinner)
        memberSpinnerHint = findViewById(R.id.memberSpinnerHint)

        memberSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                updateCommitButtonEnabled()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }

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

    override fun onResume() {
        super.onResume()
        // Reload every time this screen becomes visible, not just once
        // in onCreate - a member added via Manage Members (a natural
        // thing to do right before importing that person's first
        // statement) should show up here without having to leave and
        // re-enter this Activity.
        loadMembers()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadMembers() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val snapshot: PortfolioManualEntrySnapshot = try {
            gson.fromJson(portfolioJson, PortfolioManualEntrySnapshot::class.java)
        } catch (e: Exception) {
            PortfolioManualEntrySnapshot(emptyList(), emptyList(), emptyList())
        }
        val previousSelectionMemberId = selectedMemberId()
        members = snapshot.members.orEmpty()

        val labels = listOf("— Select member —") + members.map { it.name }
        memberSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, labels)

        // Keep the same member selected across a reload if they're still
        // present (e.g. after coming back from Manage Members having
        // added someone ELSE, not changed who was already picked here).
        val restoredIndex = members.indexOfFirst { it.id == previousSelectionMemberId }
        memberSpinner.setSelection(if (restoredIndex >= 0) restoredIndex + 1 else 0)

        memberSpinnerHint.visibility = if (members.isEmpty()) View.VISIBLE else View.GONE
        updateCommitButtonEnabled()
    }

    /** The selected member's real ID, or null if the placeholder ("— Select member —") is still selected. */
    private fun selectedMemberId(): String? {
        val position = memberSpinner.selectedItemPosition
        if (position <= 0) return null // 0 is the placeholder
        return members.getOrNull(position - 1)?.id
    }

    private fun updateCommitButtonEnabled() {
        val hasStagedRows = lastImportedRows.any { it.status == "NEW" }
        commitButton.isEnabled = hasStagedRows && selectedMemberId() != null
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
            updateCommitButtonEnabled()
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
        updateCommitButtonEnabled()
    }

    private fun commitImportedRows() {
        val newRows = lastImportedRows.filter { it.status == "NEW" }
        if (newRows.isEmpty()) {
            Toast.makeText(this, "No NEW rows to add", Toast.LENGTH_SHORT).show()
            return
        }
        val memberId = selectedMemberId()
        if (memberId == null) {
            Toast.makeText(this, "Pick whose statement this is first", Toast.LENGTH_SHORT).show()
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
                val commitResultJson = Bridge.commitStagedRows(currentPortfolioJson, rowsJson, memberId)
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

    private fun failCommit(message: String) {
        statusText.text = message
        updateCommitButtonEnabled()
    }
}

private data class CommitStagedRowsResult(
    val committed: Int,
    val skippedDuplicates: Int,
    val portfolio: com.google.gson.JsonObject
)
