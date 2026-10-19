import { describe, expect, it } from "vitest";

import { HttpError, TOO_MANY_ATTEMPTS, tooManyAttempts } from "./httpError";

/**
 * C2-012: the gateway throttles at 10 a minute and answers 429. Every screen
 * read that as "check your connection", which sends the person to look at
 * their wifi instead of waiting a minute.
 */
describe("tooManyAttempts", () => {
  it.each([
    ["an HttpError 429", new HttpError(429, "HTTP 429")],
    ["any module error carrying status 429", Object.assign(new Error("Too Many Requests"), { status: 429 })],
  ])("names the throttle for %s", (_label, err) => {
    expect(tooManyAttempts(err)).toBe(TOO_MANY_ATTEMPTS);
    expect(TOO_MANY_ATTEMPTS).toBe("Demasiados intentos. Espera un minuto y vuelve a intentarlo.");
  });

  it.each([
    ["a 401", new HttpError(401, "HTTP 401")],
    ["a 503", Object.assign(new Error("x"), { status: 503 })],
    ["a dropped connection", new TypeError("Failed to fetch")],
    ["no error at all", undefined],
    ["a status that is not a number", { status: "429" }],
  ])("says nothing for %s", (_label, err) => {
    expect(tooManyAttempts(err)).toBeUndefined();
  });
});
