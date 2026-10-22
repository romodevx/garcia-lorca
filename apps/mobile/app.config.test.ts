import { androidPackageConfig, easProjectConfig, iosBundleConfig } from "./app.config";
import type { ExpoConfig } from "expo/config";

describe("easProjectConfig", () => {
  it("adds the EAS project id and updates URL while preserving existing config", () => {
    const result = easProjectConfig(" project-123 ", {
      name: "School",
      slug: "school",
      updates: { enabled: true },
      extra: { eas: { owner: "owner" }, existing: true },
    } as Partial<ExpoConfig>);
    expect(result.extra).toEqual({ eas: { owner: "owner", projectId: "project-123" }, existing: true });
    expect(result.updates).toEqual({ enabled: true, url: "https://u.expo.dev/project-123" });
  });

  it("leaves current updates behavior untouched when the project id is absent", () => {
    const config = { updates: { enabled: true }, extra: { other: "value" } } as Partial<ExpoConfig>;
    expect(easProjectConfig(undefined, config)).toEqual({ extra: { ...config.extra, eas: {} } });
  });
});

describe("androidPackageConfig", () => {
  it("sets the Android application id from the build environment, keeping the rest of android", () => {
    const result = androidPackageConfig(" mx.example.family ", { android: { jsEngine: "hermes" } } as Partial<ExpoConfig>);
    expect(result).toEqual({ android: { jsEngine: "hermes", package: "mx.example.family" } });
  });

  it("adds nothing when no application id is given, so EAS keeps owning it", () => {
    expect(androidPackageConfig(undefined, { android: { jsEngine: "hermes" } } as Partial<ExpoConfig>)).toEqual({});
    expect(androidPackageConfig("  ", {} as Partial<ExpoConfig>)).toEqual({});
  });
});

describe("iosBundleConfig", () => {
  it("sets the iOS bundle identifier from the build environment, keeping the rest of ios", () => {
    const result = iosBundleConfig(" mx.example.family ", { ios: { jsEngine: "hermes", supportsTablet: true } } as Partial<ExpoConfig>);
    expect(result).toEqual({ ios: { jsEngine: "hermes", supportsTablet: true, bundleIdentifier: "mx.example.family" } });
  });

  it("adds nothing when no bundle identifier is given, so EAS keeps owning it", () => {
    expect(iosBundleConfig(undefined, { ios: { jsEngine: "hermes" } } as Partial<ExpoConfig>)).toEqual({});
    expect(iosBundleConfig("  ", {} as Partial<ExpoConfig>)).toEqual({});
  });
});
