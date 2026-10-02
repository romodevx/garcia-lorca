import { darkColors, lightColors } from "../../theme/tokens";
import { chipTint } from "./tint";

describe("chipTint", () => {
  it.each([
    ["light", lightColors],
    ["dark", darkColors],
  ] as const)("paints a tardy in its own color, apart from a warning (%s)", (_name, colors) => {
    const late = chipTint(colors, "late");
    expect(late).toEqual({ bg: colors.lateBg, fg: colors.late });
    expect(late.fg).not.toBe(chipTint(colors, "warn").fg);
  });
});
