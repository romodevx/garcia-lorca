import AsyncStorage from "@react-native-async-storage/async-storage";
import { act, cleanup, waitFor } from "@testing-library/react-native";
import { router } from "expo-router";
import { type ReactElement } from "react";
import { clearTokens, emitSessionLost, saveTokens } from "@/src/api";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import { readPendingUnregisters, readRegisteredToken, saveRegisteredToken } from "@/src/push/pushStore";
import { SESSION_ENDED_ROUTE } from "@/src/navigation/routes";
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

    expect(router.replace).toHaveBeenCalledWith(SESSION_ENDED_ROUTE);
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
    expect(router.replace).toHaveBeenCalledWith(SESSION_ENDED_ROUTE);

    await clearTokens();
  });

  it("unregisters this phone's push token best-effort, keeping it as a pending delete (no Bearer left)", async () => {
    await saveRegisteredToken("ExponentPushToken[xyz]");
    const fetchMock = jest.fn(async () => ({ ok: true, status: 200, json: async () => ({ students: [], notifications: [] }) }));
    globalThis.fetch = fetchMock as unknown as typeof fetch;
    await renderWithProviders(<Harness />);

    await act(async () => {
      emitSessionLost();
    });

    await waitFor(async () => expect(await readRegisteredToken()).toBeNull());
    expect(await readPendingUnregisters()).toEqual(["ExponentPushToken[xyz]"]);
    const deletes = fetchMock.mock.calls.filter((c) => String((c as unknown[])[0]).includes("/family/devices"));
    expect(deletes).toHaveLength(0);
  });

  // CC-303 gate: a dead session must not leave its child remembered for the next guardian.
  it("forgets the remembered child when the session is lost", async () => {
    await AsyncStorage.setItem("gl.selectedChildId", "s2");
    await renderWithProviders(<Harness />);

    await act(async () => {
      emitSessionLost();
    });

    await waitFor(async () => expect(await AsyncStorage.getItem("gl.selectedChildId")).toBeNull());
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
