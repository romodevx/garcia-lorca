import { apiFetch } from "./client";
import { ApiError } from "./errors";

/**
 * One notification for a guardian's child, as returned by the gateway's
 * self-scoped family surface (`GET /api/v1/family/notifications`). The set of
 * children is derived from the authenticated principal's guardian links, so the
 * client cannot widen it — see services/gateway family.go.
 *
 * `readAt` is empty while the notification is unread; the UI distinguishes read
 * state on that field.
 */
export interface FamilyNotification {
  id: string;
  studentId: string;
  /** Stable kind the client switches on for its icon: "absence" | "attendance" | "payment" | … */
  type: string;
  title: string;
  body: string;
  /** RFC3339 creation instant; the aggregate is newest-first. */
  createdAt: string;
  /** RFC3339 read instant, or "" while unread. */
  readAt: string;
}

interface NotificationsEnvelope {
  notifications: FamilyNotification[] | null;
}

/**
 * Loads every notification of every child the caller is linked to, newest first.
 * Requires the `family:read` permission plus the comms + students modules (the
 * gateway enforces this; a disabled module 404s). Throws an ApiError on any
 * non-2xx.
 */
export async function listFamilyNotifications(): Promise<FamilyNotification[]> {
  const res = await apiFetch("/api/v1/family/notifications");
  if (!res.ok) {
    throw new ApiError(res.status, "failed to load notifications");
  }
  const data = (await res.json()) as NotificationsEnvelope;
  return data.notifications ?? [];
}
