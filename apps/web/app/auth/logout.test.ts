import { afterEach, describe, expect, it, vi } from "vitest";

import { endSession } from "./logout";

afterEach(() => { vi.restoreAllMocks(); vi.resetModules(); });

/**
 * Ending a session has to end BOTH of them.
 *
 * Our cookies are only half the session once the provider is in play. The SDK
 * can hold a provider session of its own, and while it lives a fresh ID token
 * can be minted with no password and exchanged at /api/v1/auth/google for a
 * complete new set of cookies.
 *
 * The scenario that makes this urgent is the one the panel is built for: a
 * shared computer at the front desk. Somebody clicks "Cerrar sesión" and walks
 * away believing they are out. If the provider session survives, the next
 * person to sit down can be them again.
 */

describe("endSession", () => {
  it("ends the provider session too when the provider is in use", async () => {
    const apiLogout = vi.fn().mockResolvedValue(undefined);
    const providerSignOut = vi.fn().mockResolvedValue(undefined);

    await endSession("http://gw", apiLogout, true, async () => ({ providerSignOut }));

    expect(apiLogout).toHaveBeenCalledWith("http://gw");
    expect(providerSignOut).toHaveBeenCalled();
  });

  it("does not load the provider SDK when it is not in use", async () => {
    const apiLogout = vi.fn().mockResolvedValue(undefined);
    const load = vi.fn();

    await endSession("http://gw", apiLogout, false, load);

    // Loading it would pull 468 KB of JavaScript to sign out of a session that
    // was never started.
    expect(load).not.toHaveBeenCalled();
    expect(apiLogout).toHaveBeenCalled();
  });

  it("still clears our cookies when the provider sign-out fails", async () => {
    const apiLogout = vi.fn().mockResolvedValue(undefined);

    await endSession("http://gw", apiLogout, true, async () => ({
      providerSignOut: vi.fn().mockRejectedValue(new Error("offline")),
    }));

    // A provider that cannot be reached must never strand somebody in a session
    // they asked to leave. Our half is what the gateway checks on every request.
    expect(apiLogout).toHaveBeenCalled();
  });

  it("still ends the provider session when our logout call fails", async () => {
    const providerSignOut = vi.fn().mockResolvedValue(undefined);

    await endSession("http://gw", vi.fn().mockRejectedValue(new Error("offline")), true, async () => ({
      providerSignOut,
    }));

    // The dangerous half is the one that can mint a NEW session. It gets ended
    // even when the other call fails.
    expect(providerSignOut).toHaveBeenCalled();
  });
});
