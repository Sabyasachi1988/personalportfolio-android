package com.saby.personalportfolio

import android.app.AlertDialog
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.Editable
import android.text.TextWatcher
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.EditText
import android.widget.Spinner
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.ledger.bridge.Bridge
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.concurrent.Executors

class TransactionsActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())
    private lateinit var recyclerView: RecyclerView
    private lateinit var searchInput: EditText
    private lateinit var sortSpinner: Spinner

    private var allTransactions: List<StoredTransactionEntry> = emptyList()
    private var assetNameById: Map<String, String> = emptyMap()
    private var assetRawNameById: Map<String, String> = emptyMap() // NEVER nickname-resolved - see buildTransactionsCsv's use
    private var assetIsinById: Map<String, String> = emptyMap()

    private val createCsvFile = registerForActivityResult(ActivityResultContracts.CreateDocument("text/csv")) { uri ->
        if (uri != null) exportCsvTo(uri)
    }

    private val sortOptions = listOf(
        "Date (newest first)", "Date (oldest first)",
        "Amount (high to low)", "Amount (low to high)"
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_transactions)

        recyclerView = findViewById(R.id.transactionsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        searchInput = findViewById(R.id.transactionsSearchInput)
        sortSpinner = findViewById(R.id.transactionsSortSpinner)
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.TRANSACTIONS)

        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.TRANSACTIONS)

        findViewById<android.widget.Button>(R.id.exportCsvButton).setOnClickListener {
            val stamp = SimpleDateFormat("yyyy-MM-dd", Locale.US).format(Date())
            createCsvFile.launch("personalportfolio-transactions-$stamp.csv")
        }

        findViewById<android.widget.Button>(R.id.removeDuplicatesButton).setOnClickListener {
            confirmRemoveDuplicates()
        }

        sortSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, sortOptions)
        sortSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                applyFilterAndSort()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }

        searchInput.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: Editable?) {
                applyFilterAndSort()
            }
        })
    }

    override fun onResume() {
        super.onResume()
        // Re-sync every time this screen resumes, not just once in
        // onCreate - a screen reused via CLEAR_TOP (coming back to it
        // from another tab) never re-runs onCreate, so without this
        // its nav bar could keep showing a stale selection.
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.TRANSACTIONS)
        loadTransactions()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadTransactions() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)

        val snapshot: PortfolioAssetsSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioAssetsSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioAssetsSnapshot(emptyList(), emptyList(), emptyList())
        }

        assetNameById = snapshot.assets.orEmpty().associate { it.id to NicknameResolver.resolve(it.name, it.nickname) }
        assetRawNameById = snapshot.assets.orEmpty().associate { it.id to it.name }
        assetIsinById = snapshot.assets.orEmpty().associate { it.id to it.isin }
        allTransactions = snapshot.transactions.orEmpty()

        applyFilterAndSort()
    }

    private fun applyFilterAndSort() {
        val query = searchInput.text?.toString()?.trim().orEmpty()
        var result = if (query.isBlank()) {
            allTransactions
        } else {
            allTransactions.filter { txn ->
                val name = assetNameById[txn.assetId] ?: ""
                name.contains(query, ignoreCase = true)
            }
        }

        result = when (sortSpinner.selectedItemPosition) {
            0 -> result.sortedByDescending { it.date }
            1 -> result.sortedBy { it.date }
            2 -> result.sortedByDescending { it.amount }
            3 -> result.sortedBy { it.amount }
            else -> result.sortedByDescending { it.date }
        }

        recyclerView.adapter = TransactionsAdapter(result, assetNameById) { txn ->
            showEditDialog(txn, assetNameById[txn.assetId] ?: "(unknown asset)")
        }
    }

    private fun showEditDialog(txn: StoredTransactionEntry, assetName: String) {
        val dialogView = layoutInflater.inflate(R.layout.dialog_edit_transaction, null)
        val dateInput = dialogView.findViewById<EditText>(R.id.editDate)
        val amountInput = dialogView.findViewById<EditText>(R.id.editAmount)
        val unitsInput = dialogView.findViewById<EditText>(R.id.editUnits)

        dateInput.setText(txn.date)
        amountInput.setText(txn.amount.toString())
        unitsInput.setText(txn.units?.toString() ?: "")

        AlertDialog.Builder(this)
            .setTitle(assetName)
            .setView(dialogView)
            .setPositiveButton("Save") { _, _ ->
                val date = dateInput.text.toString().trim()
                val amount = amountInput.text.toString().toDoubleOrNull()
                val units = unitsInput.text.toString().toDoubleOrNull()
                if (date.isBlank() || amount == null || units == null) {
                    Toast.makeText(this, "Please fill in all fields with valid numbers", Toast.LENGTH_SHORT).show()
                    return@setPositiveButton
                }
                saveTransactionEdit(txn.id, date, amount, units)
            }
            .setNeutralButton("Delete") { _, _ ->
                confirmDelete(txn.id, assetName)
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun confirmDelete(txnId: String, assetName: String) {
        AlertDialog.Builder(this)
            .setTitle("Delete this transaction?")
            .setMessage("This removes the $assetName transaction permanently. This can't be undone.")
            .setPositiveButton("Delete") { _, _ -> deleteTransaction(txnId) }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun saveTransactionEdit(txnId: String, date: String, amount: Double, units: Double) {
        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { showError("Failed to load portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val updatedJson = Bridge.updateTransaction(currentPortfolioJson, txnId, date, amount, units)
                if (isBridgeError(updatedJson)) {
                    mainThread.post { showError("Failed to update: $updatedJson") }
                    return@execute
                }

                val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { showError("Failed to save: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    Toast.makeText(this, "Updated", Toast.LENGTH_SHORT).show()
                    loadTransactions()
                }
            } catch (e: Exception) {
                mainThread.post { showError("Failed: ${e.message}") }
            }
        }
    }

    private fun deleteTransaction(txnId: String) {
        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { showError("Failed to load portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val updatedJson = Bridge.deleteTransaction(currentPortfolioJson, txnId)
                if (isBridgeError(updatedJson)) {
                    mainThread.post { showError("Failed to delete: $updatedJson") }
                    return@execute
                }

                val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { showError("Failed to save: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    Toast.makeText(this, "Deleted", Toast.LENGTH_SHORT).show()
                    loadTransactions()
                }
            } catch (e: Exception) {
                mainThread.post { showError("Failed: ${e.message}") }
            }
        }
    }

    // Scans first (read-only - RemoveDuplicateTransactions doesn't save
    // anything by itself) so the confirmation dialog can show a real
    // count rather than a generic "this might remove things" warning,
    // and so a person with a clean portfolio gets a reassuring "nothing
    // found" message instead of a pointless confirmation prompt.
    private fun confirmRemoveDuplicates() {
        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { showError("Failed to load portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val scanResultJson = Bridge.removeDuplicateTransactions(currentPortfolioJson)
                if (isBridgeError(scanResultJson)) {
                    mainThread.post { showError("Failed to scan for duplicates: $scanResultJson") }
                    return@execute
                }
                val scanResult = try {
                    gson.fromJson(scanResultJson, RemoveDuplicatesResult::class.java)
                } catch (e: Exception) {
                    mainThread.post { showError("Duplicate scan returned unexpected data: ${e.message}") }
                    return@execute
                }

                mainThread.post {
                    if (scanResult.removed == 0) {
                        Toast.makeText(this, "No duplicate transactions found.", Toast.LENGTH_SHORT).show()
                    } else {
                        val groups = scanResult.groups.orEmpty()
                        val groupLines = groups.take(12).joinToString("\n") { g ->
                            val fundName = FundNameFormatter.shorten(g.assetName.ifBlank { "(unknown fund)" })
                            val amountStr = IndianCurrencyFormatter.format(g.amount)
                            val confidenceTag = when (g.confidence) {
                                "reference" -> " [confirmed: same bank reference]"
                                "balance" -> " [confirmed: same running balance]"
                                else -> " [unconfirmed - date/amount/units only]"
                            }
                            "• $fundName — ${g.date}, $amountStr (${g.extraCopies} extra cop${if (g.extraCopies == 1) "y" else "ies"})$confidenceTag"
                        }
                        val moreLine = if (groups.size > 12) "\n…and ${groups.size - 12} more group(s)" else ""
                        val anyUnconfirmed = groups.any { it.confidence == "heuristic" }

                        val warning = if (anyUnconfirmed) {
                            "Rows tagged [confirmed] were verified against the bank reference or the statement's own running balance - as certain as the data allows. " +
                                "Rows tagged [unconfirmed] only match on date/amount/units, which a genuine second same-day purchase can also produce - check those against what you actually did."
                        } else {
                            "All groups above were confirmed via the bank reference or the statement's own running balance, not just amount/date matching."
                        }

                        AlertDialog.Builder(this)
                            .setTitle("Remove ${scanResult.removed} duplicate transaction(s)?")
                            .setMessage(
                                "$groupLines$moreLine\n\n$warning\n\nThis can't be undone; export a backup first if unsure."
                            )
                            .setPositiveButton("Remove") { _, _ -> removeDuplicates() }
                            .setNegativeButton("Cancel", null)
                            .show()
                    }
                }
            } catch (e: Exception) {
                mainThread.post { showError("Failed: ${e.message}") }
            }
        }
    }

    private fun removeDuplicates() {
        backgroundExecutor.execute {
            try {
                val portfolioPath = PortfolioStorage.filePath(this)
                val currentPortfolioJson = PortfolioLoadCache.load(portfolioPath)
                if (isBridgeError(currentPortfolioJson)) {
                    mainThread.post { showError("Failed to load portfolio: $currentPortfolioJson") }
                    return@execute
                }

                val resultJson = Bridge.removeDuplicateTransactions(currentPortfolioJson)
                if (isBridgeError(resultJson)) {
                    mainThread.post { showError("Failed to remove duplicates: $resultJson") }
                    return@execute
                }
                val result = try {
                    gson.fromJson(resultJson, RemoveDuplicatesResult::class.java)
                } catch (e: Exception) {
                    mainThread.post { showError("Duplicate removal returned unexpected data: ${e.message}") }
                    return@execute
                }
                val updatedPortfolioJson = gson.toJson(result.portfolio)

                val saveResult = Bridge.savePortfolio(portfolioPath, updatedPortfolioJson)
                if (isBridgeError(saveResult)) {
                    mainThread.post { showError("Failed to save: $saveResult") }
                    return@execute
                }

                mainThread.post {
                    Toast.makeText(this, "Removed ${result.removed} duplicate transaction(s).", Toast.LENGTH_LONG).show()
                    loadTransactions()
                }
            } catch (e: Exception) {
                mainThread.post { showError("Failed: ${e.message}") }
            }
        }
    }

    private fun exportCsvTo(uri: Uri) {
        try {
            val csv = buildTransactionsCsv()
            contentResolver.openOutputStream(uri)?.use { out ->
                out.write(csv.toByteArray(Charsets.UTF_8))
            }
            Toast.makeText(this, "Exported ${allTransactions.size} transaction(s)", Toast.LENGTH_SHORT).show()
        } catch (e: Exception) {
            Toast.makeText(this, "Export failed: ${e.message}", Toast.LENGTH_LONG).show()
        }
    }

    // Always exports the FULL list in chronological order, regardless of
    // whatever search/sort is currently applied to the on-screen list -
    // this is meant to be a complete record (e.g. for tax filing), not
    // just a dump of whatever happens to be visible at export time. Uses
    // assetRawNameById (the real scheme name), NOT assetNameById (which
    // is nickname-resolved for on-screen display) - a personal nickname
    // like "Midcap A" would be useless, or actively wrong, on a record
    // meant to match against AMC/tax paperwork that only knows the real
    // fund name.
    private fun buildTransactionsCsv(): String {
        val header = listOf("Date", "Fund Name", "ISIN", "Type", "Amount", "Units")
        val rows = allTransactions.sortedBy { it.date }.map { txn ->
            listOf(
                txn.date,
                FundNameFormatter.shorten(assetRawNameById[txn.assetId] ?: ""),
                assetIsinById[txn.assetId] ?: "",
                txn.type,
                txn.amount.toString(),
                txn.units?.toString() ?: ""
            )
        }
        val sb = StringBuilder()
        sb.append(header.joinToString(",") { csvEscape(it) }).append("\n")
        for (row in rows) {
            sb.append(row.joinToString(",") { csvEscape(it) }).append("\n")
        }
        return sb.toString()
    }

    private fun csvEscape(value: String): String {
        return if (value.contains(",") || value.contains("\"") || value.contains("\n")) {
            "\"" + value.replace("\"", "\"\"") + "\""
        } else {
            value
        }
    }

    private fun showError(message: String) {
        Toast.makeText(this, message, Toast.LENGTH_LONG).show()
    }
}

private data class DuplicateGroup(
    val assetId: String,
    val assetName: String,
    val date: String,
    val amount: Double,
    val extraCopies: Int,
    val confidence: String
)

private data class RemoveDuplicatesResult(
    val removed: Int,
    val groups: List<DuplicateGroup>?,
    val portfolio: com.google.gson.JsonObject
)
