package com.saby.personalportfolio

import android.app.Activity
import android.os.Handler
import android.os.Looper
import android.widget.Toast
import com.google.gson.Gson
import com.ledger.bridge.Bridge

/** Adds screener funds to the tracked ("Additional Funds") list and pulls their NAV + cap/sector history. */
object ScreenerTrack {
    private val gson = Gson()

    fun trackAll(activity: Activity, funds: List<ScreenerRow>, onDone: () -> Unit = {}) {
        val main = Handler(Looper.getMainLooper())
        Toast.makeText(activity, "Adding ${funds.size} fund(s)…", Toast.LENGTH_SHORT).show()
        Thread {
            val path = PortfolioStorage.filePath(activity)
            var added = 0
            var already = 0
            var failed = 0
            val ids = ArrayList<Pair<String, String>>()
            for (row in funds) {
                val json = PortfolioLoadCache.load(path)
                val snap: AdditionalFundsSnapshot? = try { gson.fromJson(json, AdditionalFundsSnapshot::class.java) } catch (e: Exception) { null }
                val existing = snap?.assets.orEmpty().firstOrNull { row.isin.isNotBlank() && it.isin.equals(row.isin, ignoreCase = true) }
                if (existing != null) { already++; ids.add(existing.id to existing.name); continue }
                val after = Bridge.addTrackedFund(json, row.name, row.isin)
                if (after.trimStart().startsWith("{\"error\"") || Bridge.savePortfolio(path, after).trimStart().startsWith("{\"error\"")) { failed++; continue }
                added++
                val a = try { gson.fromJson(after, AdditionalFundsSnapshot::class.java)?.assets.orEmpty().lastOrNull { it.isin.equals(row.isin, true) } } catch (e: Exception) { null }
                if (a != null) ids.add(a.id to a.name)
            }
            for ((id, _) in ids) {
                try { Bridge.pullFundAtPath(path, id, false) } catch (e: Exception) { /* retried by auto-sync / Refresh */ }
            }
            main.post {
                val parts = listOfNotNull(
                    if (added > 0) "$added added" else null,
                    if (already > 0) "$already already tracked" else null,
                    if (failed > 0) "$failed failed" else null
                )
                Toast.makeText(activity, parts.joinToString(" · ").ifEmpty { "Nothing to add" } + " - history updated", Toast.LENGTH_LONG).show()
                onDone()
            }
        }.start()
    }
}
