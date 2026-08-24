package com.saby.personalportfolio

import android.content.Context
import java.io.File

object PortfolioStorage {
    // Uses the app's private internal storage (filesDir) - not visible to
    // other apps, survives app restarts, cleared on uninstall. This is
    // exactly the kind of real filesystem path store.Load/store.Save
    // (called from Go via the bridge) expect, since they were written
    // for the desktop app's plain-file storage and were never changed for
    // Android - the same Go code just works given a real path.
    fun filePath(context: Context): String {
        return File(context.filesDir, "portfolio.json").absolutePath
    }

    // Sidecar cache file for historical Progression checkpoints - see
    // finance.ProgressionCache's doc comment. A pure performance cache,
    // not user data: safe to delete at any time (worst case, the next
    // Progression view recomputes everything fresh, same as before this
    // existed), and deliberately NOT included in Settings → Export
    // Backup / Restore From Backup, since it's meaningless without the
    // exact portfolio.json it was computed against and would just be
    // dead weight in a backup file.
    fun progressionCachePath(context: Context): String {
        return File(context.filesDir, "progression-cache.json").absolutePath
    }
}
