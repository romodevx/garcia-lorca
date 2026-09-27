import { isRunningInExpoGo } from "expo";
import { Platform } from "react-native";

/** The expo-notifications API, loaded only where it can load. */
export type NotificationsModule = typeof import("expo-notifications");

/**
 * Whether this runtime can use expo-notifications. Since SDK 53 its push calls
 * (addPushTokenListener, getExpoPushTokenAsync…) THROW in Expo Go on Android —
 * remote push was removed there — and the throw from the root layout's effect
 * killed the whole app. Same check the library itself uses. Dev builds, store
 * builds and Expo Go on iOS are fine.
 */
export function pushAvailable(): boolean {
  return !(Platform.OS === "android" && isRunningInExpoGo());
}

/**
 * expo-notifications, or null where it cannot load. Every push call site goes
 * through here instead of a static import.
 */
export function loadNotifications(): NotificationsModule | null {
  if (!pushAvailable()) {
    return null;
  }
  // eslint-disable-next-line @typescript-eslint/no-require-imports -- never loaded where push cannot work
  return require("expo-notifications") as NotificationsModule;
}
