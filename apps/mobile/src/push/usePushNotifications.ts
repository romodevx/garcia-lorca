import * as Notifications from "expo-notifications";
import { router } from "expo-router";
import { useCallback, useEffect, useRef } from "react";
import { getAccessToken } from "../api";
import { useAppState } from "../state/AppState";
import { requestFreshData } from "../state/freshness";
import { registerForPush } from "./register";

/** The push payload comms sends when a teacher saves an exception. */
interface AttendancePush {
  type: "attendance";
  studentId: string;
  date: string;
}

const ATTENDANCE_ROUTE = "/(tabs)/asistencia";

function attendancePayload(notification: Notifications.Notification): AttendancePush | null {
  const data = notification.request.content.data as Record<string, unknown> | null | undefined;
  if (data?.type !== "attendance" || typeof data.studentId !== "string") {
    return null;
  }
  return { type: "attendance", studentId: data.studentId, date: String(data.date ?? "") };
}

let handlerInstalled = false;

/** A push arriving with the app open still shows its banner (it is news the parent wants). */
function installForegroundHandler(): void {
  if (handlerInstalled) {
    return;
  }
  handlerInstalled = true;
  Notifications.setNotificationHandler({
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
 *  - registers the phone once per signed-in session (after login or on a cold
 *    start with a stored session);
 *  - an attendance push arriving in the foreground reloads what is on screen;
 *  - tapping one (also the tap that cold-started the app) selects that child
 *    and opens Asistencia.
 */
export function usePushNotifications(): void {
  const { selectedChildId, selectChild } = useAppState();
  const registeredThisSession = useRef(false);
  const handledResponses = useRef(new Set<string>());

  useEffect(installForegroundHandler, []);

  // A loaded child means a signed-in family; losing it means the session ended.
  const signedIn = selectedChildId !== null;
  useEffect(() => {
    if (!signedIn) {
      registeredThisSession.current = false;
      return;
    }
    if (registeredThisSession.current) {
      return;
    }
    registeredThisSession.current = true;
    void registerForPush();
  }, [signedIn]);

  const openAttendance = useCallback(
    (response: Notifications.NotificationResponse) => {
      const id = response.notification.request.identifier;
      const payload = attendancePayload(response.notification);
      if (payload === null || handledResponses.current.has(id) || getAccessToken() === null) {
        return;
      }
      handledResponses.current.add(id);
      selectChild(payload.studentId);
      router.navigate(ATTENDANCE_ROUTE);
      requestFreshData();
    },
    [selectChild],
  );

  useEffect(() => {
    const received = Notifications.addNotificationReceivedListener((notification) => {
      if (attendancePayload(notification) !== null) {
        requestFreshData();
      }
    });
    const tapped = Notifications.addNotificationResponseReceivedListener(openAttendance);
    void Notifications.getLastNotificationResponseAsync()
      .then((response) => {
        if (response) {
          openAttendance(response);
        }
      })
      .catch(() => undefined);
    return () => {
      received.remove();
      tapped.remove();
    };
  }, [openAttendance]);
}
