package com.saby.personalportfolio

import java.io.File

/**
 * Caches Bridge.loadPortfolio's result, keyed on the portfolio file's
 * own last-modified time - NOT a fixed time-based expiry. This matters:
 * a pure "cache for N seconds" would risk serving stale data right after
 * a save (a very common pattern in this app: load → mutate → save →
 * immediately reload to show the result), which would be a far worse
 * bug than the lag this exists to fix. Checking the file's own mtime on
 * every call instead means the cache is invalidated automatically and
 * correctly by ANY successful save, from ANY screen, the moment it
 * happens - because a save always writes the file, which always bumps
 * its mtime - without needing to find and wrap every one of this app's
 * many Bridge.savePortfolio call sites individually.
 *
 * Why this exists at all: portfolio.json now carries full daily NAV/
 * index history for every fund and benchmark (years of data, thousands
 * of records each) on top of what it used to hold. Every screen was
 * already calling Bridge.loadPortfolio fresh on every load - cheap when
 * the file was small, but with that much more data to read, unmarshal
 * in Go, marshal back out, and cross the JNI boundary with, repeating
 * that full round-trip on every single bottom-nav tab switch became
 * genuinely noticeable (reported as ~0.5-1s lag specifically on tab
 * switches, not on actions within an already-loaded screen - exactly
 * what you'd expect from this cost being paid fresh on every new
 * Activity's load rather than being reused).
 */
object PortfolioLoadCache {
    private var cachedPath: String? = null
    private var cachedJson: String? = null
    private var cachedFileModifiedMillis: Long = -1

    @Synchronized
    fun load(path: String): String {
        val file = File(path)
        val fileModified = if (file.exists()) file.lastModified() else -1L

        if (cachedJson != null && cachedPath == path && fileModified == cachedFileModifiedMillis) {
            return cachedJson!!
        }

        val json = com.ledger.bridge.Bridge.loadPortfolio(path)
        cachedPath = path
        cachedJson = json
        cachedFileModifiedMillis = fileModified
        return json
    }

    /**
     * Explicit invalidation, for the rare case a caller mutates the file
     * through something other than Bridge.savePortfolio (or wants to be
     * extra sure) - the mtime check above already handles the normal
     * save-then-reload case on its own, so this is a belt-and-suspenders
     * escape hatch, not something most callers need to reach for.
     */
    @Synchronized
    fun invalidate() {
        cachedJson = null
    }
}
