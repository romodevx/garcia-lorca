import Constants from "expo-constants";
import * as Notifications from "expo-notifications";
import { Platform } from "react-native";
import { registerDevice, type DevicePlatform } from "../api/devices";
import { markPermissionAsked, saveRegisteredToken, wasPermissionAsked } from "./pushStore";

/** The Android channel attendance alerts are posted on. */
export const ATTENDANCE_CHANNEL_ID = "asistencia";

/** What a registration attempt ended in. Never surfaced to the parent as an error. */
export type PushRegistration = "registered" | "no-project" | "denied" | "unsupported" | "failed";

let missingProjectLogged = false;

/** Test hook: the "no projectId" log is once per process. */
export function resetMissingProjectLogForTests(): void {
  missingProjectLogged = false;
}

function easProjectId(): string | null {
  const eas = Constants.expoConfig?.extra?.eas as { projectId?: unknown } | undefined;
  return typeof eas?.projectId === "string" && eas.projectId !== "" ? eas.projectId : null;
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

async function ensureAttendanceChannel(): Promise<void> {
  if (Platform.OS !== "android") {
    return;
  }
  await Notifications.setNotificationChannelAsync(ATTENDANCE_CHANNEL_ID, {
    name: "Asistencia",
    importance: Notifications.AndroidImportance.HIGH,
  });
}

/**
 * Permission, asked politely and ONCE: if the parent already answered (or the
 * OS says we may not ask), we take the answer and never prompt again.
 */
async function hasPermission(): Promise<boolean> {
  const current = await Notifications.getPermissionsAsync();
  if (current.granted) {
    return true;
  }
  if (!current.canAskAgain || (await wasPermissionAsked())) {
    return false;
  }
  await markPermissionAsked();
  const answer = await Notifications.requestPermissionsAsync();
  return answer.granted;
}

/**
 * Registers this phone for attendance pushes: channel → permission → Expo push
 * token → `POST /family/devices`. Without an EAS projectId (true until the owner
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
  if (platform === null) {
    return "unsupported";
  }
  try {
    await ensureAttendanceChannel();
    if (!(await hasPermission())) {
      return "denied";
    }
    const { data: token } = await Notifications.getExpoPushTokenAsync({ projectId });
    await registerDevice(token, platform);
    await saveRegisteredToken(token);
    return "registered";
  } catch {
    return "failed";
  }
}
