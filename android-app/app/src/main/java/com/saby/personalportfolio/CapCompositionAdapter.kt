package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class CapCompositionAdapter(
    private val assets: List<AssetSummary>,
    private val existingByAssetId: Map<String, CapCompositionEntry>,
    private val onSave: (assetId: String, large: Double, mid: Double, small: Double, cash: Double, rowHolder: RowHolder) -> Unit,
    private val onFetchFromEtMoney: (assetId: String, url: String, rowHolder: RowHolder) -> Unit,
    private val onEditAssetClass: (asset: AssetSummary) -> Unit
) : RecyclerView.Adapter<CapCompositionAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.assetName)
        val assetClassRow: TextView = view.findViewById(R.id.assetClassRow)
        val etMoneyUrlInput: EditText = view.findViewById(R.id.etMoneyUrlInput)
        val fetchButton: Button = view.findViewById(R.id.fetchEtMoneyButton)
        val etMoneyStatusLabel: TextView = view.findViewById(R.id.etMoneyStatusLabel)
        val largeInput: EditText = view.findViewById(R.id.largeInput)
        val midInput: EditText = view.findViewById(R.id.midInput)
        val smallInput: EditText = view.findViewById(R.id.smallInput)
        val cashInput: EditText = view.findViewById(R.id.cashInput)
        val lastEnteredLabel: TextView = view.findViewById(R.id.lastEnteredLabel)
        val saveButton: Button = view.findViewById(R.id.saveRowButton)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_cap_composition, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val asset = assets[position]
        holder.name.text = FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" }
        holder.assetClassRow.text = if (asset.assetClassOverride.isNotBlank()) {
            "Class override: ${asset.assetClassOverride} — tap to change"
        } else {
            val amfi = asset.assetClass.ifBlank { "none" }
            "Class: automatic (AMFI category: $amfi) — tap to override"
        }
        holder.assetClassRow.setOnClickListener { onEditAssetClass(asset) }
        holder.etMoneyUrlInput.setText(asset.etMoneyUrl)
        holder.etMoneyStatusLabel.visibility = View.GONE

        val existing = existingByAssetId[asset.id]
        holder.largeInput.setText(existing?.large?.takeIf { it != 0.0 }?.toString() ?: "")
        holder.midInput.setText(existing?.mid?.takeIf { it != 0.0 }?.toString() ?: "")
        holder.smallInput.setText(existing?.small?.takeIf { it != 0.0 }?.toString() ?: "")
        holder.cashInput.setText(existing?.cash?.takeIf { it != 0.0 }?.toString() ?: "")
        holder.lastEnteredLabel.text = if (existing != null) {
            "Last entered: ${existing.asOf} (${existing.source})"
        } else {
            "No composition entered yet — using name-based guess in Allocation"
        }

        holder.saveButton.setOnClickListener {
            val large = holder.largeInput.text.toString().toDoubleOrNull() ?: 0.0
            val mid = holder.midInput.text.toString().toDoubleOrNull() ?: 0.0
            val small = holder.smallInput.text.toString().toDoubleOrNull() ?: 0.0
            val cash = holder.cashInput.text.toString().toDoubleOrNull() ?: 0.0
            onSave(asset.id, large, mid, small, cash, holder)
        }

        holder.fetchButton.setOnClickListener {
            val url = holder.etMoneyUrlInput.text.toString().trim()
            if (url.isBlank()) {
                holder.etMoneyStatusLabel.visibility = View.VISIBLE
                holder.etMoneyStatusLabel.text = "Paste an ETMoney fund page URL first"
                return@setOnClickListener
            }
            onFetchFromEtMoney(asset.id, url, holder)
        }
    }

    override fun getItemCount(): Int = assets.size
}
