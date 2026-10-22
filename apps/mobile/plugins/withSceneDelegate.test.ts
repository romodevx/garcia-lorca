import type { ExpoConfig } from "expo/config";
import type { ExportedConfig } from "expo/config-plugins";

import withSceneDelegate, { addSceneDelegate, addSceneManifest } from "./withSceneDelegate";

// The AppDelegate.swift that `expo prebuild` generates from the SDK 56 template.
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
}
`;

describe("addSceneDelegate", () => {
  it("moves the window out of the launch callback into a UIWindowSceneDelegate", () => {
    const result = addSceneDelegate(SDK56_APP_DELEGATE);

    expect(result).not.toContain("UIScreen.main.bounds");
    expect(result).toContain("reactNativeFactory = factory");
    expect(result).toContain("class SceneDelegate: UIResponder, UIWindowSceneDelegate");
    expect(result).toContain("UIWindow(windowScene: windowScene)");
    expect(result).toContain('factory.startReactNative(withModuleName: "main", in: window, launchOptions: nil)');
    expect(result).toContain("openURLContexts");
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
