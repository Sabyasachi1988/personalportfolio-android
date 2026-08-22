# PersonalPortfolio — Project Handoff

## What this is
A native Windows desktop app (Go + lxn/walk, no CGO, no database — plain JSON
file storage) for tracking Indian mutual fund holdings across family members,
with CAS PDF import (MFCentral format), AMFI/Yahoo price refresh, XIRR,
family/individual views, and a real large/mid/small-cap + cash allocation
breakdown.

Module: `ledger` (see `go.mod`). Build with Go 1.24, cross-compiled for
Windows via `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build`.

## Package layout
- `internal/store` — domain types (Member, Account, Asset, StoredTransaction,
  PriceRecord, CapComposition) + JSON persistence with automatic timestamped
  backups on every save. `NewID()` uses an atomic counter, not just a
  timestamp — a nanosecond-timestamp-only ID generator *will* collide when
  several IDs are created in a tight loop (this was a real, shipped bug;
  see git history / chat log if resurrected).
- `internal/casimport` — MFCentral CAS PDF parser. Critical fact: the PDF
  library (`github.com/ledongthuc/pdf`) returns NO usable line/column
  coordinates for this specific PDF (browser/print-to-PDF generated) —
  `GetTextByRow()` collapses each page into one blob. The working approach
  uses `page.GetPlainText(fonts)` instead, which *does* preserve real
  newline-separated fields, and the parser is a line-based state machine
  over that, not coordinate/regex-based. There's a real, confirmed
  page-break rendering bug in MFCentral's own PDF generator: a wrapped
  transaction description duplicates the same Date/Amount/Units/Price/
  Balance across the page boundary — the parser merges these by detecting
  identical (date, amount, units, price, balance) on consecutive rows.
  Extensively tested against the real 30-page statement (270 real
  transactions, 0 manual-review lines). Native CAMS/KFintech (non-MFCentral)
  format was never built — no real sample file was ever available to test
  against, and guessing at PDF layout without one was explicitly avoided
  per this project's working principle (see below).
- `internal/priceapi` — AMFI NAV file parsing + Yahoo Finance quotes. AMFI's
  NAVAll.txt format changed in 2026 from 6 semicolon-separated fields to 8
  (inserted Plan/Option columns before NAV/Date) — the parser reads NAV and
  Date as the *last two* fields rather than assuming a fixed column count,
  so it survives that kind of change. Also parses the real AMFI section
  headers (e.g. "Open Ended Schemes(Equity Scheme - Large Cap Fund)") into
  an AssetClass field.
- `internal/finance` — XIRR (Newton-Raphson + bisection fallback), holdings
  aggregation, and allocation logic. Notably: `EffectiveAssetClass` and
  `GuessMarketCapSegment` exist because AMFI buckets ALL index funds/ETFs
  under one generic "Other" category regardless of what they track — a
  heuristic reads the actual fund name (e.g. "Nifty Smallcap 250" → Small
  Cap) to recover the real equity/debt/commodity split and cap-size mix.
  `CapComposition` (in store) holds real, manually-entered large/mid/small/
  cash percentages per fund (since true portfolio composition isn't
  available via any API — AMC sites block automated fetching via
  robots.txt, and aggregator sites are JS-rendered) — these override the
  heuristic when present.
- `cmd/portfolioapp` — the actual GUI app (Member/Account/Asset/Transaction/
  Import/Portfolio/Allocation/Price/Backup tabs). Also generates a separate
  polished interactive HTML report (Chart.js, real design system, opens in
  the default browser) since native Win32 widgets can't be made to look
  good no matter how much effort goes in — that's a hard ceiling on this
  toolkit, not a fixable bug.
- `cmd/cascli`, `cmd/casimportgui` — earlier standalone diagnostic tools
  used to debug the CAS parser iteratively before it was wired into the
  main app. Not needed going forward but harmless to keep.

## Working principles established during this project (please keep these)
- **Never guess at a data format** — get real sample data (a real CAS PDF
  dump, a live-fetched AMFI file, a real factsheet) before writing
  parsing logic against it. Every parser bug fixed in this project was
  fixed by getting real data first, not by reasoning about it in the
  abstract.
- **Test against real data, not just synthetic fixtures.** The CAS parser
  and AMFI parser tests use actual lines pulled from the user's real
  statement / a live AMFI fetch, not made-up examples.
- **State plainly when something can't be verified**, rather than
  presenting a guess with unwarranted confidence (e.g. the stamp-duty
  hypothesis for a small invested-amount discrepancy was explicitly
  flagged as unconfirmed, not asserted as fact).
- **lxn/walk gotchas worth knowing**: passing the `*walk.MainWindow`
  variable into tab-constructor functions before `.Create()` runs
  captures a nil snapshot — reference it as a package-level var instead.
  `Children().Clear()` does NOT dispose widgets, only removes bookkeeping
  — always call `.Dispose()` on each child first when rebuilding dynamic
  widget lists, and guard against reentrant rebuilds from `OnSizeChanged`
  firing more often than expected.

## Known gaps / not built
- Native CAMS/KFintech CAS format (only MFCentral is supported).
- Demat/equity holdings tab (user's account shows "No Folios Found" there,
  never had real data to build against).
- FIFO cost-basis lot tracking (net-invested / XIRR only, no per-lot basis).
- Automated cap-composition refresh (confirmed not feasible — AMC sites
  block robots, aggregators are JS-rendered/unscrapable reliably).

## User context relevant to a mobile app discussion
The user (Saby) wants an Android app "eventually," explicitly deferred
earlier in this project. If picked up: the Go backend (store/finance/
priceapi/casimport packages) is UI-framework-agnostic and could
potentially be reused via `gomobile` bindings or a small local HTTP API
consumed by a Kotlin/Flutter frontend, rather than rewritten from scratch
— worth evaluating early rather than assuming a full rewrite is required.
