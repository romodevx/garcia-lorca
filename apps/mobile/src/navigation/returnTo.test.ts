import { HOME_ROUTE } from "./routes";
import { destinationAfterSignIn } from "./returnTo";

describe("destinationAfterSignIn", () => {
  it.each([
    ["/avisos", "/avisos"],
    ["/aviso/123", "/aviso/123"],
    ["/boleta/r1", "/boleta/r1"],
  ])("returns the in-app route %s the family was opening", (requested, expected) => {
    expect(destinationAfterSignIn(requested)).toBe(expected);
  });

  it.each([
    ["nothing requested", undefined],
    ["an empty value", ""],
    ["the login screen itself", "/"],
    ["a protocol-relative host", "//evil.example/avisos"],
    ["a backslash host", "/\\evil.example"],
    ["an absolute URL", "https://evil.example/avisos"],
    ["another app's scheme", "otherapp://avisos"],
    ["a relative path", "avisos"],
  ])("falls back to Inicio for %s", (_case, requested) => {
    expect(destinationAfterSignIn(requested)).toBe(HOME_ROUTE);
  });

  it("takes the first value when the param arrives as an array", () => {
    expect(destinationAfterSignIn(["/avisos", "/cuenta"])).toBe("/avisos");
  });
});
