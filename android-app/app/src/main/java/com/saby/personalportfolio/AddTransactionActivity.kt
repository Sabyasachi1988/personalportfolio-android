package com.saby.personalportfolio

import android.app.DatePickerDialog
import android.os.Bundle
import android.text.Editable
import android.text.TextWatcher
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.ledger.bridge.Bridge
import java.util.Calendar
import java.util.Locale

/**
 * One-at-a-time manual Buy/Sell entry. Fund identity (asset + account) comes
 * from a dropdown of assets already in the portfolio, so only date, amount and
 * NAV are typed. Units are derived as amount / NAV - the SAME formula the Go
 * side (Bridge.addFundTransaction) uses when it stores the transaction; the
 * figure shown here is only a preview.
 */
class AddTransactionActivity : AppCompatActivity() {

    private val gson = Gson()

    private lateinit var fundSpinner: Spinner
    private lateinit var typeSpinner: Spinner
    private lateinit var dateInput: EditText
    private lateinit var amountInput: EditText
    private lateinit var navInput: EditText
    private lateinit var unitsPreview: TextView
    private lateinit var statusText: TextView

    private var assets: List<AssetSummary> = emptyList()
    private var accountsById: Map<String, AccountSummary> = emptyMap()
    private var membersById: Map<String, Member> = emptyMap()

    private val typeOptions = listOf("Buy" to "PURCHASE", "Sell" to "REDEMPTION")

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_add_transaction)
        title = "Add Transaction"

        fundSpinner = findViewById(R.id.addTxnFundSpinner)
        typeSpinner = findViewById(R.id.addTxnTypeSpinner)
        dateInput = findViewById(R.id.addTxnDateInput)
        amountInput = findViewById(R.id.addTxnAmountInput)
        navInput = findViewById(R.id.addTxnNavInput)
        unitsPreview = findViewById(R.id.addTxnUnitsPreview)
        statusText = findViewById(R.id.addTxnStatusText)

        typeSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, typeOptions.map { it.first })

        findViewById<Button>(R.id.addTxnPickDateButton).setOnClickListener { showDatePicker() }
        findViewById<Button>(R.id.addTxnSaveButton).setOnClickListener { save() }

        val watcher = object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}
            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {}
            override fun afterTextChanged(s: Editable?) = updateUnitsPreview()
        }
        amountInput.addTextChangedListener(watcher)
        navInput.addTextChangedListener(watcher)

        loadFunds()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadFunds() {
        val portfolioJson = PortfolioLoadCache.load(PortfolioStorage.filePath(this))
        val snapshot: PortfolioManualEntrySnapshot = try {
            gson.fromJson(portfolioJson, PortfolioManualEntrySnapshot::class.java)
        } catch (e: Exception) {
            PortfolioManualEntrySnapshot(emptyList(), emptyList(), emptyList())
        }
        membersById = snapshot.members.orEmpty().associateBy { it.id }
        accountsById = snapshot.accounts.orEmpty().associateBy { it.id }
        // Sorted by display name so a fund is easy to find in a long list.
        assets = snapshot.assets.orEmpty().sortedBy { displayName(it).lowercase(Locale.ROOT) }

        // The same fund name can exist under several members/accounts, so the
        // member is appended whenever more than one member exists - otherwise
        // two identical rows would be indistinguishable.
        val showMember = membersById.size > 1
        val labels = assets.map { asset ->
            val owner = accountsById[asset.accountId]?.memberId?.let { membersById[it]?.name }
            if (showMember && owner != null) "${displayName(asset)}  ·  $owner" else displayName(asset)
        }
        fundSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, labels)
        if (assets.isEmpty()) statusText.text = "No funds in the portfolio yet - import a statement first."
    }

    private fun displayName(a: AssetSummary): String = NicknameResolver.resolve(a.name, a.nickname)

    private fun showDatePicker() {
        val cal = Calendar.getInstance()
        val existing = dateInput.text.toString().trim().split("-")
        if (existing.size == 3) {
            val y = existing[0].toIntOrNull()
            val m = existing[1].toIntOrNull()
            val d = existing[2].toIntOrNull()
            if (y != null && m != null && d != null) cal.set(y, m - 1, d)
        }
        DatePickerDialog(this, { _, y, m, d ->
            dateInput.setText(String.format(Locale.US, "%04d-%02d-%02d", y, m + 1, d))
        }, cal.get(Calendar.YEAR), cal.get(Calendar.MONTH), cal.get(Calendar.DAY_OF_MONTH)).show()
    }

    private fun updateUnitsPreview() {
        val amount = amountInput.text.toString().toDoubleOrNull()
        val nav = navInput.text.toString().toDoubleOrNull()
        unitsPreview.text = if (amount != null && nav != null && nav > 0) {
            String.format(Locale.US, "Units: %.4f", amount / nav)
        } else {
            "Units: —"
        }
    }

    private fun save() {
        val asset = assets.getOrNull(fundSpinner.selectedItemPosition)
        if (asset == null) {
            statusText.text = "Pick a fund first."
            return
        }
        val date = dateInput.text.toString().trim()
        if (!Regex("""\d{4}-\d{2}-\d{2}""").matches(date)) {
            statusText.text = "Enter the date as YYYY-MM-DD (or tap Pick)."
            return
        }
        val amount = amountInput.text.toString().toDoubleOrNull()
        val nav = navInput.text.toString().toDoubleOrNull()
        if (amount == null || amount <= 0 || nav == null || nav <= 0) {
            statusText.text = "Enter an amount and a NAV greater than zero."
            return
        }
        val txnType = typeOptions[typeSpinner.selectedItemPosition].second

        val path = PortfolioStorage.filePath(this)
        val current = PortfolioLoadCache.load(path)
        if (isBridgeError(current)) {
            statusText.text = "Failed to load portfolio: $current"
            return
        }
        val updated = Bridge.addFundTransaction(current, asset.accountId, asset.id, date, txnType, amount, nav)
        if (isBridgeError(updated)) {
            statusText.text = updated
            return
        }
        val saveResult = Bridge.savePortfolio(path, updated)
        if (isBridgeError(saveResult)) {
            statusText.text = "Failed to save: $saveResult"
            return
        }

        statusText.text = ""
        amountInput.text.clear()
        navInput.text.clear()
        Toast.makeText(this, "Saved ${typeOptions[typeSpinner.selectedItemPosition].first} for ${displayName(asset)}", Toast.LENGTH_SHORT).show()
    }
}
