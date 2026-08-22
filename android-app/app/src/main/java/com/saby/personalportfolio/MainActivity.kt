package com.saby.personalportfolio

import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
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

    // Registers the system file picker, restricted to PDFs. This is the
    // standard Android "Storage Access Framework" picker — it works without
    // asking for any storage permission, since the user is choosing the
    // file themselves.
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

        findViewById<android.widget.Button>(R.id.importButton).setOnClickListener {
            pickPdf.launch(arrayOf("application/pdf"))
        }
    }

    private fun importCasFile(uri: Uri) {
        statusText.text = "Reading and parsing PDF…"

        backgroundExecutor.execute {
            try {
                val pdfBytes = contentResolver.openInputStream(uri)?.use { it.readBytes() }
                    ?: throw IllegalStateException("Could not open the selected file")

                // This is the actual call into the tested Go domain logic —
                // everything before this line is just getting bytes off the
                // phone; everything after is just displaying what Go
                // returned.
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
            return
        }

        val staged = result.staged.orEmpty()
        val manualReview = result.manualReview.orEmpty()

        statusText.text = buildString {
            append("Format: ${result.format}\n")
            append("${staged.size} transaction(s) parsed")
            if (manualReview.isNotEmpty()) {
                append(", ${manualReview.size} line(s) need manual review")
            }
        }

        transactionsList.adapter = TransactionAdapter(staged)
    }
}
