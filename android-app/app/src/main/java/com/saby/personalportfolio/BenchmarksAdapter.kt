package com.saby.personalportfolio

import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Button
import android.widget.ImageButton
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView

class BenchmarksAdapter(
    private val benchmarks: List<Benchmark>,
    private val hasHistory: (benchmarkId: String) -> Boolean,
    private val onRefresh: (benchmark: Benchmark, rowHolder: RowHolder) -> Unit,
    private val onDelete: (benchmark: Benchmark) -> Unit
) : RecyclerView.Adapter<BenchmarksAdapter.RowHolder>() {

    class RowHolder(view: View) : RecyclerView.ViewHolder(view) {
        val name: TextView = view.findViewById(R.id.benchmarkRowName)
        val status: TextView = view.findViewById(R.id.benchmarkRowStatus)
        val refreshButton: Button = view.findViewById(R.id.benchmarkRowRefreshButton)
        val deleteButton: ImageButton = view.findViewById(R.id.benchmarkRowDeleteButton)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): RowHolder {
        val view = LayoutInflater.from(parent.context).inflate(R.layout.item_benchmark, parent, false)
        return RowHolder(view)
    }

    override fun onBindViewHolder(holder: RowHolder, position: Int) {
        val benchmark = benchmarks[position]
        holder.name.text = NicknameResolver.resolve(benchmark.name, benchmark.nickname)
        holder.status.text = if (hasHistory(benchmark.id)) {
            benchmark.yahooTicker
        } else {
            "${benchmark.yahooTicker} - no history fetched yet, tap Refresh"
        }
        holder.refreshButton.setOnClickListener { onRefresh(benchmark, holder) }
        holder.deleteButton.setOnClickListener { onDelete(benchmark) }
    }

    override fun getItemCount(): Int = benchmarks.size
}
