import Constants from "expo-constants";
import { Platform } from "react-native";
import { registerDevice, type DevicePlatform } from "../api/devices";
import { loadNotifications, type NotificationsModule } from "./notificationsModule";
import { markPermissionAsked, saveRegisteredToken, wasPermissionAsked } from "./pushStore";
import { retryPendingUnregisters } from "./unregister";

/** The Android channel attendance alerts are posted on. */
export const ATTENDANCE_CHANNEL_ID = "asistencia";

/** What a registration attempt ended in. Never surfaced to the parent as an error. */
export type PushRegistration = "registered" | "no-project" | "denied" | "unsupported" | "failed";

let missingProjectLogged = false;

/** Test hook: the "no projectId" log is once per process. */
export function resetMissingProjectLogForTests(): void {
  missingProjectLogged = false;
}

function nonEmpty(value: unknown): string | null {
  return typeof value === "string" && value !== "" ? value : null;
}

/** The EAS build's projectId first (set on EAS builds), then the app config's. */
function easProjectId(): string | null {
  const extra = Constants.expoConfig?.extra?.eas as { projectId?: unknown } | undefined;
  return nonEmpty(Constants.easConfig?.projectId) ?? nonEmpty(extra?.projectId);
}

function devicePlatform(): DevicePlatform | null {
  return Platform.OS === "ios" || Platform.OS === "android" ? Platform.OS : null;
}

function logMissingProjectOnce(): void {
  if (missingProjectLogged) {
    return;
  }
  missingProjectLogged = true;
  if (__DEV__) {
    console.warn("[push] no EAS projectId in app config; push registration skipped until EAS is configured.");
  }
}

async function ensureAttendanceChannel(notifications: NotificationsModule): Promise<void> {
  if (Platform.OS !== "android") {
    return;
  }
  await notifications.setNotificationChannelAsync(ATTENDANCE_CHANNEL_ID, {
    name: "Asistencia",
    importance: notifications.AndroidImportance.HIGH,
  });
}

/**
 * Permission, asked politely and ONCE: if the parent already answered (or the
 * OS says we may not ask), we take the answer and never prompt again.
 */
async function hasPermission(notifications: NotificationsModule): Promise<boolean> {
  const current = await notifications.getPermissionsAsync();
  if (current.granted) {
    return true;
  }
  if (!current.canAskAgain || (await wasPermissionAsked())) {
    return false;
  }
  await markPermissionAsked();
  const answer = await notifications.requestPermissionsAsync();
  return answer.granted;
}

/**
 * Registers this phone for attendance pushes: pending deletes → channel →
 * permission → Expo push token → `POST /family/devices`. A token whose delete
 * never reached the gateway is retried FIRST; while one still fails, nothing is
 * registered (that late DELETE could remove the new registration). Without an EAS projectId (true until the owner
 * configures EAS) it skips before prompting, so nobody is asked for a
 * permission that cannot yet do anything. Never throws.
 */
export async function registerForPush(): Promise<PushRegistration> {
  const projectId = easProjectId();
  if (projectId === null) {
    logMissingProjectOnce();
    return "no-project";
  }
  const platform = devicePlatform();
  const notifications = loadNotifications();
  if (platform === null || notifications === null) {
    return "unsupported";
  }
  try {
    if (!(await retryPendingUnregisters())) {
      return "failed";
    }
    await ensureAttendanceChannel(notifications);
    if (!(await hasPermission(notifications))) {
      return "denied";
    }
    const { data: token } = await notifications.getExpoPushTokenAsync({ projectId });
    await registerDevice(token, platform);
    await saveRegisteredToken(token);
    return "registered";
  } catch {
    return "failed";
  }
}
