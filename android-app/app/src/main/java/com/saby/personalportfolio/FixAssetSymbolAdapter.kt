package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.Spinner
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class FixAssetSymbolAdapter(
    private val assets: List<AssetSummary>,
    private val onSave: (assetId: String, symbol: String, type: String, rowHolder: RowHolder) -> Unit
) : RecyclerView.Adapter<FixAssetSymbolAdapter.RowHolder>() {

    // Matches the values store.Asset.Type actually understands - see
    // that field's doc comment in internal/store/portfolio.go.
    private val typeOptions = listOf("MutualFund", "Stock", "ETF")

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.fixAssetName)
        val isin: TextView = view.findViewById(R.id.fixAssetIsin)
        val typeSpinner: Spinner = view.findViewById(R.id.fixAssetTypeSpinner)
        val symbolInput: EditText = view.findViewById(R.id.fixAssetSymbolInput)
        val saveButton: Button = view.findViewById(R.id.fixAssetSaveButton)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_fix_asset_symbol, parent, false)
        val holder = RowHolder(view)
        holder.typeSpinner.adapter = ArrayAdapter(parent.context, android.R.layout.simple_spinner_dropdown_item, typeOptions)
        return holder
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val asset = assets[position]
        holder.name.text = FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" }
        holder.isin.text = if (asset.isin.isNotBlank()) "ISIN: ${asset.isin}" else "No ISIN"
        holder.symbolInput.setText(asset.symbol)

        val typeIndex = typeOptions.indexOf(asset.type).coerceAtLeast(0)
        holder.typeSpinner.setSelection(typeIndex)

        holder.saveButton.setOnClickListener {
            val symbol = holder.symbolInput.text.toString().trim()
            val type = typeOptions.getOrElse(holder.typeSpinner.selectedItemPosition) { "Stock" }
            onSave(asset.id, symbol, type, holder)
        }
    }

    override fun getItemCount(): Int = assets.size
}
