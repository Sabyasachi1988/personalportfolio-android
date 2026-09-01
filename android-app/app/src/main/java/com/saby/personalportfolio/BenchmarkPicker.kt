package com.saby.personalportfolio

import android.app.AlertDialog
import androidx.appcompat.app.AppCompatActivity
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.ledger.bridge.Bridge

/**
 * Picking and persisting a fund's benchmark (Asset.PreferredBenchmarkID
 * on the Go side) - extracted out of ReturnsDetailActivity so both that
 * screen's own "Compare against" button AND the Compare screen's
 * quantitative table tab (which needs the exact same picker per fund
 * column) share one implementation instead of two copies drifting apart.
 * See Asset.PreferredBenchmarkID's Go doc comment and
 * Asset.UsableAsBenchmark's Go doc comment for why this offers BOTH
 * already-added Benchmarks AND tracked funds flagged usable-as-
 * benchmark (creating the underlying Benchmark on the spot via
 * AddBenchmarkFromAsset - a local copy, no network call - if a tracked
 * fund is picked).
 */
object BenchmarkPicker {

    private val gson = Gson()

    private fun isBridgeError(json: String): Boolean = json.trimStart().startsWith("{\"error\"")

    /**
     * @param currentBenchmarkId empty means "no manual override, auto-pick" - used only to
     *   pre-select the right radio option, since the picker itself always re-reads
     *   Asset.PreferredBenchmarkID indirectly via [onChanged]'s caller, not from this parameter.
     * @param onChanged called with the updated, already-saved portfolio JSON after a pick -
     *   the caller is responsible for reloading whatever it displays from this new JSON.
     *   Never called if the person cancels or there was nothing to pick from.
     */
    fun show(
        activity: AppCompatActivity,
        seriesId: String,
        portfolioJson: String,
        currentBenchmarkId: String,
        onChanged: (updatedPortfolioJson: String) -> Unit
    ) {
        val snapshot: PortfolioBenchmarksSnapshot = try {
            gson.fromJson(portfolioJson, PortfolioBenchmarksSnapshot::class.java)
        } catch (e: Exception) {
            null
        } ?: PortfolioBenchmarksSnapshot(emptyList(), emptyList())
        val benchmarks = snapshot.benchmarks ?: emptyList()

        val existingProxyISINs = benchmarks.filter { it.proxyFundISIN.isNotEmpty() }.map { it.proxyFundISIN }.toSet()
        val nameListJson = Bridge.computeNameList(portfolioJson)
        val trackedFunds: List<NameListEntry> = if (!isBridgeError(nameListJson)) {
            val entryType = object : TypeToken<List<NameListEntry>>() {}.type
            try {
                gson.fromJson<List<NameListEntry>>(nameListJson, entryType)
                    ?.filter { !it.isBenchmark && it.usableAsBenchmark && it.isin.isNotBlank() && it.isin !in existingProxyISINs && it.seriesId != seriesId }
                    ?: emptyList()
            } catch (e: Exception) {
                emptyList()
            }
        } else {
            emptyList()
        }
        if (benchmarks.isEmpty() && trackedFunds.isEmpty()) return

        val trackedLabels = trackedFunds.map { "[Fund] " + NicknameResolver.resolve(it.name, it.nickname) }
        val labels = (listOf("Auto-pick (recommended)") + benchmarks.map { it.name } + trackedLabels).toTypedArray()
        val currentIndex = if (currentBenchmarkId.isEmpty()) {
            0
        } else {
            benchmarks.indexOfFirst { it.id == currentBenchmarkId }.let { if (it < 0) 0 else it + 1 }
        }
        AlertDialog.Builder(activity)
            .setTitle("Compare against")
            .setSingleChoiceItems(labels, currentIndex) { dialog, which ->
                dialog.dismiss()
                when {
                    which == 0 -> persistPreferredBenchmark(activity, seriesId, portfolioJson, "", onChanged)
                    which - 1 < benchmarks.size -> persistPreferredBenchmark(activity, seriesId, portfolioJson, benchmarks[which - 1].id, onChanged)
                    else -> addBenchmarkFromAssetAndSelect(activity, seriesId, portfolioJson, trackedFunds[which - 1 - benchmarks.size], onChanged)
                }
            }
            .setNegativeButton("Cancel", null)
            .show()
    }

    private fun persistPreferredBenchmark(
        activity: AppCompatActivity,
        seriesId: String,
        portfolioJson: String,
        benchmarkId: String,
        onChanged: (String) -> Unit
    ) {
        val portfolioPath = PortfolioStorage.filePath(activity)
        val afterSet = Bridge.setPreferredBenchmark(portfolioJson, seriesId, benchmarkId)
        if (isBridgeError(afterSet)) return
        val saveResult = Bridge.savePortfolio(portfolioPath, afterSet)
        if (isBridgeError(saveResult)) return
        onChanged(afterSet)
    }

    private fun addBenchmarkFromAssetAndSelect(
        activity: AppCompatActivity,
        seriesId: String,
        portfolioJson: String,
        entry: NameListEntry,
        onChanged: (String) -> Unit
    ) {
        val portfolioPath = PortfolioStorage.filePath(activity)
        val afterAdd = Bridge.addBenchmarkFromAsset(portfolioJson, entry.seriesId)
        if (isBridgeError(afterAdd)) return
        val updatedSnapshot: PortfolioBenchmarksSnapshot = try {
            gson.fromJson(afterAdd, PortfolioBenchmarksSnapshot::class.java)
        } catch (e: Exception) {
            null
        } ?: return
        val newBenchmark = updatedSnapshot.benchmarks.orEmpty().lastOrNull { it.proxyFundISIN == entry.isin } ?: return
        val saveResult = Bridge.savePortfolio(portfolioPath, afterAdd)
        if (isBridgeError(saveResult)) return
        persistPreferredBenchmark(activity, seriesId, afterAdd, newBenchmark.id, onChanged)
    }
}
