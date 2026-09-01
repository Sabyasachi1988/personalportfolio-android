package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.ImageButton
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class AdditionalFundsAdapter(
    private val funds: List<AssetSummary>,
    private val hasHistory: (assetId: String) -> Boolean,
    private val onRefresh: (fund: AssetSummary, rowHolder: RowHolder) -> Unit,
    private val onDelete: (fund: AssetSummary) -> Unit
) : RecyclerView.Adapter<AdditionalFundsAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.additionalFundRowName)
        val status: TextView = view.findViewById(R.id.additionalFundRowStatus)
        val refreshButton: Button = view.findViewById(R.id.additionalFundRowRefreshButton)
        val deleteButton: ImageButton = view.findViewById(R.id.additionalFundRowDeleteButton)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_additional_fund, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val fund = funds[position]
        // Confirmed real gap: every other screen (Holdings, Returns,
        // Compare, fund detail) resolves a set nickname per
        // NicknameResolver's own doc comment - this list was reading
        // the raw default name only, so a fund the person had already
        // nicknamed in Manage Names still showed its original name
        // here.
        val displayName = NicknameResolver.resolve(fund.name, fund.nickname)
        holder.name.text = displayName
        holder.status.text = if (hasHistory(fund.id)) {
            fund.isin
        } else {
            "${fund.isin} - no history fetched yet, tap Refresh"
        }
        holder.refreshButton.setOnClickListener { onRefresh(fund, holder) }
        holder.deleteButton.setOnClickListener { onDelete(fund) }
    }

    override fun getItemCount(): Int = funds.size
}
