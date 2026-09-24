import { describe, expect, it } from "vitest";

import { fitWithin } from "./photoResize";

describe("fitWithin", () => {
  it.each([
    [1600, 1200, { width: 800, height: 600 }],
    [1200, 1600, { width: 600, height: 800 }],
    [640, 480, { width: 640, height: 480 }],
    [801, 1, { width: 800, height: 1 }],
  ])("%ix%i", (w, h, want) => {
    expect(fitWithin(w, h, 800)).toEqual(want);
  });
});
