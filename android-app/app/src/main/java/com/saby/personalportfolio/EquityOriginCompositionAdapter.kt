package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class EquityOriginCompositionAdapter(
    private val assets: List<AssetSummary>,
    private val existingByAssetId: Map<String, EquityOriginEntry>,
    private val onSave: (assetId: String, indian: Double, international: Double, rowHolder: RowHolder) -> Unit
) : RecyclerView.Adapter<EquityOriginCompositionAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.assetName)
        val indianInput: EditText = view.findViewById(R.id.indianInput)
        val internationalInput: EditText = view.findViewById(R.id.internationalInput)
        val lastEnteredLabel: TextView = view.findViewById(R.id.lastEnteredLabel)
        val saveButton: Button = view.findViewById(R.id.saveRowButton)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_equity_origin_composition, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val asset = assets[position]
        holder.name.text = FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" }

        val existing = existingByAssetId[asset.id]
        holder.indianInput.setText(existing?.indian?.takeIf { it != 0.0 }?.toString() ?: "")
        holder.internationalInput.setText(existing?.international?.takeIf { it != 0.0 }?.toString() ?: "")
        holder.lastEnteredLabel.text = if (existing != null) {
            "Last entered: ${existing.asOf} (${existing.source})"
        } else {
            "No composition entered yet — defaults to 100% Indian in Allocation"
        }

        holder.saveButton.setOnClickListener {
            val indian = holder.indianInput.text.toString().toDoubleOrNull() ?: 0.0
            val international = holder.internationalInput.text.toString().toDoubleOrNull() ?: 0.0
            onSave(asset.id, indian, international, holder)
        }
    }

    override fun getItemCount(): Int = assets.size
}
