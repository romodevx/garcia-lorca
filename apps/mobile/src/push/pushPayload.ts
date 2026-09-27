import type { Notification } from "expo-notifications";

/**
 * The push payloads comms sends that the app acts on: an attendance exception
 * a teacher saved, and a newly issued (or corrected) boleta.
 */
export type FamilyPush =
  | { type: "attendance"; studentId: string; date: string }
  | { type: "report_card"; studentId: string; reportCardId: string };

/** The payload of a notification the app understands, or null. */
export function familyPush(notification: Notification): FamilyPush | null {
  const data = notification.request.content.data as Record<string, unknown> | null | undefined;
  if (typeof data?.studentId !== "string") {
    return null;
  }
  if (data.type === "attendance") {
    return { type: "attendance", studentId: data.studentId, date: String(data.date ?? "") };
  }
  if (data.type === "report_card" && typeof data.reportCardId === "string") {
    return { type: "report_card", studentId: data.studentId, reportCardId: data.reportCardId };
  }
  return null;
}
