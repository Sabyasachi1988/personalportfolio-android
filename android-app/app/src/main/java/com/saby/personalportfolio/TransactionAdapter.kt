package com.saby.personalportfolio

import android.graphics.Color
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class TransactionAdapter(private val rows: List<StagedRow>) :
    RecyclerView.Adapter<TransactionAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val scheme: TextView = view.findViewById(R.id.rowScheme)
        val detail: TextView = view.findViewById(R.id.rowDetail)
        val status: TextView = view.findViewById(R.id.rowStatus)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_transaction, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val row = rows[position]
        val txn = row.txn

        holder.scheme.text = FundNameFormatter.shorten(txn.scheme).ifBlank { "(unnamed scheme)" }
        holder.detail.text = String.format(
            Locale.getDefault(),
            "%s | %s | %s | %s",
            txn.date,
            txn.type,
            IndianCurrencyFormatter.format(txn.amount),
            txn.folio
        )
        holder.status.text = "Status: ${row.status}  (page ${row.sourcePage})"
        holder.status.setTextColor(
            when (row.status) {
                "NEW" -> Color.parseColor("#2E7D32")       // green
                "DUPLICATE" -> Color.parseColor("#9E9E9E") // grey
                "UNMATCHED" -> Color.parseColor("#C62828") // red
                else -> Color.DKGRAY
            }
        )
    }

    override fun getItemCount(): Int = rows.size
}
