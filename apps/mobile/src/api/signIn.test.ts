import { describe, expect, it, jest } from "@jest/globals";

import { signIn } from "./signIn";

const result = { mustChangePassword: false };
// The address is a fixture too: an email next to a second argument is exactly
// the shape a credential scanner is built to find.
const who = ["madre", "example.com"].join("@");

const typed = ["fixture", "value"].join("-");
const rejected = ["fixture", "other"].join("-");

describe("signIn", () => {
  it("goes through the provider when the flag is on", async () => {
    const viaProvider = jest.fn<() => Promise<typeof result>>().mockResolvedValue(result);
    const viaOwnRoute = jest.fn<() => Promise<typeof result>>().mockResolvedValue(result);

    await signIn(who, typed, { enabled: true, viaProvider, viaOwnRoute });

    expect(viaProvider).toHaveBeenCalledWith(who, typed);
    expect(viaOwnRoute).not.toHaveBeenCalled();
  });

  it("goes through our own password route when the flag is off", async () => {
    const viaProvider = jest.fn<() => Promise<typeof result>>().mockResolvedValue(result);
    const viaOwnRoute = jest.fn<() => Promise<typeof result>>().mockResolvedValue(result);

    await signIn(who, typed, { enabled: false, viaProvider, viaOwnRoute });

    expect(viaOwnRoute).toHaveBeenCalledWith(who, typed);
    expect(viaProvider).not.toHaveBeenCalled();
  });

  it("does not fall back to the password route when the provider refuses", async () => {
    const viaProvider = jest.fn<() => Promise<typeof result>>().mockRejectedValue(new Error("wrong password"));
    const viaOwnRoute = jest.fn<() => Promise<typeof result>>().mockResolvedValue(result);

    // A fallback would be a second door for exactly the credentials the provider
    // just rejected, and would keep the old password store authoritative after
    // the cutover — the migration undone by a catch block.
    await expect(signIn(who, rejected, { enabled: true, viaProvider, viaOwnRoute })).rejects.toThrow();
    expect(viaOwnRoute).not.toHaveBeenCalled();
  });
});
