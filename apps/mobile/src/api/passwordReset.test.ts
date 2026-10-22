import { ApiError } from "./errors";
import { requestPasswordReset } from "./passwordReset";

describe("requestPasswordReset", () => {
  it("posts the address to the public password-reset route", async () => {
    const fetchMock = jest.fn(async () => ({ ok: true, status: 204 }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await requestPasswordReset("laura@correo.com");

    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toContain("/api/v1/auth/password-reset");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ email: "laura@correo.com" });
    // Public route: no session goes with it.
    expect((init.headers as Record<string, string>).Authorization).toBeUndefined();
  });

  it("throws a typed error on a non-2xx, so the screen can tell a 429", async () => {
    globalThis.fetch = jest.fn(async () => ({ ok: false, status: 429 })) as unknown as typeof fetch;

    await expect(requestPasswordReset("laura@correo.com")).rejects.toEqual(new ApiError(429, "password reset failed"));
  });
});
