package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class TransactionsAdapter(
    private val transactions: List<StoredTransactionEntry>,
    private val assetNameById: Map<String, String>,
    private val onClick: (StoredTransactionEntry) -> Unit
) : RecyclerView.Adapter<TransactionsAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val assetName: TextView = view.findViewById(R.id.txnAssetName)
        val detail: TextView = view.findViewById(R.id.txnDetail)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_transaction_row, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val txn = transactions[position]
        holder.assetName.text = assetNameById[txn.assetId]?.let { FundNameFormatter.shorten(it) } ?: "(unknown asset)"
        holder.detail.text = String.format(
            Locale.getDefault(),
            "%s | %s | %s | %s units",
            txn.date,
            txn.type,
            IndianCurrencyFormatter.format(txn.amount),
            unitsDisplay(txn.units)
        )
        holder.itemView.setOnClickListener { onClick(txn) }
    }

    // Same reasoning as HoldingsAdapter's unitsDisplay: a transaction's
    // unit count, combined with the fund's publicly-known NAV on that
    // date, reveals the masked rupee amount right next to it - a FIXED
    // placeholder here too, not one sized to the real digit count.
    private fun unitsDisplay(units: Double?): String {
        if (units == null) return "—"
        if (IncognitoMode.isEnabled) return "•••"
        return String.format(Locale.getDefault(), "%.3f", units)
    }

    override fun getItemCount(): Int = transactions.size
}
