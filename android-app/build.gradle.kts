// Root build file. Deliberately pinned to AGP 8.5 / Kotlin 1.9.24 — an
// older but extremely well-documented, stable combination — rather than
// the newer AGP 9.x line, since this whole toolchain is being tested for
// the first time via CI with nobody able to debug it interactively on a
// local machine. Fewer moving parts = fewer surprises on the first run.
plugins {
    id("com.android.application") version "8.5.0" apply false
    id("org.jetbrains.kotlin.android") version "1.9.24" apply false
}
