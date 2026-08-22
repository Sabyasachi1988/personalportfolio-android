package com.saby.personalportfolio

/**
 * Indian mutual fund scheme names universally end with boilerplate like
 * "- Direct Growth Plan Growth Option" that's identical (or near-
 * identical) across every fund in a portfolio, pushing the actually
 * distinguishing part (the scheme name itself) out of a single-line
 * display before it's ever seen. This strips that boilerplate, not any
 * specific AMC's naming convention.
 */
object FundNameFormatter {

    // Longest/most specific patterns first, so "Growth Plan Growth
    // Option" gets removed as one unit rather than leaving a dangling
    // "Growth Option" behind after only "Growth Plan" matched.
    private val trailingBoilerplate = listOf(
        Regex("""\s*-?\s*direct\s+growth\s+plan\s+growth\s+option\s*$""", RegexOption.IGNORE_CASE),
        Regex("""\s*-?\s*regular\s+growth\s+plan\s+growth\s+option\s*$""", RegexOption.IGNORE_CASE),
        Regex("""\s*-?\s*direct\s+growth\s+plan\s*$""", RegexOption.IGNORE_CASE),
        Regex("""\s*-?\s*regular\s+growth\s+plan\s*$""", RegexOption.IGNORE_CASE),
        Regex("""\s*-?\s*direct\s+plan\s*$""", RegexOption.IGNORE_CASE),
        Regex("""\s*-?\s*regular\s+plan\s*$""", RegexOption.IGNORE_CASE),
        Regex("""\s*-?\s*growth\s+option\s*$""", RegexOption.IGNORE_CASE),
        Regex("""\s*-?\s*idcw\s+option\s*$""", RegexOption.IGNORE_CASE)
    )

    fun shorten(name: String): String {
        var result = name.trim()
        var changed = true
        while (changed) {
            changed = false
            for (pattern in trailingBoilerplate) {
                val stripped = pattern.replace(result, "")
                if (stripped != result) {
                    result = stripped.trim()
                    changed = true
                }
            }
        }
        // Never return something blank - an over-aggressive strip
        // falling back to the original full name is far better than
        // showing nothing at all.
        return result.ifBlank { name.trim() }
    }
}
