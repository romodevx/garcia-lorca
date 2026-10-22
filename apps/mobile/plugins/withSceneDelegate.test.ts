import type { ExpoConfig } from "expo/config";
import type { ExportedConfig } from "expo/config-plugins";

import withSceneDelegate, { addSceneDelegate, addSceneManifest } from "./withSceneDelegate";

// AppDelegate.swift exactly as `expo prebuild` generates it from the SDK 56
// template, including its open-URL and universal-link overrides.
const SDK56_APP_DELEGATE = `internal import Expo
import React
import ReactAppDependencyProvider

@main
class AppDelegate: ExpoAppDelegate {
  var window: UIWindow?

  var reactNativeDelegate: ExpoReactNativeFactoryDelegate?
  var reactNativeFactory: RCTReactNativeFactory?

  public override func application(
    _ application: UIApplication,
    didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
  ) -> Bool {
    let delegate = ReactNativeDelegate()
    let factory = ExpoReactNativeFactory(delegate: delegate)
    delegate.dependencyProvider = RCTAppDependencyProvider()

    reactNativeDelegate = delegate
    reactNativeFactory = factory

#if os(iOS) || os(tvOS)
    window = UIWindow(frame: UIScreen.main.bounds)
    factory.startReactNative(
      withModuleName: "main",
      in: window,
      launchOptions: launchOptions)
#endif

    return super.application(application, didFinishLaunchingWithOptions: launchOptions)
  }

  // Linking API
  public override func application(
    _ app: UIApplication,
    open url: URL,
    options: [UIApplication.OpenURLOptionsKey: Any] = [:]
  ) -> Bool {
    return super.application(app, open: url, options: options) || RCTLinkingManager.application(app, open: url, options: options)
  }

  // Universal Links
  public override func application(
    _ application: UIApplication,
    continue userActivity: NSUserActivity,
    restorationHandler: @escaping ([UIUserActivityRestoring]?) -> Void
  ) -> Bool {
    let result = RCTLinkingManager.application(application, continue: userActivity, restorationHandler: restorationHandler)
    return super.application(application, continue: userActivity, restorationHandler: restorationHandler) || result
  }
}

class ReactNativeDelegate: ExpoReactNativeFactoryDelegate {
  // Extension point for config-plugins

  override func sourceURL(for bridge: RCTBridge) -> URL? {
    // needed to return the correct URL for expo-dev-client.
    bridge.bundleURL ?? bundleURL()
  }

  override func bundleURL() -> URL? {
#if DEBUG
    return RCTBundleURLProvider.sharedSettings().jsBundleURL(forBundleRoot: ".expo/.virtual-metro-entry")
#else
    return Bundle.main.url(forResource: "main", withExtension: "jsbundle")
#endif
  }
}
`;

/** The SceneDelegate the plugin appends: everything from its class declaration on. */
function sceneDelegateOf(appDelegate: string): string {
  return appDelegate.slice(appDelegate.indexOf("class SceneDelegate"));
}

