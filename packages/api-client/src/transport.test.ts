import { describe, expect, it } from "vitest";
import { createTransport } from "./transport";

describe("createTransport", () => {
  it("returns a Connect transport with an unary method", () => {
    const transport = createTransport({ baseUrl: "https://api.example.test" });
    expect(typeof transport.unary).toBe("function");
  });
});
