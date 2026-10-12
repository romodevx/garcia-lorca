import { describe, expect, it } from "vitest";
import { LOCALES, messages } from "@garcia-lorca/i18n";

import { MODULES } from "../_design/data";
import { HOME_AREAS } from "./areas";

/**
 * Every key the refreshed home asks for exists, non-empty, in every language
 * (eng review E16). next-intl throws MISSING_MESSAGE at render time, so a
 * module added to MODULES without its description would take the home down.
 * Hidden modules included: un-hiding one must not need a copy change too.
 */
describe("the home can name every area and describe every module", () => {
  for (const locale of LOCALES) {
    describe(locale, () => {
      const home = messages[locale].home as Record<string, string>;
      const expectKey = (key: string): void => {
        expect(home[key], `home.${key} is missing in ${locale}`).toBeDefined();
        expect(home[key]?.trim(), `home.${key} is empty in ${locale}`).not.toBe("");
      };

      it.each(HOME_AREAS.map((a) => a.id))("group.%s", (id) => expectKey(`group.${id}`));
      it.each(MODULES.map((m) => m.id))("desc.%s", (id) => expectKey(`desc.${id}`));
      it("heading.yours (≤ 4 modules)", () => expectKey("heading.yours"));
    });
  }
});
