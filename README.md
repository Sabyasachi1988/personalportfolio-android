# PersonalPortfolio — Android prototype

This repo contains:
- The original, tested Windows desktop app source (`internal/`, `cmd/`).
  See `PROJECT_HANDOFF.md`.
- `mobile/bridge/` — a small new Go package that exposes the tested
  domain logic (CAS import, XIRR, holdings, allocation) to Android.
- `android-app/` — a minimal Android app that calls that bridge and shows
  "Go bridge says: bridge ok" on screen if it worked.

**This is not the real app yet.** Its only job is to prove that the whole
chain — Go code, compiled via `gomobile bind`, called from Kotlin, packaged
into an installable `.apk` — actually works, all done automatically by
GitHub (see `.github/workflows/build.yml`), with nothing installed or run
on your own computer.

## To build it
1. Go to the **Actions** tab of this repo on GitHub's website.
2. Click **Build Android App** on the left, then the **Run workflow**
   button, then **Run workflow** again to confirm.
3. Wait a few minutes (GitHub is doing the work, not you).
4. When it finishes with a green checkmark, click into that run and
   download **PersonalPortfolio-test-apk** at the bottom of the page.
5. Copy that `.apk` file to your Android phone and open it (you'll need to
   allow "install from unknown sources" once — Android will prompt you).
6. Open the app. If it says "Go bridge says: bridge ok" — the toolchain
   works, and we build the real app on top of it next.

If the build fails (red X instead of green check), that's useful
information, not a dead end — click into the failed step to see the error
and bring it back to the chat.
