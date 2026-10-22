#!/usr/bin/env bash
# A RELEASE iOS Simulator build of the family app (apps/mobile), talking to the
# production-mode local gateway (scripts/prod-local.sh).
#
#   scripts/mobile-release-ios.sh            build the .app
#   scripts/mobile-release-ios.sh --install  build, install and launch on the booted simulator
#
# What "release" means here: `expo prebuild` (Continuous Native Generation, the
# ios/ folder is generated and gitignored) + `xcodebuild -configuration Release`
# for the iphonesimulator SDK: Hermes bytecode, minified bundle, no dev menu, no
# Metro. A simulator build needs no code signing; a device or store build goes
# through EAS with the real certificates.
#
# Two deliberate local-only deviations, both applied to the GENERATED ios/
# folder or the build environment, never to the app's source:
#   * EXPO_PUBLIC_API_URL=http://127.0.0.1:18080 — the simulator shares the
#     host's loopback, where prod-local publishes the gateway on IPv4 only.
#     Not "localhost": it resolves to ::1 first and the connection is refused.
#   * NSAppTransportSecurity/NSAllowsLocalNetworking=true — ATS blocks plain
#     http; the local gateway has no TLS. It only relaxes unqualified and .local
#     names and IP literals (127.0.0.1 included; Apple's NSAllowsLocalNetworking
#     docs), and production talks https and does not need it.
#
# Requirements on this machine (checked below, fails loudly):
#   Xcode with the iOS Simulator SDK, CocoaPods, pnpm.
set -euo pipefail

REPO_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
readonly REPO_ROOT
readonly MOBILE_DIR="$REPO_ROOT/apps/mobile"
readonly DEFAULT_API_URL="http://127.0.0.1:18080"
# LOCAL-ONLY bundle id (".local" suffix): the store id is the owner's call and
# is not fixed in app.json yet. app.config.ts reads IOS_BUNDLE_ID.
readonly DEFAULT_IOS_BUNDLE_ID="mx.edu.garcialorca.familias.local"
readonly OUT_DIR="$REPO_ROOT/.prod-local"
readonly APP_OUT="$OUT_DIR/garcia-lorca-family-release.app"
readonly PLIST_BUDDY=/usr/libexec/PlistBuddy

die() { echo "[ios] ERROR: $*" >&2; exit 1; }
say() { echo "[ios] $*"; }

INSTALL=""
[ "${1:-}" = "--install" ] && INSTALL=yes

# ---- toolchain --------------------------------------------------------------
command -v xcodebuild >/dev/null || die "xcodebuild not found (install Xcode)"
xcodebuild -showsdks 2>/dev/null | grep -q iphonesimulator \
  || die "no iOS Simulator SDK (Xcode > Settings > Components)"
command -v pod >/dev/null || die "CocoaPods not found (brew install cocoapods)"
command -v pnpm >/dev/null || die "pnpm not found (corepack enable pnpm)"

export EXPO_PUBLIC_API_URL="${EXPO_PUBLIC_API_URL:-$DEFAULT_API_URL}"
export NODE_ENV=production
export IOS_BUNDLE_ID="${IOS_BUNDLE_ID:-$DEFAULT_IOS_BUNDLE_ID}"
# CocoaPods breaks on non-UTF-8 locales.
export LANG="${LANG:-en_US.UTF-8}"
say "$(xcodebuild -version | head -1)"
say "EXPO_PUBLIC_API_URL=$EXPO_PUBLIC_API_URL"

# ---- native project ---------------------------------------------------------
cd "$REPO_ROOT"
# --prod=false: NODE_ENV=production would otherwise skip the devDependencies
# that prebuild and the bundler need.
pnpm install --frozen-lockfile --prod=false --filter @garcia-lorca/mobile...

mkdir -p "$OUT_DIR"
cd "$MOBILE_DIR"
say "expo prebuild (ios, clean)"
# prebuild rewrites package.json's android/ios scripts to `expo run:*`; that is
# a choice for the repo, not a side effect of a local build. Put it back.
# Restored on ANY exit (a failed prebuild or xcodebuild included), not only on success.
readonly PACKAGE_BACKUP="$OUT_DIR/package.json.before-prebuild"
cp package.json "$PACKAGE_BACKUP"
trap 'cp "$PACKAGE_BACKUP" "$MOBILE_DIR/package.json"' EXIT
CI=1 pnpm exec expo prebuild --platform ios --clean --no-install

