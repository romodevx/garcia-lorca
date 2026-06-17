import { describe, expect, it } from "vitest";

import { checkParity } from "../parity/parity";
import { messages, NAMESPACES, type MessageKey } from "./index";

/**
 * Guards the seeded catalogs themselves: every namespace must be in full es/en
 * parity (keys + ICU arg signatures + valid ICU + `other` branches + non-empty +
 * flat), and the compile-time key type must reject unknown keys.
 */
describe("seeded catalogs", () => {
  it.each(NAMESPACES)("namespace '%s' is in full es/en parity", (namespace) => {
    const issues = checkParity(namespace, {
      es: messages.es[namespace],
      en: messages.en[namespace],
    });
    expect(issues).toEqual([]);
  });

  it("derives a type that accepts real keys and rejects unknown ones at compile time", () => {
    const realKey: MessageKey = "common.action.save";
    // @ts-expect-error — 'common.action.nope' is not a key in any seeded catalog.
    const unknownKey: MessageKey = "common.action.nope";

    expect(realKey).toBe("common.action.save");
    expect(typeof unknownKey).toBe("string");
  });
});
