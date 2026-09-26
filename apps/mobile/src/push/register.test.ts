import AsyncStorage from "@react-native-async-storage/async-storage";
import * as Notifications from "expo-notifications";
import { Platform } from "react-native";
import { clearTokens, saveTokens } from "../api";
import { readRegisteredToken } from "./pushStore";
import { ATTENDANCE_CHANNEL_ID, registerForPush, resetMissingProjectLogForTests } from "./register";

const mockExpoConfig: { extra?: { eas?: { projectId?: string } } } = {};
jest.mock("expo-constants", () => ({
  __esModule: true,
  default: {
    get expoConfig() {
      return mockExpoConfig;
    },
  },
}));

jest.mock("expo-notifications", () => ({
  getPermissionsAsync: jest.fn(),
  requestPermissionsAsync: jest.fn(),
  getExpoPushTokenAsync: jest.fn(),
  setNotificationChannelAsync: jest.fn(async () => null),
  AndroidImportance: { HIGH: 4 },
}));

const mocked = Notifications as jest.Mocked<typeof Notifications>;
type Perm = Awaited<ReturnType<typeof Notifications.getPermissionsAsync>>;
const perm = (status: "granted" | "denied" | "undetermined", canAskAgain = true): Perm =>
  ({ status, granted: status === "granted", canAskAgain, expires: "never" }) as unknown as Perm;

function mockGateway(): jest.Mock {
  const fetchMock = jest.fn(async () => ({ ok: true, status: 204, json: async () => ({}) }));
  globalThis.fetch = fetchMock as unknown as typeof fetch;
  return fetchMock;
}

describe("registerForPush", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    await AsyncStorage.clear();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
    mockExpoConfig.extra = { eas: { projectId: "proj-123" } };
    resetMissingProjectLogForTests();
    mocked.getExpoPushTokenAsync.mockResolvedValue({ type: "expo", data: "ExponentPushToken[xyz]" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("asks once, gets the Expo token with the EAS projectId and registers it with the gateway", async () => {
    const fetchMock = mockGateway();
    mocked.getPermissionsAsync.mockResolvedValue(perm("undetermined"));
    mocked.requestPermissionsAsync.mockResolvedValue(perm("granted"));

    await expect(registerForPush()).resolves.toBe("registered");

    expect(mocked.requestPermissionsAsync).toHaveBeenCalledTimes(1);
    expect(mocked.getExpoPushTokenAsync).toHaveBeenCalledWith({ projectId: "proj-123" });
    const [url, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toContain("/api/v1/family/devices");
    expect(JSON.parse(String(init.body))).toEqual({ token: "ExponentPushToken[xyz]", platform: Platform.OS });
    expect(await readRegisteredToken()).toBe("ExponentPushToken[xyz]");
  });

  it("does not ask again when already granted", async () => {
    mockGateway();
    mocked.getPermissionsAsync.mockResolvedValue(perm("granted"));

    await expect(registerForPush()).resolves.toBe("registered");
    expect(mocked.requestPermissionsAsync).not.toHaveBeenCalled();
  });

  it("stops quietly when the parent says no, and never nags on the next session", async () => {
    const fetchMock = mockGateway();
    mocked.getPermissionsAsync.mockResolvedValue(perm("undetermined"));
    mocked.requestPermissionsAsync.mockResolvedValue(perm("denied", false));

    await expect(registerForPush()).resolves.toBe("denied");
    await expect(registerForPush()).resolves.toBe("denied");

    expect(mocked.requestPermissionsAsync).toHaveBeenCalledTimes(1);
    expect(mocked.getExpoPushTokenAsync).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("skips registration without a projectId: no prompt, no request, one dev log", async () => {
    const fetchMock = mockGateway();
    mockExpoConfig.extra = {};
    const warn = jest.spyOn(console, "warn").mockImplementation(() => undefined);

    await expect(registerForPush()).resolves.toBe("no-project");
    await expect(registerForPush()).resolves.toBe("no-project");

    expect(mocked.getPermissionsAsync).not.toHaveBeenCalled();
    expect(mocked.requestPermissionsAsync).not.toHaveBeenCalled();
    expect(fetchMock).not.toHaveBeenCalled();
    expect(warn).toHaveBeenCalledTimes(1);
    warn.mockRestore();
  });

  it("never throws when the token or the gateway fails", async () => {
    mockGateway();
    mocked.getPermissionsAsync.mockResolvedValue(perm("granted"));
    mocked.getExpoPushTokenAsync.mockRejectedValue(new Error("no FCM"));

    await expect(registerForPush()).resolves.toBe("failed");
  });

  it("creates the Android 'asistencia' channel before asking", async () => {
    mockGateway();
    const original = Platform.OS;
    Object.defineProperty(Platform, "OS", { value: "android", configurable: true });
    mocked.getPermissionsAsync.mockResolvedValue(perm("granted"));

    await registerForPush();

    expect(mocked.setNotificationChannelAsync).toHaveBeenCalledWith(
      ATTENDANCE_CHANNEL_ID,
      expect.objectContaining({ name: "Asistencia" }),
    );
    Object.defineProperty(Platform, "OS", { value: original, configurable: true });
  });
});
