import { lookBootstrapScript } from "../look/look";
import { themeBootstrapScript } from "./theme-script";

/**
 * The single <head> script that runs before the first paint: the theme
 * (`gl-theme`) and the look (`gl-look`). Each part has its own try/catch, so a
 * storage failure in one never costs the other.
 */
export const prePaintScript = `${themeBootstrapScript}\n${lookBootstrapScript}`;
