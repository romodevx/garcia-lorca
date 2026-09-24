import { describe, expect, it } from "vitest";

import { lowercaseKeepingCaret } from "./lowercaseKeepingCaret";

function inputWith(value: string, caret: number): HTMLInputElement {
  const input = document.createElement("input");
  input.type = "text";
  document.body.append(input);
  input.value = value;
  input.setSelectionRange(caret, caret);
  return input;
}

describe("lowercaseKeepingCaret", () => {
  it("lowers the value and keeps the caret where it was", () => {
    const input = inputWith("ana.Mruiz@x.mx", 5);
    expect(lowercaseKeepingCaret(input, false)).toBe("ana.mruiz@x.mx");
    expect(input.selectionStart).toBe(5);
  });

  it("follows the caret when lowering changes the length before it", () => {
    // "İ" (U+0130) lowers to "i̇" — two UTF-16 units. The caret after it must
    // land after both, not between the "i" and its combining dot.
    const input = inputWith("ana.İruiz@x.mx", 5);
    const lowered = lowercaseKeepingCaret(input, false);
    expect(lowered).toBe("ana.i̇ruiz@x.mx");
    expect(input.selectionStart).toBe(6);
  });

  it("leaves the field alone while an IME is composing", () => {
    // Rewriting the value mid-composition makes Android keyboards re-commit
    // the composed text: characters duplicate. Lowering waits for the end.
    const input = inputWith("Ana", 3);
    expect(lowercaseKeepingCaret(input, true)).toBe("Ana");
    expect(input.value).toBe("Ana");
  });
});
