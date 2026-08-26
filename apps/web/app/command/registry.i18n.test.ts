import { describe, expect, it } from "vitest";
import { LOCALES, messages } from "@garcia-lorca/i18n";

import { MODULES } from "../_design/data";
import { ACTIONS } from "./registry";

/**
 * Every key the palette asks for has to exist, in every language.
 *
 * The palette builds one entry per module and looks up `palette.nav.<id>`. Adding
 * a module to MODULES without adding its key throws at RENDER time — next-intl
 * raises MISSING_MESSAGE and the whole shell fails — and nothing before this test
 * connected the two lists. That is exactly what happened when Docentes shipped.
 */
describe("the palette can name every module it offers", () => {
  for (const locale of LOCALES) {
    describe(locale, () => {
      const palette = messages[locale].palette as Record<string, string>;

      for (const module of MODULES) {
        it(`nav.${module.id}`, () => {
          const value = palette[`nav.${module.id}`];
          expect(value, `palette.nav.${module.id} is missing in ${locale}`).toBeDefined();
          expect(value?.trim(), `palette.nav.${module.id} is empty in ${locale}`).not.toBe("");
        });
      }

      // The action commands ("cobrar", "pasar lista"…) name their own keys, and a
      // typo there fails the same way.
      for (const action of ACTIONS) {
        it(`${action.titleKey}`, () => {
          expect(palette[action.titleKey], `${action.titleKey} is missing in ${locale}`).toBeDefined();
        });
        if (action.aliasKey !== undefined) {
          it(`${action.aliasKey}`, () => {
            expect(palette[action.aliasKey!], `${action.aliasKey} is missing in ${locale}`).toBeDefined();
          });
        }
      }
    });
  }
});
