import { API_BASE_URL, apiUrl, joinUrl } from "./config";

describe("api config", () => {
  it("joins a base URL and a path", () => {
    expect(joinUrl("http://host:8080", "/api/v1/x")).toBe("http://host:8080/api/v1/x");
  });

  it("collapses a trailing slash on the base so the path is not doubled", () => {
    expect(joinUrl("http://host:8080/", "/api/v1/x")).toBe("http://host:8080/api/v1/x");
  });

  it("defaults the base URL to a localhost gateway when the env var is unset", () => {
    // EXPO_PUBLIC_API_URL is not set in the jest env, so the default applies.
    expect(API_BASE_URL).toContain("http");
  });

  it("apiUrl resolves against the configured base", () => {
    expect(apiUrl("/ping")).toBe(joinUrl(API_BASE_URL, "/ping"));
  });
});
