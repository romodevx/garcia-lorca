import { describe, expect, it, jest } from "@jest/globals";

import { endSession } from "./endSession";

describe("endSession", () => {
  it("ends the provider session too when the provider is in use", async () => {
    const ours = jest.fn<() => Promise<void>>().mockResolvedValue(undefined);
    const providerSignOut = jest.fn<() => Promise<void>>().mockResolvedValue(undefined);

    await endSession(ours, true, async () => ({ providerSignOut }));

    expect(ours).toHaveBeenCalled();
    // Otherwise a fresh ID token could be minted with no password and exchanged
    // for a whole new session: "Cerrar sesión" would log nobody out.
    expect(providerSignOut).toHaveBeenCalled();
  });

  it("clears our tokens even when the provider cannot be reached", async () => {
    const ours = jest.fn<() => Promise<void>>().mockResolvedValue(undefined);
    const providerSignOut = jest.fn<() => Promise<void>>().mockRejectedValue(new Error("offline"));

    await expect(endSession(ours, true, async () => ({ providerSignOut }))).resolves.toBeUndefined();
    // A provider that cannot be reached must never strand somebody in a session
    // they asked to leave.
    expect(ours).toHaveBeenCalled();
  });

  it("does not touch the provider when it is not in use", async () => {
    const ours = jest.fn<() => Promise<void>>().mockResolvedValue(undefined);
    const load = jest.fn<() => Promise<{ providerSignOut: () => Promise<void> }>>();

    await endSession(ours, false, load);

    expect(ours).toHaveBeenCalled();
    expect(load).not.toHaveBeenCalled();
  });
});
