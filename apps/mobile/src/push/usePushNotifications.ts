import type { Notification, NotificationResponse } from "expo-notifications";
import { router } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import { getAccessToken } from "../api";
import { useAppState } from "../state/AppState";
import { requestFreshData } from "../state/freshness";
import { loadNotifications } from "./notificationsModule";
import { usePushRegistration } from "./usePushRegistration";

/** The push payload comms sends when a teacher saves an exception. */
interface AttendancePush {
  type: "attendance";
  studentId: string;
  date: string;
}

const ATTENDANCE_ROUTE = "/(tabs)/asistencia";
const INBOX_ROUTE = "/(tabs)/avisos";

function attendancePayload(notification: Notification): AttendancePush | null {
  const data = notification.request.content.data as Record<string, unknown> | null | undefined;
  if (data?.type !== "attendance" || typeof data.studentId !== "string") {
    return null;
  }
  return { type: "attendance", studentId: data.studentId, date: String(data.date ?? "") };
}

let handlerInstalled = false;

/** A push arriving with the app open still shows its banner (it is news the parent wants). */
function installForegroundHandler(): void {
  const notifications = loadNotifications();
  if (handlerInstalled || notifications === null) {
    return;
  }
  handlerInstalled = true;
  notifications.setNotificationHandler({
    handleNotification: async () => ({
      shouldShowBanner: true,
      shouldShowList: true,
      shouldPlaySound: false,
      shouldSetBadge: false,
    }),
  });
}

/**
 * Wires attendance pushes into the app. Mount once, inside the providers:
 *  - keeps the phone registered while a family is signed in (usePushRegistration);
 *  - an attendance push arriving in the foreground reloads what is on screen;
 *  - tapping one (also the tap that cold-started the app) opens Asistencia for
 *    that child — but only once the child list has loaded, and only when the
 *    child is one of THIS guardian's; any other push opens Avisos instead.
 */
export function usePushNotifications(): void {
  const { children: linkedChildren, childLoading, selectedChildId, selectChild } = useAppState();
  const handledResponses = useRef(new Set<string>());
  const [pendingTap, setPendingTap] = useState<AttendancePush | null>(null);

  useEffect(installForegroundHandler, []);

  // A loaded child means a signed-in family; losing it means the session ended.
  usePushRegistration(selectedChildId !== null);

  const queueTap = useCallback((response: NotificationResponse) => {
    const id = response.notification.request.identifier;
    const payload = attendancePayload(response.notification);
    if (payload === null || handledResponses.current.has(id)) {
      return;
    }
    handledResponses.current.add(id);
    setPendingTap(payload);
  }, []);

  useEffect(() => {
    if (pendingTap === null || childLoading) {
      return;
    }
    setPendingTap(null);
    // Consumed: a remount (or the next cold start) must not replay it.
    loadNotifications()?.clearLastNotificationResponse();
    if (getAccessToken() === null) {
      return;
    }
    if (!linkedChildren.some((c) => c.id === pendingTap.studentId)) {
      router.navigate(INBOX_ROUTE);
      return;
    }
    selectChild(pendingTap.studentId);
    router.navigate(ATTENDANCE_ROUTE);
    requestFreshData();
  }, [pendingTap, childLoading, linkedChildren, selectChild]);

  useEffect(() => {
    const notifications = loadNotifications();
    if (notifications === null) {
      return;
    }
    const received = notifications.addNotificationReceivedListener((notification) => {
      if (attendancePayload(notification) !== null) {
        requestFreshData();
      }
    });
    const tapped = notifications.addNotificationResponseReceivedListener(queueTap);
    void notifications.getLastNotificationResponseAsync()
      .then((response) => {
        if (response) {
          queueTap(response);
        }
      })
      .catch(() => undefined);
    return () => {
      received.remove();
      tapped.remove();
    };
  }, [queueTap]);
}
