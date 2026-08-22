plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.saby.personalportfolio"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.saby.personalportfolio"
        minSdk = 24
        targetSdk = 34
        versionCode = 1
        // This "0.1-prototype" name is intentional: this build's only job
        // is to prove the Go -> gomobile bind -> Kotlin -> APK toolchain
        // works at all (see MainActivity.kt). It is not the real app yet.
        versionName = "0.1-prototype"
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }
    kotlinOptions {
        jvmTarget = "17"
    }
}

dependencies {
    // Produced during the CI build by `gomobile bind` — see
    // .github/workflows/build.yml. Not committed to the repo since it's a
    // generated build artifact, not source.
    implementation(files("libs/bridge.aar"))

    implementation("androidx.core:core-ktx:1.13.1")
    implementation("androidx.appcompat:appcompat:1.7.0")
    implementation("androidx.activity:activity-ktx:1.9.0")
    implementation("androidx.recyclerview:recyclerview:1.3.2")
    implementation("com.google.code.gson:gson:2.11.0")
}
