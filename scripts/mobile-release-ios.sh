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
#   * EXPO_PUBLIC_API_URL=http://localhost:18080 — the simulator shares the
#     host's loopback, where prod-local publishes the gateway.
#   * NSAppTransportSecurity/NSAllowsLocalNetworking=true — ATS blocks plain
#     http; the local gateway has no TLS. It only relaxes local addresses, and
#     production talks https and does not need it.
#
# Requirements on this machine (checked below, fails loudly):
#   Xcode with the iOS Simulator SDK, CocoaPods, pnpm.
set -euo pipefail

REPO_ROOT=$(CDPATH='' cd -- "$(dirname "$0")/.." && pwd)
readonly REPO_ROOT
readonly MOBILE_DIR="$REPO_ROOT/apps/mobile"
readonly DEFAULT_API_URL="http://localhost:18080"
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

# UIScene lifecycle: on the iOS 27 runtime, an app linked against the iOS 27
# SDK that still uses the AppDelegate-only lifecycle is killed at launch
# (EXC_BREAKPOINT in _UIApplicationEvaluateRuntimeIssueForNoSceneLifecycleAdoption).
# Expo SDK 56's template has no scene delegate, so move the window into one.
appdelegate="ios/$scheme/AppDelegate.swift"
[ -f "$appdelegate" ] || die "prebuild produced no $appdelegate"
grep -q 'window = UIWindow(frame: UIScreen.main.bounds)' "$appdelegate" \
  || die "$appdelegate no longer matches the SDK 56 template; revisit the scene patch"
# Launch builds only the factory; SceneDelegate creates the window and starts React Native in it.
perl -0pi -e 's/#if os\(iOS\) \|\| os\(tvOS\)\n\s*window = UIWindow\(frame: UIScreen\.main\.bounds\)\n\s*factory\.startReactNative\(\n.*?launchOptions: launchOptions\)\n#endif\n//s' "$appdelegate"
! grep -q 'UIScreen.main.bounds' "$appdelegate" || die "could not move the window out of $appdelegate"
cat >> "$appdelegate" <<'SWIFT'

class SceneDelegate: UIResponder, UIWindowSceneDelegate {
  var window: UIWindow?

  func scene(
    _ scene: UIScene,
    willConnectTo session: UISceneSession,
    options connectionOptions: UIScene.ConnectionOptions
  ) {
    guard let windowScene = scene as? UIWindowScene,
          let appDelegate = UIApplication.shared.delegate as? AppDelegate,
          let factory = appDelegate.reactNativeFactory else { return }
    let window = UIWindow(windowScene: windowScene)
    self.window = window
    appDelegate.window = window
    factory.startReactNative(withModuleName: "main", in: window, launchOptions: nil)
    if let url = connectionOptions.urlContexts.first?.url {
      RCTLinkingManager.application(UIApplication.shared, open: url, options: [:])
    }
  }

  func scene(_ scene: UIScene, openURLContexts URLContexts: Set<UIOpenURLContext>) {
    guard let url = URLContexts.first?.url else { return }
    RCTLinkingManager.application(UIApplication.shared, open: url, options: [:])
  }
}
SWIFT
"$PLIST_BUDDY" -c "Delete :UIApplicationSceneManifest" "$plist" 2>/dev/null || true
"$PLIST_BUDDY" \
  -c "Add :UIApplicationSceneManifest dict" \
  -c "Add :UIApplicationSceneManifest:UIApplicationSupportsMultipleScenes bool false" \
  -c "Add :UIApplicationSceneManifest:UISceneConfigurations dict" \
  -c "Add :UIApplicationSceneManifest:UISceneConfigurations:UIWindowSceneSessionRoleApplication array" \
  -c "Add :UIApplicationSceneManifest:UISceneConfigurations:UIWindowSceneSessionRoleApplication:0 dict" \
  -c "Add :UIApplicationSceneManifest:UISceneConfigurations:UIWindowSceneSessionRoleApplication:0:UISceneConfigurationName string Default" \
  -c "Add :UIApplicationSceneManifest:UISceneConfigurations:UIWindowSceneSessionRoleApplication:0:UISceneDelegateClassName string \$(PRODUCT_MODULE_NAME).SceneDelegate" \
  "$plist"

say "pod install"
(cd ios && pod install)
[ -d "ios/$scheme.xcworkspace" ] || die "pod install produced no ios/$scheme.xcworkspace"

# ---- build ------------------------------------------------------------------
# Some pods' resource-bundle targets still declare iOS 12.4/13.4, below the
# floor of recent Xcode (15.0), which fails the build. Build every target for
# the app's own minimum, read from the generated project.
deployment_target=$(grep -m1 -oE 'IPHONEOS_DEPLOYMENT_TARGET = [0-9.]+' "$project/project.pbxproj" | grep -oE '[0-9.]+$')
[ -n "$deployment_target" ] || die "no IPHONEOS_DEPLOYMENT_TARGET in $project"
say "xcodebuild Release ($scheme, iphonesimulator, iOS >= $deployment_target)"
(cd ios && xcodebuild \
  -workspace "$scheme.xcworkspace" \
  -scheme "$scheme" \
  -configuration Release \
  -sdk iphonesimulator \
  -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath build \
  CODE_SIGNING_ALLOWED=NO \
  IPHONEOS_DEPLOYMENT_TARGET="$deployment_target" \
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
