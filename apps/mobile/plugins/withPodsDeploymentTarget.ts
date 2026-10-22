import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";

import { type ConfigPlugin, withDangerousMod } from "expo/config-plugins";

/**
 * Builds every pod for at least the app's own minimum iOS version.
 *
 * Some pods' resource-bundle targets (react-native-svg 12.4, async-storage
 * 13.4) declare a minimum below Xcode 27's floor (15.0), which fails the build.
 * expo-build-properties' ios.deploymentTarget only sets the Podfile platform,
 * which those targets ignore, so this adds a post_install step that raises any
 * pod target below the app's minimum. It never lowers one.
 *
 * The Podfile is Ruby and has no structured mod, so this is a dangerous mod: an
 * idempotent insertion right after the template's post_install line that fails
 * loudly when that line is missing.
 */

const BEGIN = "# @generated begin garcia-lorca with-pods-deployment-target";
const END = "# @generated end garcia-lorca with-pods-deployment-target";
const POST_INSTALL = /^(\s*)post_install do \|installer\|\n/m;

const RAISE_POD_TARGETS = `${BEGIN}
    app_minimum = installer.aggregate_targets.first.platform.deployment_target.to_s
    installer.pods_project.targets.each do |pod_target|
      pod_target.build_configurations.each do |build_config|
        current = build_config.build_settings['IPHONEOS_DEPLOYMENT_TARGET']
        next unless current && Gem::Version.new(current) < Gem::Version.new(app_minimum)
        build_config.build_settings['IPHONEOS_DEPLOYMENT_TARGET'] = app_minimum
      end
    end
    ${END}
`;

/** Inserts the raise-pod-targets step at the start of the Podfile's post_install hook. */
export function addPodsDeploymentTarget(podfile: string): string {
  if (podfile.includes(BEGIN)) return podfile;
  if (!POST_INSTALL.test(podfile)) {
    throw new Error("withPodsDeploymentTarget: the Podfile has no post_install hook to extend");
  }
  return podfile.replace(POST_INSTALL, (hook, indent: string) => `${hook}${indent}  ${RAISE_POD_TARGETS}`);
}

const withPodsDeploymentTarget: ConfigPlugin = (config) =>
  withDangerousMod(config, [
    "ios",
    async (mod) => {
      const podfilePath = path.join(mod.modRequest.platformProjectRoot, "Podfile");
      const podfile = await readFile(podfilePath, "utf8");
      await writeFile(podfilePath, addPodsDeploymentTarget(podfile));
      return mod;
    },
  ]);

export default withPodsDeploymentTarget;
