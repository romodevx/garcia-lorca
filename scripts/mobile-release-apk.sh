#!/usr/bin/env bash
# A RELEASE Android build of the family app (apps/mobile), for the local
# emulator, talking to the production-mode local gateway (scripts/prod-local.sh).
#
#   scripts/mobile-release-apk.sh            build the APK
#   scripts/mobile-release-apk.sh --install  build, boot the emulator if needed, install, launch
#
# What "release" means here: `expo prebuild` (Continuous Native Generation, the
# android/ folder is generated and gitignored) + `./gradlew assembleRelease`:
# Hermes bytecode, minified bundle, no dev menu, no Metro. It is signed with the
# template's DEBUG keystore, which is what makes it installable on an emulator
# without a release key; a store build goes through EAS with the real key.
#
# Two deliberate local-only deviations, both applied to the GENERATED android/
# folder and never to the app's source:
#   * EXPO_PUBLIC_API_URL=http://10.0.2.2:18080 — 10.0.2.2 is the emulator's
#     alias for the host's loopback, where prod-local publishes the gateway.
#   * usesCleartextTraffic="true" — Android blocks plain http in release builds;
#     the local gateway has no TLS. Production talks https and does not need it.
#
# Requirements on this machine (checked below, fails loudly):
#   JDK 17 exactly (Homebrew openjdk@17 is used when no JAVA_HOME is set),
#   Android SDK at $ANDROID_HOME or ~/Library/Android/sdk, pnpm.
set -euo pipefail

REPO_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
readonly REPO_ROOT
readonly MOBILE_DIR="$REPO_ROOT/apps/mobile"
# JDK 17 exactly: React Native's gradle plugin asks for a Java 17 toolchain, and
# with any other JDK (Android Studio's JBR is 21) Gradle tries to download one
# through foojay-resolver 0.5.0, which crashes on Gradle 9 ("JvmVendorSpec does
# not have member field IBM_SEMERU"). `brew install openjdk@17` (no sudo).
readonly BREW_JDK17="/opt/homebrew/opt/openjdk@17/libexec/openjdk.jdk/Contents/Home"
readonly DEFAULT_API_URL="http://10.0.2.2:18080"
readonly DEFAULT_AVD="Medium_Phone_API_36.1"
# LOCAL-ONLY application id (".local" suffix): the store id is the owner's call
# and is not fixed in app.json yet. app.config.ts reads ANDROID_PACKAGE.
readonly DEFAULT_ANDROID_PACKAGE="mx.edu.garcialorca.familias.local"
readonly OUT_DIR="$REPO_ROOT/.prod-local"
readonly APK_OUT="$OUT_DIR/garcia-lorca-family-release.apk"
readonly BOOT_TIMEOUT_SECONDS=180

die() { echo "[apk] ERROR: $*" >&2; exit 1; }
say() { echo "[apk] $*"; }

INSTALL=""
[ "${1:-}" = "--install" ] && INSTALL=yes

# ---- toolchain --------------------------------------------------------------
if [ -z "${JAVA_HOME:-}" ] && [ -x "$BREW_JDK17/bin/java" ]; then
  export JAVA_HOME="$BREW_JDK17"
fi
[ -n "${JAVA_HOME:-}" ] && [ -x "$JAVA_HOME/bin/java" ] \
  || die "no JDK 17: brew install openjdk@17 (or set JAVA_HOME to a JDK 17)"
"$JAVA_HOME/bin/java" -version 2>&1 | grep -q 'version "17' \
  || die "JAVA_HOME is not a JDK 17 ($JAVA_HOME); RN's gradle plugin needs exactly 17"
export ANDROID_HOME="${ANDROID_HOME:-$HOME/Library/Android/sdk}"
[ -d "$ANDROID_HOME/platforms" ] || die "no Android SDK at $ANDROID_HOME"
export PATH="$JAVA_HOME/bin:$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator:$PATH"
command -v pnpm >/dev/null || die "pnpm not found (corepack enable pnpm)"

export EXPO_PUBLIC_API_URL="${EXPO_PUBLIC_API_URL:-$DEFAULT_API_URL}"
export NODE_ENV=production
export ANDROID_PACKAGE="${ANDROID_PACKAGE:-$DEFAULT_ANDROID_PACKAGE}"
say "JAVA_HOME=$JAVA_HOME"
say "EXPO_PUBLIC_API_URL=$EXPO_PUBLIC_API_URL"

# ---- native project ---------------------------------------------------------
cd "$REPO_ROOT"
pnpm install --frozen-lockfile --filter @garcia-lorca/mobile...

cd "$MOBILE_DIR"
say "expo prebuild (android, clean)"
CI=1 pnpm exec expo prebuild --platform android --clean --no-install

manifest=android/app/src/main/AndroidManifest.xml
[ -f "$manifest" ] || die "prebuild produced no $manifest"
if ! grep -q 'android:usesCleartextTraffic="true"' "$manifest"; then
  sed -i.bak 's/<application /<application android:usesCleartextTraffic="true" /' "$manifest"
  rm -f "$manifest.bak"
fi
grep -q 'android:usesCleartextTraffic="true"' "$manifest" || die "could not enable cleartext traffic in $manifest"

# ---- build ------------------------------------------------------------------
# Only the emulator's ABI: four ABIs is four native builds for nothing.
abi="${ANDROID_ABI:-arm64-v8a}"
say "gradlew assembleRelease ($abi)"
(cd android && ./gradlew --no-daemon assembleRelease -PreactNativeArchitectures="$abi")

apk=android/app/build/outputs/apk/release/app-release.apk
[ -f "$apk" ] || die "assembleRelease produced no $apk"
if unzip -l "$apk" | grep -qE '\.map$'; then
  die "the release APK contains a source map"
fi
mkdir -p "$OUT_DIR"
cp "$apk" "$APK_OUT"
say "APK: $APK_OUT ($(du -h "$APK_OUT" | cut -f1)), no source maps inside"

[ -n "$INSTALL" ] || exit 0

# ---- emulator ---------------------------------------------------------------
if ! adb devices | grep -q 'emulator-.*device$'; then
  avd="${AVD:-$DEFAULT_AVD}"
  say "booting emulator $avd"
  nohup emulator -avd "$avd" -no-snapshot-save >/dev/null 2>&1 &
  adb wait-for-device
  waited=0
  until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do
    [ "$waited" -lt "$BOOT_TIMEOUT_SECONDS" ] || die "emulator did not boot in ${BOOT_TIMEOUT_SECONDS} s"
    sleep 3; waited=$((waited + 3))
  done
fi
adb install -r "$APK_OUT"
adb shell monkey -p "$ANDROID_PACKAGE" -c android.intent.category.LAUNCHER 1 >/dev/null
say "installed and launched $ANDROID_PACKAGE on the emulator"
