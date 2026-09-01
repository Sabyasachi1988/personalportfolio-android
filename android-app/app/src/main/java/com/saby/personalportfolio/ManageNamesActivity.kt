package com.saby.personalportfolio

import android.app.AlertDialog
import android.os.Bundle
import android.text.Editable
import android.text.TextWatcher
import android.view.View
import android.widget.EditText
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.recyclerview.widget.LinearLayoutManager
import androidx.recyclerview.widget.RecyclerView
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge
import java.util.Locale

/**
 * Dedicated screen to give any fund or tracked index a short personal
 * name - see NicknameResolver's doc comment for exactly where that name
 * then shows up. This is the ONE place renaming happens; every other
 * screen either already reads the resolved name straight off a bridge
 * call (Holdings, Returns, Compare, fund detail - no per-screen rename
 * UI needed there) or, for the few screens that read the raw portfolio
 * directly (Transactions, Progression's fund picker, Manage Benchmarks),
 * resolves it via NicknameResolver rather than offering its own
 * separate rename entry point.
 */
class ManageNamesActivity : AppCompatActivity() {

    private val gson = Gson()
    private lateinit var recyclerView: RecyclerView
    private lateinit var searchBox: EditText
    private lateinit var emptyState: View

    private var allEntries: List<NameListEntry> = emptyList()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_manage_names)

        recyclerView = findViewById(R.id.manageNamesRecyclerView)
        recyclerView.layoutManager = LinearLayoutManager(this)
        searchBox = findViewById(R.id.manageNamesSearch)
        emptyState = findViewById(R.id.manageNamesEmptyState)

        searchBox.addTextChangedListener(object : TextWatcher {
            override fun afterTextChanged(s: Editable?) = renderFiltered(s?.toString().orEmpty())
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
        })

        loadEntries()
    }

    override fun onResume() {
        super.onResume()
        // A rename dialog can be reopened after Save without leaving
        // this Activity, but reloading on every resume (not just
        // onCreate) also means coming back from another screen (e.g.
        // after adding a new benchmark elsewhere) shows it here too.
        loadEntries()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadEntries() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val resultJson = Bridge.computeNameList(portfolioJson)
        if (isBridgeError(resultJson)) {
            (emptyState as android.widget.TextView).text = "Could not load funds/indices: $resultJson"
            emptyState.visibility = View.VISIBLE
            return
        }
        val entryType = object : TypeToken<List<NameListEntry>>() {}.type
        allEntries = try {
            gson.fromJson(resultJson, entryType) ?: emptyList()
        } catch (e: Exception) {
            emptyList()
        }
        // Funds first (the common case), then indices, each
        // alphabetically by default name - a stable, predictable order
        // rather than whatever order they happen to sit in the
        // portfolio file.
        allEntries = allEntries.sortedWith(compareBy({ it.isBenchmark }, { it.name.lowercase(Locale.getDefault()) }))
        renderFiltered(searchBox.text?.toString().orEmpty())
    }

    private fun renderFiltered(query: String) {
        val filtered = if (query.isBlank()) {
            allEntries
        } else {
            allEntries.filter {
                it.name.contains(query, ignoreCase = true) || it.nickname.contains(query, ignoreCase = true)
            }
        }
        if (allEntries.isEmpty()) {
            (emptyState as android.widget.TextView).text =
                "No funds or indices yet. Import a CAS statement, or add a benchmark from Returns → gear icon."
            emptyState.visibility = View.VISIBLE
            recyclerView.visibility = View.GONE
        } else if (filtered.isEmpty()) {
            (emptyState as android.widget.TextView).text = "No matches for \"$query\"."
            emptyState.visibility = View.VISIBLE
            recyclerView.visibility = View.GONE
        } else {
            emptyState.visibility = View.GONE
            recyclerView.visibility = View.VISIBLE
            recyclerView.adapter = ManageNamesAdapter(filtered, { entry -> showRenameDialog(entry) }, { entry, usable -> toggleUsableAsBenchmark(entry, usable) })
        }
    }

    private fun showRenameDialog(entry: NameListEntry) {
        val input = EditText(this).apply {
            setText(entry.nickname)
            hint = entry.name
            setSelection(text.length)
        }
        val paddingPx = (20 * resources.displayMetrics.density).toInt()
        input.setPadding(paddingPx, paddingPx / 2, paddingPx, 0)

        val dialog = AlertDialog.Builder(this)
            .setTitle(entry.name)
            .setView(input)
            .setPositiveButton("Save") { _, _ -> saveNickname(entry, input.text?.toString().orEmpty()) }
            .setNeutralButton("Clear nickname") { _, _ -> saveNickname(entry, "") }
            .setNegativeButton("Cancel", null)
            .create()
        dialog.show()
    }

    private fun toggleUsableAsBenchmark(entry: NameListEntry, usable: Boolean) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterSet = Bridge.setUsableAsBenchmark(portfolioJson, entry.seriesId, usable)
        if (isBridgeError(afterSet)) {
            Toast.makeText(this, "Failed to save: $afterSet", Toast.LENGTH_LONG).show()
            loadEntries() // revert the checkbox to the actual saved state
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, afterSet)
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            loadEntries()
            return
        }
        loadEntries()
    }

    private fun saveNickname(entry: NameListEntry, nickname: String) {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = PortfolioLoadCache.load(portfolioPath)
        val afterSet = Bridge.setNickname(portfolioJson, entry.seriesId, nickname.trim())
        if (isBridgeError(afterSet)) {
            Toast.makeText(this, "Failed to save: $afterSet", Toast.LENGTH_LONG).show()
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, afterSet)
        if (isBridgeError(saveResult)) {
            Toast.makeText(this, "Failed to save: $saveResult", Toast.LENGTH_LONG).show()
            return
        }
        loadEntries()
    }
}
