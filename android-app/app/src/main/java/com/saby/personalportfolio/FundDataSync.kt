package com.saby.personalportfolio

import android.content.Context
import com.ledger.bridge.Bridge
import java.util.concurrent.Executors

/**
 * Keeps each fund's published cap/sector history current with no action from the person. Runs when the app comes to
 * the foreground, at most once every 6 hours. The check is one tiny request (the published manifest version); only
 * funds whose stored version is behind are downloaded, and only their data is merged (append-only, guarded - see
 * store.MergeFundMonths). The Go side reloads the portfolio file fresh before saving, so it never overwrites an edit
 * made while it was fetching. Failures are silent and retried on the next foreground.
 */
object FundDataSync {
    private const val PREFS = "fund_data_sync"
    private const val KEY_LAST = "last_run_ms"
    private const val MIN_INTERVAL_MS = 6L * 60 * 60 * 1000
    private val executor = Executors.newSingleThreadExecutor()
    @Volatile private var running = false

    fun maybeRun(context: Context) {
        val app = context.applicationContext
        val prefs = app.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
        val now = System.currentTimeMillis()
        if (running || now - prefs.getLong(KEY_LAST, 0L) < MIN_INTERVAL_MS) return
        running = true
        executor.execute {
            try {
                val res = Bridge.syncFundHistoriesAtPath(PortfolioStorage.filePath(app))
                // Only a successful check (changed or already-current) starts the 6h wait; an error retries next time.
                if (!res.trimStart().startsWith("{\"error\"")) {
                    prefs.edit().putLong(KEY_LAST, System.currentTimeMillis()).apply()
                }
            } catch (e: Exception) {
                // silent; retried on the next foreground
            } finally {
                running = false
            }
        }
    }
}
