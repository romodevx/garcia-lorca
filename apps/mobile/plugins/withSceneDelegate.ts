import { type ConfigPlugin, type InfoPlist, withAppDelegate, withInfoPlist } from "expo/config-plugins";

/**
 * Adopts the UIScene lifecycle on iOS.
 *
 * On the iOS 27 runtime, an app linked against the iOS 27 SDK that still uses
 * the AppDelegate-only lifecycle is killed at launch (EXC_BREAKPOINT in
 * _UIApplicationEvaluateRuntimeIssueForNoSceneLifecycleAdoption). Expo SDK 56's
 * template has no scene delegate and no option for one, so this plugin moves
 * the window into a UIWindowSceneDelegate during prebuild, for local and EAS
 * builds alike.
 */

const MARKER = "// @generated garcia-lorca with-scene-delegate";

// The window block of the SDK 56 template's didFinishLaunching. With scenes,
// a window created from UIScreen bounds is never attached to a scene.
const TEMPLATE_WINDOW_BLOCK =
  /#if os\(iOS\) \|\| os\(tvOS\)\n\s*window = UIWindow\(frame: UIScreen\.main\.bounds\)\n\s*factory\.startReactNative\(\n[\s\S]*?launchOptions: launchOptions\)\n#endif\n/;

const SCENE_DELEGATE = `
${MARKER}
// Launch builds only the React Native factory; the scene creates the window
// and starts React Native in it.
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

  // With scenes, deep links arrive here instead of application(_:open:options:).
  func scene(_ scene: UIScene, openURLContexts URLContexts: Set<UIOpenURLContext>) {
    guard let url = URLContexts.first?.url else { return }
    RCTLinkingManager.application(UIApplication.shared, open: url, options: [:])
  }
}
`;

/** Moves the template's window into a SceneDelegate appended to AppDelegate.swift. */
export function addSceneDelegate(appDelegate: string): string {
  if (appDelegate.includes(MARKER)) return appDelegate;
  if (!TEMPLATE_WINDOW_BLOCK.test(appDelegate)) {
    throw new Error("withSceneDelegate: AppDelegate.swift no longer matches the Expo SDK 56 template; revisit the scene patch");
  }
  return appDelegate.replace(TEMPLATE_WINDOW_BLOCK, "") + SCENE_DELEGATE;
}

/** Declares one application scene whose delegate is SceneDelegate. */
export function addSceneManifest(infoPlist: InfoPlist): InfoPlist {
  return {
    ...infoPlist,
    UIApplicationSceneManifest: {
      UIApplicationSupportsMultipleScenes: false,
      UISceneConfigurations: {
        UIWindowSceneSessionRoleApplication: [
          {
            UISceneConfigurationName: "Default",
            UISceneDelegateClassName: "$(PRODUCT_MODULE_NAME).SceneDelegate",
          },
        ],
      },
    },
  };
}

const withSceneDelegate: ConfigPlugin = (config) => {
  const withDelegate = withAppDelegate(config, (mod) => {
    if (mod.modResults.language !== "swift") {
      throw new Error(`withSceneDelegate: expected a Swift AppDelegate, got ${mod.modResults.language}`);
    }
    mod.modResults.contents = addSceneDelegate(mod.modResults.contents);
    return mod;
  });
  return withInfoPlist(withDelegate, (mod) => {
    mod.modResults = addSceneManifest(mod.modResults);
    return mod;
  });
};

export default withSceneDelegate;
