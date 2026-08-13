import type { FamilyNotification } from "../api";
import type { ChipKind } from "../data/mock";
import type { IconName } from "../icons";
import type { TintName } from "../theme";

/**
 * Presentation adapter for family notifications. The gateway returns a `type`
 * (the stable comms vocabulary: "absence" | "attendance" | "payment" | …) rather
 * than the rich category the design mock carried, so the UI derives its icon,
 * tint, sender label and chip from that type.
 */
export interface NotificationKindMeta {
  /** Sender/category label shown in the row meta (the API carries no "from"). */
  label: string;
  icon: IconName;
  tint: TintName;
  chip: ChipKind;
}

// A new backend type must never break the UI, so unknown types fall through to a
// neutral "Escuela" style rather than crashing on a missing key.
const KIND_META: Record<string, NotificationKindMeta> = {
  absence: { label: "Asistencia", icon: "calendar", tint: "orange", chip: "warn" },
  attendance: { label: "Asistencia", icon: "calendar", tint: "forest", chip: "ok" },
  payment: { label: "Pagos", icon: "wallet", tint: "lime", chip: "ok" },
};

const DEFAULT_META: NotificationKindMeta = {
  label: "Escuela",
  icon: "megaphone",
  tint: "purple",
  chip: "purple",
};

/** Display meta for a notification type, falling back to a neutral school style. */
export function notificationKind(type: string): NotificationKindMeta {
  return KIND_META[type] ?? DEFAULT_META;
}

/**
 * es-MX day+month label from an RFC3339 instant; empty on an unparseable date.
 * The list uses the `short` month, the detail the `long` one.
 */
export function formatNotificationDate(iso: string, style: "short" | "long"): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  return date.toLocaleDateString("es-MX", { day: "numeric", month: style });
}

/**
 * A notification is unread when the server has not stamped a read instant AND the
 * user has not marked it read locally in this session.
 *
 * NOTE(pendiente-backend: POST /comms/notifications/{id}/read): the guardian role
 * holds only `family:read`, not `comms:write`, so the mark-read endpoint is not
 * reachable from the family app. Read state is therefore session-local until a
 * guardian-reachable acknowledge route exists. See .claude/docs/PLAN-mock-data-audit.md.
 */
export function isUnread(n: FamilyNotification, readIds: ReadonlySet<string>): boolean {
  return n.readAt === "" && !readIds.has(n.id);
}
