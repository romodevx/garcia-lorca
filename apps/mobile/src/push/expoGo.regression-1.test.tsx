// Regression: ISSUE-001 — the family app crashed at launch in Expo Go on Android
// Found by /qa on 2026-09-27
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-26-t5.md
//
// Since SDK 53, expo-notifications' push calls THROW in Expo Go on Android
// (remote push was removed there). The root layout's push wiring called them,
// so the whole app died before the first screen. In Expo Go on Android the push
// code must never touch that module: push is simply "unsupported". The mock
// below throws on load, so ANY use of the module fails this suite.
import { renderHook, waitFor } from "@testing-library/react-native";
import { type ReactNode } from "react";
import { Platform } from "react-native";
import { clearTokens, saveTokens } from "../api";
import { Providers } from "../Providers";
import { registerForPush } from "./register";
import { usePushNotifications } from "./usePushNotifications";

jest.mock("expo-notifications", () => {
  throw new Error("expo-notifications: Android Push notifications was removed from Expo Go with SDK 53.");
});

jest.mock("expo", () => ({ ...jest.requireActual("expo"), isRunningInExpoGo: () => true }));

jest.mock("expo-constants", () => ({
  __esModule: true,
  default: { expoConfig: { extra: { eas: { projectId: "proj-123" } } }, easConfig: null },
}));

const wrapper = ({ children }: { children: ReactNode }): ReactNode => <Providers>{children}</Providers>;

describe("push in Expo Go on Android", () => {
  const originalOS = Platform.OS;
  beforeEach(async () => {
    Object.defineProperty(Platform, "OS", { configurable: true, get: () => "android" });
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    globalThis.fetch = jest.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ students: [], notifications: [] }),
    })) as unknown as typeof fetch;
  });
  afterEach(async () => {
    Object.defineProperty(Platform, "OS", { configurable: true, get: () => originalOS });
    await clearTokens();
  });

  it("mounts the push wiring without loading expo-notifications", async () => {
    await renderHook(() => usePushNotifications(), { wrapper });
    // The providers kept running past the push wiring and asked for the children.
    await waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
  });

  it("reports registration as unsupported instead of throwing", async () => {
    await expect(registerForPush()).resolves.toBe("unsupported");
  });
});
