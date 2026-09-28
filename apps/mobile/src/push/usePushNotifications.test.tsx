import { act, renderHook, waitFor } from "@testing-library/react-native";
import * as Notifications from "expo-notifications";
import { router } from "expo-router";
import { type ReactNode } from "react";
import { clearTokens, saveTokens } from "../api";
import { Providers } from "../Providers";
import { useAppState } from "../state/AppState";
import { requestFreshData, useFreshDataSignal } from "../state/freshness";
import { registerForPush } from "./register";
import { usePushNotifications } from "./usePushNotifications";

jest.mock("./register", () => ({ registerForPush: jest.fn(async () => "registered") }));

type Listener = (event: unknown) => void;
const mockListeners: { received: Listener | null; response: Listener | null; token: Listener | null } = {
  received: null,
  response: null,
  token: null,
};
const mockTokenRemove = jest.fn();
jest.mock("expo-notifications", () => ({
  setNotificationHandler: jest.fn(),
  addNotificationReceivedListener: jest.fn((l: Listener) => {
    mockListeners.received = l;
    return { remove: jest.fn() };
  }),
  addNotificationResponseReceivedListener: jest.fn((l: Listener) => {
    mockListeners.response = l;
    return { remove: jest.fn() };
  }),
  getLastNotificationResponseAsync: jest.fn(async () => null),
  clearLastNotificationResponse: jest.fn(),
  addPushTokenListener: jest.fn((l: Listener) => {
    mockListeners.token = l;
    return { remove: mockTokenRemove };
  }),
}));

const children = [
  { id: "s1", firstName: "Mateo", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "3° A" },
  { id: "s2", firstName: "Lucía", lastName: "Herrera", level: "Primaria", levelId: "l1", gradeGroup: "1° B" },
];

function mockGateway(): void {
  globalThis.fetch = jest.fn(async (url: string) => {
    if (String(url).endsWith("/family/students")) {
      return { ok: true, status: 200, json: async () => ({ students: children }) };
    }
    return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
  }) as unknown as typeof fetch;
}

const wrapper = ({ children: ui }: { children: ReactNode }): ReactNode => <Providers>{ui}</Providers>;

const notification = (data: Record<string, unknown>, id = "n1"): unknown => ({
  request: { identifier: id, content: { data } },
});

