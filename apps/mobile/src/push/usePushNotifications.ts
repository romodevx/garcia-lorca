import type { NotificationResponse } from "expo-notifications";
import { router } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import { getAccessToken } from "../api";
import { useAppState } from "../state/AppState";
import { requestFreshData } from "../state/freshness";
import { loadNotifications } from "./notificationsModule";
import { familyPush, type FamilyPush } from "./pushPayload";
import { usePushRegistration } from "./usePushRegistration";

const ATTENDANCE_ROUTE = "/(tabs)/asistencia";
const GRADES_ROUTE = "/(tabs)/calificaciones";
const REPORT_CARD_ROUTE = "/boleta/[id]";
const INBOX_ROUTE = "/(tabs)/avisos";

/** A tapped push waiting for the child list, with the id of the tap that queued it. */
interface PendingTap {
  push: FamilyPush;
  responseId: string;
}

/** Opens the screen a push is about; the child is already selected. */
function openPushTarget({ push, responseId }: PendingTap): void {
  if (push.type === "attendance") {
    router.navigate(ATTENDANCE_ROUTE);
    return;
  }
  router.navigate({ pathname: GRADES_ROUTE, params: { vista: "boletas", k: responseId } });
  router.push({ pathname: REPORT_CARD_ROUTE, params: { id: push.reportCardId } });
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
 * Wires family pushes into the app. Mount once, inside the providers:
 *  - keeps the phone registered while a family is signed in (usePushRegistration);
 *  - an attendance or boleta push arriving in the foreground reloads what is on screen;
 *  - tapping one (also the tap that cold-started the app) selects that child and
 *    opens Asistencia, or Calificaciones → Boletas → that boleta — but only once
 *    the child list has loaded, and only when the child is one of THIS
 *    guardian's; any other push opens Avisos instead.
 */
export function usePushNotifications(): void {
  const { children: linkedChildren, childLoading, selectedChildId, selectChild } = useAppState();
  const handledResponses = useRef(new Set<string>());
  const [pendingTap, setPendingTap] = useState<PendingTap | null>(null);

  useEffect(installForegroundHandler, []);

  // A loaded child means a signed-in family; losing it means the session ended.
  usePushRegistration(selectedChildId !== null);

  const queueTap = useCallback((response: NotificationResponse) => {
    const id = response.notification.request.identifier;
    const push = familyPush(response.notification);
    if (push === null || handledResponses.current.has(id)) {
      return;
    }
    handledResponses.current.add(id);
    setPendingTap({ push, responseId: id });
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
    const { studentId } = pendingTap.push;
    if (!linkedChildren.some((c) => c.id === studentId)) {
      router.navigate(INBOX_ROUTE);
      return;
    }
    selectChild(studentId);
    openPushTarget(pendingTap);
    requestFreshData();
  }, [pendingTap, childLoading, linkedChildren, selectChild]);

  useEffect(() => {
    const notifications = loadNotifications();
    if (notifications === null) {
      return;
    }
    const received = notifications.addNotificationReceivedListener((notification) => {
      if (familyPush(notification) !== null) {
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
