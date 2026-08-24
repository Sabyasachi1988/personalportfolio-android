package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class FundGroupsAdapter(
    private val assets: List<AssetSummary>,
    private val onSave: (assetId: String, label: String, rowHolder: RowHolder) -> Unit
) : RecyclerView.Adapter<FundGroupsAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.fundGroupAssetName)
        val labelInput: EditText = view.findViewById(R.id.fundGroupLabelInput)
        val saveButton: Button = view.findViewById(R.id.fundGroupSaveButton)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_fund_group, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val asset = assets[position]
        holder.name.text = FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" }
        holder.labelInput.setText(asset.groupLabel)
        holder.saveButton.setOnClickListener {
            onSave(asset.id, holder.labelInput.text.toString().trim(), holder)
        }
    }

    override fun getItemCount(): Int = assets.size
}
