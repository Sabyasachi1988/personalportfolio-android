package com.saby.personalportfolio

import android.app.AlertDialog
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.EditText
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.ledger.bridge.Bridge
import java.util.concurrent.Executors

class TransactionsActivity : AppCompatActivity() {

    private val gson = Gson()
    private val backgroundExecutor = Executors.newSingleThreadExecutor()
    private val mainThread = Handler(Looper.getMainLooper())
    private lateinit var recyclerView: RecyclerView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_transactions)

        recyclerView = findViewById(R.id.transactionsRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        BottomNavHelper.setup(this, findViewById(R.id.bottomNav), BottomNavDestination.TRANSACTIONS)
    }

    override fun onResume() {
        super.onResume()
        loadTransactions()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadTransactions() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)

        val snapshot: PortfolioAssetsSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioAssetsSnapshot::class.java)
        } catch (e: Exception) {
            Toast.makeText(this, "Could not read portfolio: ${e.message}", Toast.LENGTH_LONG).show()
            PortfolioAssetsSnapshot(emptyList(), emptyList(), emptyList())
        }

        val assetNameById = snapshot.assets.orEmpty().associate { it.id to it.name }
        // Most recent first - that's what someone checking "did I fat-finger
        // something" wants to see.
        val sorted = snapshot.transactions.orEmpty().sortedByDescending { it.date }

        recyclerView.adapter = TransactionsAdapter(sorted, assetNameById) { txn ->
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
                val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
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
                val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
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

    private fun showError(message: String) {
        Toast.makeText(this, message, Toast.LENGTH_LONG).show()
    }
}
