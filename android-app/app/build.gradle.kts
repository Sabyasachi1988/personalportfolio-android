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
        versionCode = 2
        versionName = "0.2"
    }

    // IMPORTANT: without this, every CI build (each running in a fresh,
    // throwaway container) would auto-generate a brand-new random debug
    // signing key, which made Android refuse to install an update over
    // the existing app - forcing an uninstall (and losing the portfolio
    // data) on every single new build. This fixed, committed keystore
    // makes every future build sign with the SAME key, so updates
    // install in place from here on. This is a debug-only key, not a
    // secret - the standard debug alias/password ("android"/"android")
    // is intentional and matches Android's own default convention.
    signingConfigs {
        create("fixedDebug") {
            storeFile = file("${rootDir}/keystore/debug.keystore")
            storePassword = "android"
            keyAlias = "androiddebugkey"
            keyPassword = "android"
        }
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
            signingConfig = signingConfigs.getByName("fixedDebug")
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
    implementation("com.google.android.material:material:1.12.0")
    implementation("androidx.activity:activity-ktx:1.9.0")
    implementation("androidx.recyclerview:recyclerview:1.3.2")
    implementation("androidx.viewpager2:viewpager2:1.1.0")
    implementation("com.google.code.gson:gson:2.11.0")
    implementation("androidx.biometric:biometric:1.1.0")
    implementation("androidx.lifecycle:lifecycle-process:2.8.7")
}
