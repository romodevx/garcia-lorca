import { apiFetch } from "./client";
import { ApiError } from "./errors";

/**
 * Push-device registration on the guardian's family surface. The gateway binds
 * the token to the authenticated guardian, so comms can reach this phone when a
 * child's attendance is recorded as an exception.
 */

export type DevicePlatform = "ios" | "android";

/** Registers this phone's Expo push token for the signed-in guardian. */
export async function registerDevice(token: string, platform: DevicePlatform): Promise<void> {
  const res = await apiFetch("/api/v1/family/devices", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ token, platform }),
  });
  if (!res.ok) {
    throw new ApiError(res.status, "failed to register device");
  }
}

/** Removes this phone's push token, so a signed-out phone stops receiving the child's news. */
export async function unregisterDevice(token: string): Promise<void> {
  const res = await apiFetch(`/api/v1/family/devices/${encodeURIComponent(token)}`, { method: "DELETE" });
  if (!res.ok) {
    throw new ApiError(res.status, "failed to unregister device");
  }
}