describe("usePushNotifications", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    mockListeners.received = null;
    mockListeners.response = null;
    mockListeners.token = null;
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    mockGateway();
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("registers for push once the signed-in family has loaded", async () => {
    await renderHook(() => usePushNotifications(), { wrapper });
    await waitFor(() => expect(registerForPush).toHaveBeenCalledTimes(1));
  });

  it("does not register while signed out", async () => {
    await clearTokens();
    await renderHook(() => usePushNotifications(), { wrapper });
    expect(registerForPush).not.toHaveBeenCalled();
  });

  it("reloads the on-screen data when an attendance push arrives in the foreground", async () => {
    const reload = jest.fn(async () => undefined);
    await renderHook(
      () => {
        usePushNotifications();
        useFreshDataSignal(reload);
      },
      { wrapper },
    );

    await act(async () => mockListeners.received?.(notification({ type: "other" })));
    expect(reload).not.toHaveBeenCalled();

    await act(async () => mockListeners.received?.(notification({ type: "attendance", studentId: "s1", date: "2026-09-28" })));
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("opens Asistencia for the child named in a tapped attendance push", async () => {
    const { result } = await renderHook(
      () => {
        usePushNotifications();
        return useAppState();
      },
      { wrapper },
    );
    await waitFor(() => expect(result.current.selectedChildId).toBe("s1"));

    await act(async () =>
      mockListeners.response?.({
        notification: notification({ type: "attendance", studentId: "s2", date: "2026-09-28" }),
        actionIdentifier: "default",
      }),
    );

    expect(result.current.selectedChildId).toBe("s2");
    expect(router.navigate).toHaveBeenCalledWith("/(tabs)/asistencia");
  });

  it("handles the push that cold-started the app", async () => {
    (Notifications.getLastNotificationResponseAsync as jest.Mock).mockResolvedValueOnce({
      notification: notification({ type: "attendance", studentId: "s2", date: "2026-09-28" }, "cold"),
      actionIdentifier: "default",
    });

    await renderHook(() => usePushNotifications(), { wrapper });

    await waitFor(() => expect(router.navigate).toHaveBeenCalledWith("/(tabs)/asistencia"));
  });

  it("clears the cold-start response once handled, so it never replays", async () => {
    (Notifications.getLastNotificationResponseAsync as jest.Mock).mockResolvedValueOnce({
      notification: notification({ type: "attendance", studentId: "s2", date: "2026-09-28" }, "cold"),
      actionIdentifier: "default",
    });

    await renderHook(() => usePushNotifications(), { wrapper });

    await waitFor(() => expect(Notifications.clearLastNotificationResponse).toHaveBeenCalled());
  });

  it("ignores a tapped push for a child who is not this guardian's, and opens Avisos instead", async () => {
    const { result } = await renderHook(
      () => {
        usePushNotifications();
        return useAppState();
      },
      { wrapper },
    );
    await waitFor(() => expect(result.current.selectedChildId).toBe("s1"));

    await act(async () =>
      mockListeners.response?.({
        notification: notification({ type: "attendance", studentId: "stranger", date: "2026-09-28" }),
        actionIdentifier: "default",
      }),
    );

    expect(result.current.selectedChildId).toBe("s1");
    expect(router.navigate).not.toHaveBeenCalledWith("/(tabs)/asistencia");
    expect(router.navigate).toHaveBeenCalledWith("/(tabs)/avisos");
  });

  it("on a cold start waits for the child list before selecting the tapped child", async () => {
    let releaseStudents: () => void = () => undefined;
    const studentsGate = new Promise<void>((resolve) => (releaseStudents = resolve));
    globalThis.fetch = jest.fn(async (url: string) => {
      if (String(url).endsWith("/family/students")) {
        await studentsGate;
        return { ok: true, status: 200, json: async () => ({ students: children }) };
      }
      return { ok: true, status: 200, json: async () => ({ notifications: [] }) };
    }) as unknown as typeof fetch;
    (Notifications.getLastNotificationResponseAsync as jest.Mock).mockResolvedValueOnce({
      notification: notification({ type: "attendance", studentId: "s2", date: "2026-09-28" }, "cold-wait"),
      actionIdentifier: "default",
    });

    const { result } = await renderHook(
      () => {
        usePushNotifications();
        return useAppState();
      },
      { wrapper },
    );
    await act(async () => undefined);
    expect(router.navigate).not.toHaveBeenCalled();

    await act(async () => releaseStudents());

    await waitFor(() => expect(router.navigate).toHaveBeenCalledWith("/(tabs)/asistencia"));
    expect(result.current.selectedChildId).toBe("s2");
  });

  it("opens Calificaciones → Boletas → that boleta for the child named in a tapped boleta push", async () => {
    const { result } = await renderHook(
      () => {
        usePushNotifications();
        return useAppState();
      },
      { wrapper },
    );
    await waitFor(() => expect(result.current.selectedChildId).toBe("s1"));

    await act(async () =>
      mockListeners.response?.({
        notification: notification({ type: "report_card", studentId: "s2", reportCardId: "rc9" }, "rc-push"),
        actionIdentifier: "default",
      }),
    );

    expect(result.current.selectedChildId).toBe("s2");
    expect(router.navigate).toHaveBeenCalledWith({ pathname: "/(tabs)/calificaciones", params: { vista: "boletas", k: "rc-push" } });
    expect(router.push).toHaveBeenCalledWith({ pathname: "/boleta/[id]", params: { id: "rc9", studentId: "s2" } });
  });

  it("ignores a boleta push for a child who is not this guardian's", async () => {
    const { result } = await renderHook(
      () => {
        usePushNotifications();
        return useAppState();
      },
      { wrapper },
    );
    await waitFor(() => expect(result.current.selectedChildId).toBe("s1"));

    await act(async () =>
      mockListeners.response?.({
        notification: notification({ type: "report_card", studentId: "stranger", reportCardId: "rc9" }),
        actionIdentifier: "default",
      }),
    );

    expect(result.current.selectedChildId).toBe("s1");
    expect(router.push).not.toHaveBeenCalled();
    expect(router.navigate).toHaveBeenCalledWith("/(tabs)/avisos");
  });

  it("reloads the on-screen data when a boleta push arrives in the foreground", async () => {
    const reload = jest.fn(async () => undefined);
    await renderHook(
      () => {
        usePushNotifications();
        useFreshDataSignal(reload);
      },
      { wrapper },
    );

    await act(async () => mockListeners.received?.(notification({ type: "report_card", studentId: "s1", reportCardId: "rc9" })));

    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("retries a failed registration when the app comes back, but not a finished one", async () => {
    (registerForPush as jest.Mock).mockResolvedValueOnce("failed");
    await renderHook(() => usePushNotifications(), { wrapper });
    await waitFor(() => expect(registerForPush).toHaveBeenCalledTimes(1));

    await act(async () => requestFreshData());
    await waitFor(() => expect(registerForPush).toHaveBeenCalledTimes(2));

    await act(async () => requestFreshData());
    await act(async () => undefined);
    expect(registerForPush).toHaveBeenCalledTimes(2);
  });

  it("re-registers when the push service rotates the token, and stops listening on unmount", async () => {
    const { unmount } = await renderHook(() => usePushNotifications(), { wrapper });
    await waitFor(() => expect(registerForPush).toHaveBeenCalledTimes(1));

    await act(async () => mockListeners.token?.({ type: "ios", data: "new-device-token" }));
    await waitFor(() => expect(registerForPush).toHaveBeenCalledTimes(2));

    await act(async () => unmount());
    expect(mockTokenRemove).toHaveBeenCalled();
  });
});
