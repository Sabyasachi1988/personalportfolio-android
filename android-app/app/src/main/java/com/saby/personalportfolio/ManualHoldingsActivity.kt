package com.saby.personalportfolio

import android.os.Bundle
import android.view.View
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.ledger.bridge.Bridge

class ManualHoldingsActivity : AppCompatActivity() {

    private val gson = Gson()

    private lateinit var accountMemberSpinner: Spinner
    private lateinit var accountNameInput: EditText
    private lateinit var accountCurrencySpinner: Spinner
    private lateinit var accountStatusText: TextView

    private lateinit var assetAccountSpinner: Spinner
    private lateinit var assetNameInput: EditText
    private lateinit var assetSymbolInput: EditText
    private lateinit var assetTypeSpinner: Spinner
    private lateinit var assetStatusText: TextView

    private lateinit var txnAccountSpinner: Spinner
    private lateinit var txnAssetSpinner: Spinner
    private lateinit var txnDateInput: EditText
    private lateinit var txnTypeSpinner: Spinner
    private lateinit var txnAmountInput: EditText
    private lateinit var txnUnitsInput: EditText
    private lateinit var transactionStatusText: TextView

    private var members: List<Member> = emptyList()
    private var accounts: List<AccountSummary> = emptyList()
    private var assets: List<AssetSummary> = emptyList()

    private val currencyOptions = listOf("CAD", "USD", "INR", "EUR", "GBP")
    private val assetTypeOptions = listOf("ETF", "Stock")
    // Label shown to the person -> the store.TransactionType string the
    // bridge actually expects (see AddManualTransaction's allow-list).
    private val txnTypeOptions = listOf(
        "Buy" to "PURCHASE",
        "Sell" to "REDEMPTION",
        "Reinvested Distribution" to "DIVIDEND_REINVEST"
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_manual_holdings)

        accountMemberSpinner = findViewById(R.id.accountMemberSpinner)
        accountNameInput = findViewById(R.id.accountNameInput)
        accountCurrencySpinner = findViewById(R.id.accountCurrencySpinner)
        accountStatusText = findViewById(R.id.accountStatusText)

        assetAccountSpinner = findViewById(R.id.assetAccountSpinner)
        assetNameInput = findViewById(R.id.assetNameInput)
        assetSymbolInput = findViewById(R.id.assetSymbolInput)
        assetTypeSpinner = findViewById(R.id.assetTypeSpinner)
        assetStatusText = findViewById(R.id.assetStatusText)

        txnAccountSpinner = findViewById(R.id.txnAccountSpinner)
        txnAssetSpinner = findViewById(R.id.txnAssetSpinner)
        txnDateInput = findViewById(R.id.txnDateInput)
        txnTypeSpinner = findViewById(R.id.txnTypeSpinner)
        txnAmountInput = findViewById(R.id.txnAmountInput)
        txnUnitsInput = findViewById(R.id.txnUnitsInput)
        transactionStatusText = findViewById(R.id.transactionStatusText)

        accountCurrencySpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, currencyOptions)
        assetTypeSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, assetTypeOptions)
        txnTypeSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, txnTypeOptions.map { it.first })

        txnAccountSpinner.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                populateTxnAssetSpinner()
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }

        findViewById<Button>(R.id.addAccountButton).setOnClickListener { addAccount() }
        findViewById<Button>(R.id.addAssetButton).setOnClickListener { addAsset() }
        findViewById<Button>(R.id.addTransactionButton).setOnClickListener { addTransaction() }
    }

    override fun onResume() {
        super.onResume()
        loadSnapshot()
    }

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    private fun loadSnapshot() {
        val portfolioPath = PortfolioStorage.filePath(this)
        val portfolioJson = Bridge.loadPortfolio(portfolioPath)
        val snapshot: PortfolioManualEntrySnapshot = try {
            gson.fromJson(portfolioJson, PortfolioManualEntrySnapshot::class.java)
        } catch (e: Exception) {
            PortfolioManualEntrySnapshot(emptyList(), emptyList(), emptyList())
        }
        members = snapshot.members.orEmpty()
        accounts = snapshot.accounts.orEmpty()
        assets = snapshot.assets.orEmpty()

        val memberLabels = members.map { it.name }
        accountMemberSpinner.adapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, memberLabels)

        val accountLabels = accounts.map { "${it.name} (${it.currency})" }
        val accountAdapter = ArrayAdapter(this, android.R.layout.simple_spinner_dropdown_item, accountLabels)
        assetAccountSpinner.adapter = accountAdapter
        txnAccountSpinner.adapter = accountAdapter

        populateTxnAssetSpinner()
    }

    private fun populateTxnAssetSpinner() {
        val accountIndex = txnAccountSpinner.selectedItemPosition
        val selectedAccountId = accounts.getOrNull(accountIndex)?.id
        val assetsForAccount = if (selectedAccountId != null) {
            assets.filter { it.accountId == selectedAccountId }
        } else {
            emptyList()
        }
        txnAssetSpinner.adapter = ArrayAdapter(
            this, android.R.layout.simple_spinner_dropdown_item, assetsForAccount.map { it.name }
        )
        // Stash the filtered list for addTransaction() to read back against
        // the selected position, since the spinner itself only carries labels.
        txnAssetSpinner.tag = assetsForAccount
    }

    private fun addAccount() {
        val memberIndex = accountMemberSpinner.selectedItemPosition
        val member = members.getOrNull(memberIndex)
        if (member == null) {
            accountStatusText.text = "Add a member first (Settings > Manage Members)."
            return
        }
        val name = accountNameInput.text.toString().trim()
        if (name.isEmpty()) {
            accountStatusText.text = "Enter an account name first."
            return
        }
        val currency = currencyOptions[accountCurrencySpinner.selectedItemPosition]

        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            accountStatusText.text = "Failed to load portfolio: $currentPortfolioJson"
            return
        }
        val updatedJson = Bridge.addAccount(currentPortfolioJson, member.id, name, currency)
        if (isBridgeError(updatedJson)) {
            accountStatusText.text = updatedJson
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
        if (isBridgeError(saveResult)) {
            accountStatusText.text = "Failed to save: $saveResult"
            return
        }

        accountStatusText.text = ""
        accountNameInput.text.clear()
        Toast.makeText(this, "Added account $name", Toast.LENGTH_SHORT).show()
        loadSnapshot()
    }

    private fun addAsset() {
        val accountIndex = assetAccountSpinner.selectedItemPosition
        val account = accounts.getOrNull(accountIndex)
        if (account == null) {
            assetStatusText.text = "Add an account first."
            return
        }
        val name = assetNameInput.text.toString().trim()
        if (name.isEmpty()) {
            assetStatusText.text = "Enter a fund/ETF name first."
            return
        }
        val symbol = assetSymbolInput.text.toString().trim()
        val type = assetTypeOptions[assetTypeSpinner.selectedItemPosition]

        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            assetStatusText.text = "Failed to load portfolio: $currentPortfolioJson"
            return
        }
        val updatedJson = Bridge.addAsset(currentPortfolioJson, account.id, name, symbol, type)
        if (isBridgeError(updatedJson)) {
            assetStatusText.text = updatedJson
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
        if (isBridgeError(saveResult)) {
            assetStatusText.text = "Failed to save: $saveResult"
            return
        }

        assetStatusText.text = ""
        assetNameInput.text.clear()
        assetSymbolInput.text.clear()
        Toast.makeText(this, "Added asset $name", Toast.LENGTH_SHORT).show()
        loadSnapshot()
    }

    @Suppress("UNCHECKED_CAST")
    private fun addTransaction() {
        val accountIndex = txnAccountSpinner.selectedItemPosition
        val account = accounts.getOrNull(accountIndex)
        if (account == null) {
            transactionStatusText.text = "Add an account first."
            return
        }
        val assetsForAccount = (txnAssetSpinner.tag as? List<AssetSummary>).orEmpty()
        val asset = assetsForAccount.getOrNull(txnAssetSpinner.selectedItemPosition)
        if (asset == null) {
            transactionStatusText.text = "Add an asset under this account first."
            return
        }
        val date = txnDateInput.text.toString().trim()
        if (date.isEmpty()) {
            transactionStatusText.text = "Enter a date (YYYY-MM-DD) first."
            return
        }
        val (_, txnType) = txnTypeOptions[txnTypeSpinner.selectedItemPosition]

        var amount = txnAmountInput.text.toString().toDoubleOrNull()
        var units = txnUnitsInput.text.toString().toDoubleOrNull()
        if (amount == null || units == null) {
            transactionStatusText.text = "Enter valid numbers for amount and units."
            return
        }
        // Enter positive numbers either way - a Sell reduces holdings and
        // is a cash inflow to the investor, so Amount and Units both need
        // to be negative for that case (same sign convention CAS import
        // uses for a Redemption). Buy and Reinvested Distribution keep
        // whatever sign was typed.
        if (txnType == "REDEMPTION") {
            amount = -kotlin.math.abs(amount)
            units = -kotlin.math.abs(units)
        }

        val portfolioPath = PortfolioStorage.filePath(this)
        val currentPortfolioJson = Bridge.loadPortfolio(portfolioPath)
        if (isBridgeError(currentPortfolioJson)) {
            transactionStatusText.text = "Failed to load portfolio: $currentPortfolioJson"
            return
        }
        val updatedJson = Bridge.addManualTransaction(currentPortfolioJson, account.id, asset.id, date, txnType, amount, units)
        if (isBridgeError(updatedJson)) {
            transactionStatusText.text = updatedJson
            return
        }
        val saveResult = Bridge.savePortfolio(portfolioPath, updatedJson)
        if (isBridgeError(saveResult)) {
            transactionStatusText.text = "Failed to save: $saveResult"
            return
        }

        transactionStatusText.text = ""
        txnAmountInput.text.clear()
        txnUnitsInput.text.clear()
        Toast.makeText(this, "Added transaction", Toast.LENGTH_SHORT).show()
    }
}