# The scheme is the project name prebuild derives from the app name; the
# .xcworkspace only appears after `pod install`.
project=$(find ios -maxdepth 1 -name '*.xcodeproj' -print -quit)
[ -n "$project" ] || die "prebuild produced no ios/*.xcodeproj"
scheme=$(basename "$project" .xcodeproj)
plist="ios/$scheme/Info.plist"
[ -f "$plist" ] || die "prebuild produced no $plist"
"$PLIST_BUDDY" -c "Add :NSAppTransportSecurity dict" "$plist" 2>/dev/null || true
"$PLIST_BUDDY" -c "Delete :NSAppTransportSecurity:NSAllowsLocalNetworking" "$plist" 2>/dev/null || true
"$PLIST_BUDDY" -c "Add :NSAppTransportSecurity:NSAllowsLocalNetworking bool true" "$plist"
[ "$("$PLIST_BUDDY" -c "Print :NSAppTransportSecurity:NSAllowsLocalNetworking" "$plist")" = "true" ] \
  || die "could not allow local networking in $plist"

# UIScene lifecycle and the pods' minimum iOS are applied by the app's own
# config plugins (apps/mobile/plugins/), so EAS builds get them too. Here we
# only check that prebuild ran them.
appdelegate="ios/$scheme/AppDelegate.swift"
grep -q 'class SceneDelegate: UIResponder, UIWindowSceneDelegate' "$appdelegate" \
  || die "$appdelegate has no SceneDelegate: is plugins/withSceneDelegate.ts registered in app.json?"
"$PLIST_BUDDY" -c "Print :UIApplicationSceneManifest:UISceneConfigurations:UIWindowSceneSessionRoleApplication:0:UISceneDelegateClassName" "$plist" 2>/dev/null \
  | grep -q 'SceneDelegate$' || die "$plist declares no scene manifest (withSceneDelegate plugin)"
grep -q 'with-pods-deployment-target' ios/Podfile \
  || die "ios/Podfile lacks the pods deployment target step: is plugins/withPodsDeploymentTarget.ts registered?"

say "pod install"
(cd ios && pod install)
[ -d "ios/$scheme.xcworkspace" ] || die "pod install produced no ios/$scheme.xcworkspace"

# ---- build ------------------------------------------------------------------
deployment_target=$(grep -m1 -oE 'IPHONEOS_DEPLOYMENT_TARGET = [0-9.]+' "$project/project.pbxproj" | grep -oE '[0-9.]+$')
[ -n "$deployment_target" ] || die "no IPHONEOS_DEPLOYMENT_TARGET in $project"
lowest_pod_target=$(grep -oE 'IPHONEOS_DEPLOYMENT_TARGET = [0-9.]+' ios/Pods/Pods.xcodeproj/project.pbxproj \
  | grep -oE '[0-9.]+$' | sort -V | head -1)
[ "$(printf '%s\n%s\n' "$lowest_pod_target" "$deployment_target" | sort -V | head -1)" = "$deployment_target" ] \
  || die "a pod still targets iOS $lowest_pod_target, below the app's $deployment_target (withPodsDeploymentTarget plugin)"
say "xcodebuild Release ($scheme, iphonesimulator, iOS >= $deployment_target)"
(cd ios && xcodebuild \
  -workspace "$scheme.xcworkspace" \
  -scheme "$scheme" \
  -configuration Release \
  -sdk iphonesimulator \
  -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath build \
  CODE_SIGNING_ALLOWED=NO \
  build)

app="ios/build/Build/Products/Release-iphonesimulator/$scheme.app"
[ -d "$app" ] || die "xcodebuild produced no $app"
if find "$app" -name '*.map' | grep -q .; then
  die "the release .app contains a source map"
fi
# Only the JS bundle: Hermes' own binary legitimately contains the string.
bundle="$app/main.jsbundle"
[ -f "$bundle" ] || die "the release .app has no embedded main.jsbundle"
if grep -aq 'sourceMappingURL' "$bundle"; then
  die "the release JS bundle references a source map"
fi
rm -rf "$APP_OUT"
cp -R "$app" "$APP_OUT"
say "APP: $APP_OUT ($(du -sh "$APP_OUT" | cut -f1)), no source maps inside"

[ -n "$INSTALL" ] || exit 0

# ---- simulator --------------------------------------------------------------
xcrun simctl list devices booted | grep -q Booted \
  || die "no booted simulator (open -a Simulator, or xcrun simctl boot <udid>)"
xcrun simctl install booted "$APP_OUT"
xcrun simctl launch booted "$IOS_BUNDLE_ID"
say "installed and launched $IOS_BUNDLE_ID on the booted simulator"
