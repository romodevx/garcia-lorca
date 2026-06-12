import { describe, expect, it } from "vitest";
import { paginationParamsSchema } from "./pagination";

describe("paginationParamsSchema", () => {
  it("applies the default page size when omitted", () => {
    const parsed = paginationParamsSchema.parse({});
    expect(parsed.pageSize).toBe(20);
  });

  it("rejects a page size above the maximum", () => {
    expect(() => paginationParamsSchema.parse({ pageSize: 1000 })).toThrow();
  });
});
