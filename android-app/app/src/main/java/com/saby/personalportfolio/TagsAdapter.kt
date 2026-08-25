package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class TagsAdapter(
    private val assets: List<AssetSummary>,
    private val onSave: (assetId: String, tags: List<String>, primaryTag: String, rowHolder: RowHolder) -> Unit
) : RecyclerView.Adapter<TagsAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.tagsAssetName)
        val tagsInput: EditText = view.findViewById(R.id.tagsInput)
        val primaryInput: EditText = view.findViewById(R.id.tagsPrimaryInput)
        val saveButton: Button = view.findViewById(R.id.tagsSaveButton)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_tags, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val asset = assets[position]
        holder.name.text = FundNameFormatter.shorten(asset.name).ifBlank { "(unnamed asset)" }
        holder.tagsInput.setText(asset.tags.joinToString(", "))
        holder.primaryInput.setText(asset.primaryTag)
        holder.saveButton.setOnClickListener {
            // Split on commas, trim each piece, drop anything left blank
            // (e.g. a trailing comma or double comma) - preserves the
            // order the person typed them in, which matters: Asset.Tags'
            // insertion order is exactly what EffectiveTag falls back to
            // when no Primary Tag override is set.
            val tags = holder.tagsInput.text.toString()
                .split(",")
                .map { it.trim() }
                .filter { it.isNotEmpty() }
            val primaryTag = holder.primaryInput.text.toString().trim()
            onSave(asset.id, tags, primaryTag, holder)
        }
    }

    override fun getItemCount(): Int = assets.size
}
