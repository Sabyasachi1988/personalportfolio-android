package com.saby.personalportfolio

import android.graphics.Color
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class HoldingsAdapter(private val holdings: List<Holding>) :
    RecyclerView.Adapter<HoldingsAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.holdingName)
        val valueLine: TextView = view.findViewById(R.id.holdingValueLine)
        val gainLine: TextView = view.findViewById(R.id.holdingGainLine)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_holding, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val h = holdings[position]

        holder.name.text = h.assetName.ifBlank { "(unnamed asset)" }
        holder.valueLine.text = String.format(
            Locale.getDefault(),
            "Units: %.3f | Invested: ₹%.2f",
            h.unitsHeld,
            h.netInvested
        )

        if (h.hasPrice) {
            val xirrPart = if (h.hasXirr) String.format(Locale.getDefault(), " | XIRR: %.2f%%", h.xirr) else ""
            holder.gainLine.text = String.format(
                Locale.getDefault(),
                "Value: ₹%.2f | Gain: ₹%.2f (%.2f%%)%s",
                h.currentValue,
                h.gain,
                h.gainPercent,
                xirrPart
            )
            holder.gainLine.setTextColor(
                if (h.gain >= 0) Color.parseColor("#2E7D32") else Color.parseColor("#C62828")
            )
        } else {
            holder.gainLine.text = "No current price available"
            holder.gainLine.setTextColor(Color.GRAY)
        }
    }

    override fun getItemCount(): Int = holdings.size
}