describe("addSceneDelegate", () => {
  it("moves the window out of the launch callback into a UIWindowSceneDelegate", () => {
    const result = addSceneDelegate(SDK56_APP_DELEGATE);

    expect(result).not.toContain("UIScreen.main.bounds");
    expect(result).toContain("reactNativeFactory = factory");
    expect(result).toContain("class SceneDelegate: UIResponder, UIWindowSceneDelegate");
    expect(result).toContain("UIWindow(windowScene: windowScene)");
  });

  it("keeps the template's open-URL and universal-link overrides and the ReactNativeDelegate", () => {
    const result = addSceneDelegate(SDK56_APP_DELEGATE);

    expect(result).toContain("RCTLinkingManager.application(app, open: url, options: options)");
    expect(result).toContain("RCTLinkingManager.application(application, continue: userActivity, restorationHandler: restorationHandler)");
    expect(result).toContain("class ReactNativeDelegate: ExpoReactNativeFactoryDelegate");
  });

  it("hands a cold-start deep link to React Native as launch options, so getInitialURL sees it", () => {
    const scene = sceneDelegateOf(addSceneDelegate(SDK56_APP_DELEGATE));

    expect(scene).toContain('factory.startReactNative(withModuleName: "main", in: window, launchOptions: launchOptions(from: connectionOptions))');
    expect(scene).toContain("options[.url] = url");
    expect(scene).toContain("options[.userActivityDictionary] = [");
    expect(scene).toContain("UIApplication.LaunchOptionsKey.userActivityType.rawValue: userActivity.activityType");
    expect(scene).toContain('"UIApplicationLaunchOptionsUserActivityKey": userActivity');
    // A notification posted before JS subscribes is lost; launch options are not.
    expect(scene).not.toContain("RCTLinkingManager");
  });

  it("hands a cold-start link to Expo's open-URL subscriber before React Native starts", () => {
    const scene = sceneDelegateOf(addSceneDelegate(SDK56_APP_DELEGATE));
    const connect = scene.slice(scene.indexOf("willConnectTo"), scene.indexOf("openURLContexts"));

    // expo-router reads the initial URL from expo-linking's registry, which only
    // that subscriber fills; it must be set before the JS bundle asks for it.
    const forward = connect.indexOf("appDelegate?.application(UIApplication.shared, open: url, options: [:])");
    expect(forward).toBeGreaterThan(-1);
    expect(forward).toBeLessThan(connect.indexOf("factory.startReactNative("));
  });

  it("routes warm deep links through the AppDelegate so Expo's subscribers see them", () => {
    const scene = sceneDelegateOf(addSceneDelegate(SDK56_APP_DELEGATE));

    expect(scene).toContain("func scene(_ scene: UIScene, openURLContexts URLContexts: Set<UIOpenURLContext>)");
    expect(scene).toContain("appDelegate?.application(UIApplication.shared, open: url, options: [:])");
  });

  it("forwards user activities (universal links) at connect time and while running", () => {
    const scene = sceneDelegateOf(addSceneDelegate(SDK56_APP_DELEGATE));

    expect(scene).toContain("func scene(_ scene: UIScene, continue userActivity: NSUserActivity)");
    expect(scene).toContain("connectionOptions.userActivities.forEach");
    expect(scene).toContain("appDelegate?.application(UIApplication.shared, continue: userActivity, restorationHandler: { _ in })");
  });

  it("fails loudly in debug builds when launch built no React Native factory", () => {
    const scene = sceneDelegateOf(addSceneDelegate(SDK56_APP_DELEGATE));

    expect(scene).toContain("assertionFailure(");
    expect(scene).toContain("NSLog(");
  });

  it("documents why every scene connection gets a fresh window", () => {
    expect(sceneDelegateOf(addSceneDelegate(SDK56_APP_DELEGATE))).toMatch(/reconnect/i);
  });

  it("is idempotent, so a second prebuild without --clean changes nothing", () => {
    const once = addSceneDelegate(SDK56_APP_DELEGATE);
    expect(addSceneDelegate(once)).toBe(once);
  });

  it("fails loudly when the template no longer matches, instead of half-patching it", () => {
    const changed = SDK56_APP_DELEGATE.replace("UIWindow(frame: UIScreen.main.bounds)", "makeWindow()");
    expect(() => addSceneDelegate(changed)).toThrow(/AppDelegate/);
  });
});

describe("addSceneManifest", () => {
  it("declares a single-window scene backed by SceneDelegate, keeping the rest of Info.plist", () => {
    const result = addSceneManifest({ CFBundleName: "García Lorca" });

    expect(result.CFBundleName).toBe("García Lorca");
    expect(result.UIApplicationSceneManifest).toEqual({
      UIApplicationSupportsMultipleScenes: false,
      UISceneConfigurations: {
        UIWindowSceneSessionRoleApplication: [
          {
            UISceneConfigurationName: "Default",
            UISceneDelegateClassName: "$(PRODUCT_MODULE_NAME).SceneDelegate",
          },
        ],
      },
    });
  });
});

describe("withSceneDelegate", () => {
  it("registers the AppDelegate and Info.plist mods for iOS", () => {
    const config: ExportedConfig = withSceneDelegate({ name: "School", slug: "school" } as ExpoConfig);
    const iosMods = config.mods?.ios ?? {};

    expect(Object.keys(iosMods)).toEqual(expect.arrayContaining(["appDelegate", "infoPlist"]));
  });
});
