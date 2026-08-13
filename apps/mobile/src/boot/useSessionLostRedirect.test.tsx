import { act, cleanup, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { type ReactElement } from "react";
import { clearTokens, emitSessionLost, saveTokens } from "@/src/api";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import { useSessionLostRedirect } from "./useSessionLostRedirect";

/** Mounts the hook under test; renders nothing itself. */
function Harness(): ReactElement | null {
  useSessionLostRedirect();
  return null;
}

describe("useSessionLostRedirect", () => {
  // Auto-cleanup is off in this project, so unmount any mounted tree between
  // tests — otherwise a prior Harness stays subscribed and leaks its redirect.
  afterEach(cleanup);

  beforeEach(() => {
    (router.replace as jest.Mock).mockClear();
  });

  it("forces navigation back to Login when the session is lost", async () => {
    await renderWithProviders(<Harness />);

    await act(async () => {
      emitSessionLost();
    });

    expect(router.replace).toHaveBeenCalledWith("/");
  });

  it("refreshes BOTH the child slice and notifications on session-lost (bug_006)", async () => {
    // A token is present so each refresh actually reaches the network — that lets
    // us observe that BOTH slices were re-fetched (the child slice would otherwise
    // keep the previous guardian's data).
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    const fetchMock = jest.fn(async (url: string) => {
      const u = String(url);
      if (u.includes("/family/students")) return { ok: true, status: 200, json: async () => ({ students: [] }) };
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    });
    globalThis.fetch = fetchMock as unknown as typeof fetch;

    await renderWithProviders(<Harness />);
    // Let the provider's initial mount loads settle, then watch only the reaction.
    await waitFor(() => expect(fetchMock).toHaveBeenCalled());
    fetchMock.mockClear();

    await act(async () => {
      emitSessionLost();
    });

    await waitFor(() => {
      const urls = fetchMock.mock.calls.map((c) => String(c[0]));
      expect(urls.some((u) => u.includes("/family/students"))).toBe(true);
      expect(urls.some((u) => u.includes("/family/notifications"))).toBe(true);
    });
    expect(router.replace).toHaveBeenCalledWith("/");

    await clearTokens();
  });

  it("stops reacting once unmounted (subscription is cleaned up)", async () => {
    const { unmount } = await renderWithProviders(<Harness />);
    await act(async () => {
      unmount();
    });

    await act(async () => {
      emitSessionLost();
    });

    expect(router.replace).not.toHaveBeenCalled();
  });
});
