import { HOME_ROUTE } from "./routes";
import { destinationAfterSignIn } from "./returnTo";

describe("destinationAfterSignIn", () => {
  it.each([
    ["/avisos", "/avisos"],
    ["/aviso/123", "/aviso/123"],
    ["/boleta/r1", "/boleta/r1"],
    ["/boleta/r1?studentId=s1", "/boleta/r1?studentId=s1"],
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
    ["an encoded protocol-relative host", "%2F%2Fevil"],
    ["a slash hidden behind an encoded slash", "/%2F/evil"],
    ["an encoded backslash host", "/%5Cevil"],
    ["an upper-case javascript: URL", "JAVASCRIPT:x"],
    ["a host behind leading whitespace", " //evil"],
    ["the login screen carrying its own destination", "/?destino=//evil"],
    ["the login screen carrying the session-ended notice", "/?motivo=sesion"],
    ["the login screen with a fragment", "/#x"],
    ["malformed percent-encoding", "/%E0%A4%A"],
  ])("falls back to Inicio for %s", (_case, requested) => {
    expect(destinationAfterSignIn(requested)).toBe(HOME_ROUTE);
  });

  it("takes the first value when the param arrives as an array", () => {
    expect(destinationAfterSignIn(["/avisos", "/cuenta"])).toBe("/avisos");
  });
});
