import type { ExpoConfig } from "expo/config";
import type { ExportedConfig } from "expo/config-plugins";

import withPodsDeploymentTarget, { addPodsDeploymentTarget } from "./withPodsDeploymentTarget";

// The post_install block of the SDK 56 template Podfile.
const SDK56_PODFILE = `target 'GarcaLorca' do
  use_expo_modules!

  post_install do |installer|
    react_native_post_install(
      installer,
      config[:reactNativePath],
      :mac_catalyst_enabled => false,
      :ccache_enabled => ccache_enabled?(podfile_properties),
    )
  end
end
`;

describe("addPodsDeploymentTarget", () => {
  it("raises pod targets below the app's minimum iOS inside post_install", () => {
    const result = addPodsDeploymentTarget(SDK56_PODFILE);

    expect(result).toMatch(/post_install do \|installer\|\n[\s\S]*IPHONEOS_DEPLOYMENT_TARGET[\s\S]*react_native_post_install/);
    expect(result).toContain("installer.aggregate_targets");
  });

  it("indents the step relative to the post_install hook it extends", () => {
    const nested = "abstract_target 'All' do\n  target 'App' do\n    post_install do |installer|\n      react_native_post_install(installer)\n    end\n  end\nend\n";
    const lines = addPodsDeploymentTarget(nested).split("\n");

    expect(lines).toContain("      app_minimum = installer.aggregate_targets.first.platform.deployment_target.to_s");
    expect(lines).toContain("        pod_target.build_configurations.each do |build_config|");
    expect(lines.filter((line) => line.includes("@generated")).every((line) => line.startsWith("      #"))).toBe(true);
  });

  it("captures only the hook's own indent, not a blank line above it", () => {
    const withBlankLine = "target 'App' do\n  use_expo_modules!\n\n  post_install do |installer|\n    react_native_post_install(installer)\n  end\nend\n";

    expect(addPodsDeploymentTarget(withBlankLine)).toContain(
      "\n\n  post_install do |installer|\n    # @generated begin garcia-lorca with-pods-deployment-target\n    app_minimum =",
    );
  });

  it("is idempotent", () => {
    const once = addPodsDeploymentTarget(SDK56_PODFILE);
    expect(addPodsDeploymentTarget(once)).toBe(once);
  });

  it("fails loudly when the Podfile has no post_install hook", () => {
    expect(() => addPodsDeploymentTarget("target 'X' do\nend\n")).toThrow(/post_install/);
  });
});

describe("withPodsDeploymentTarget", () => {
  it("registers an iOS dangerous mod for the Podfile", () => {
    const config: ExportedConfig = withPodsDeploymentTarget({ name: "School", slug: "school" } as ExpoConfig);
    expect(Object.keys(config.mods?.ios ?? {})).toContain("dangerous");
  });
});
