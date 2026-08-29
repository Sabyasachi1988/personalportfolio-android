package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class MfapiSearchResultsAdapter(
    private val results: List<MfapiSchemeMatch>,
    private val onPick: (match: MfapiSchemeMatch) -> Unit
) : RecyclerView.Adapter<MfapiSearchResultsAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.searchResultRowName)
        val isin: TextView = view.findViewById(R.id.searchResultRowIsin)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_mfapi_search_result, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val match = results[position]
        holder.name.text = match.name
        holder.isin.text = match.isin
        holder.itemView.setOnClickListener { onPick(match) }
    }

    override fun getItemCount(): Int = results.size
}
