package com.saby.personalportfolio

/**
 * Resolves what to actually SHOW for a fund/index name, given its real
 * name and an optional personal nickname (see store.Asset.Nickname /
 * store.Benchmark.Nickname's doc comments on the Go side).
 *
 * Most screens don't need this directly - wherever a bridge function
 * already computes and returns a single Name/AssetName field (Holdings,
 * Returns, Compare, fund detail), that field is ALREADY nickname-
 * resolved server-side (see store.Asset.DisplayName), so those screens
 * keep calling FundNameFormatter.shorten(name) exactly as before with
 * no changes needed. This resolver exists only for the handful of
 * screens that read Assets/Benchmarks DIRECTLY off the raw portfolio
 * JSON rather than through a bridge compute call (Transactions,
 * Progression's fund picker, Manage Benchmarks) - those need the
 * nickname applied client-side.
 *
 * Deliberately does NOT also call FundNameFormatter.shorten on a
 * nickname - shorten() strips specific Indian-mutual-fund-scheme
 * boilerplate ("Direct Growth Plan Growth Option" etc.) that a short,
 * personal nickname the person typed themselves is essentially
 * guaranteed not to contain; running a hand-picked nickname through a
 * regex meant for long official scheme names risks a confusing edit
 * the person never asked for, however unlikely to actually trigger.
 */
object NicknameResolver {
    fun resolve(name: String, nickname: String?): String {
        val trimmed = nickname?.trim()
        if (!trimmed.isNullOrEmpty()) return trimmed
        return FundNameFormatter.shorten(name).ifBlank { name }
    }
}
