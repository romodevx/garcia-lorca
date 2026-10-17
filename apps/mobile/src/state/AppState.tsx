import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  getAccessToken,
  listChildren,
  listFamilyNotifications,
  markNotificationRead,
  type FamilyNotification,
  type FamilyStudent,
} from "../api";
import { isUnread } from "../screens/notificationView";
import { useFreshDataSignal } from "./freshness";
import { readSelectedChild, rememberSelectedChild } from "./selectedChildStore";

/**
 * App-wide interactive state for the parent app.
 *
 * Both slices are REAL: the child slice bootstraps the guardian's linked children
 * from the gateway's self-scoped family surface (`GET /api/v1/family/students`)
 * and selects one — the header's switcher changes it and the choice survives a
 * relaunch (ISSUE-CC-303); the notifications slice hydrates from
 * `GET /api/v1/family/notifications`. Both are auth-gated: with no access token
 * they settle empty and fire no request. Per-child reads (account, attendance,
 * grades) live in their own hooks keyed on the selected child id (see
 * ./familyData.ts).
 */
interface AppStateValue {
  /** The guardian's linked children. */
  children: FamilyStudent[];
  /** The currently selected child, or null while loading / when none are linked. */
  child: FamilyStudent | null;
  /** Id of the selected child, or null. */
  selectedChildId: string | null;
  /** Selects a child by id and remembers it for the next launch. */
  selectChild: (id: string) => void;
  /** True while the first (or a manual) child load is in flight. */
  childLoading: boolean;
  /** True when the last child load failed. */
  childError: boolean;
  /** Re-fetches the linked children (retry / post-login). */
  refreshChildren: () => Promise<void>;

  /** Family notifications loaded from the gateway (newest first). */
  notifications: FamilyNotification[];
  /** True while the first (or a manual) notification load is in flight. */
  notificationsLoading: boolean;
  /** True when the last notification load failed. */
  notificationsError: boolean;
  /** Re-fetches the notification inbox (pull-to-refresh / retry). */
  refreshNotifications: () => Promise<void>;

  /** Ids of notifications acknowledged (optimistically) this session. */
  readIds: ReadonlySet<string>;
  /** Count of still-unread notifications. */
  unreadCount: number;
  /** Acknowledges one notification (persists via the family API). */
  markRead: (id: string) => void;
}

const AppStateContext = createContext<AppStateValue | null>(null);

export interface AppStateProviderProps {
  children: ReactNode;
}

export function AppStateProvider({ children }: AppStateProviderProps): ReactNode {
  const [childList, setChildList] = useState<FamilyStudent[]>([]);
  const [selectedChildId, setSelectedChildId] = useState<string | null>(null);
  const [childLoading, setChildLoading] = useState(true);
  const [childError, setChildError] = useState(false);

  const [notifications, setNotifications] = useState<FamilyNotification[]>([]);
  const [notificationsLoading, setNotificationsLoading] = useState(true);
  const [notificationsError, setNotificationsError] = useState(false);
  // Read state is optimistic: acknowledging a notification persists via the family
  // API (POST /family/notifications/{id}/read); readIds gives immediate feedback
  // until the next refetch stamps readAt server-side. Ids are notification ids.
  const [readIds, setReadIds] = useState<ReadonlySet<string>>(() => new Set());

  // Every child load (and the signed-out reset a logout runs) takes a new
  // generation; a load that is no longer the latest writes nothing, so the
  // previous guardian's slow list never repaints the slice after a logout.
  const childGeneration = useRef(0);

  const refreshChildren = useCallback(async () => {
    const generation = (childGeneration.current += 1);
    const isCurrent = (): boolean => generation === childGeneration.current;
    // Signed out: no token to authenticate the family request, so treat the child
    // list as cleanly empty rather than firing an unauthenticated call. This also
    // empties the slice after logout.
    if (getAccessToken() === null) {
      setChildList([]);
      setSelectedChildId(null);
      setChildLoading(false);
      setChildError(false);
      return;
    }
    setChildLoading(true);
    setChildError(false);
    try {
      const [list, remembered] = await Promise.all([listChildren(), readSelectedChild()]);
      if (!isCurrent()) return;
      setChildList(list);
      // Keep the current selection if it still exists; else the child chosen
      // before the app was closed (ISSUE-CC-303); else the first child.
      const linked = (id: string | null): id is string => id !== null && list.some((c) => c.id === id);
      setSelectedChildId((prev) => {
        if (linked(prev)) return prev;
        if (linked(remembered)) return remembered;
        return list[0]?.id ?? null;
      });
    } catch {
      if (isCurrent()) setChildError(true);
    } finally {
      if (isCurrent()) setChildLoading(false);
    }
  }, []);

  const refreshNotifications = useCallback(async () => {
    // Same auth gate as refreshChildren: no token ⇒ cleanly empty, no request.
    if (getAccessToken() === null) {
      setNotifications([]);
      setNotificationsLoading(false);
      setNotificationsError(false);
      return;
    }
    setNotificationsLoading(true);
    setNotificationsError(false);
    try {
      setNotifications(await listFamilyNotifications());
    } catch {
      setNotificationsError(true);
    } finally {
      setNotificationsLoading(false);
    }
  }, []);

  useEffect(() => {
    void refreshChildren();
    void refreshNotifications();
  }, [refreshChildren, refreshNotifications]);

  // Back from the background or an attendance push: the inbox may have news.
  useFreshDataSignal(refreshNotifications);

  const selectChild = useCallback((id: string) => {
    setSelectedChildId(id);
    rememberSelectedChild(id);
  }, []);

  const markRead = useCallback((id: string) => {
    // Optimistic: reflect the acknowledgement immediately, then persist it. A
    // failed persist reverts so the badge doesn't silently drift from the server.
    setReadIds((prev) => new Set(prev).add(id));
    void markNotificationRead(id).catch(() => {
      setReadIds((prev) => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    });
  }, []);

  const unreadCount = useMemo(
    () => notifications.filter((n) => isUnread(n, readIds)).length,
    [notifications, readIds],
  );

  const child = useMemo(
    () => childList.find((c) => c.id === selectedChildId) ?? null,
    [childList, selectedChildId],
  );

  const value = useMemo<AppStateValue>(
    () => ({
      children: childList,
      child,
      selectedChildId,
      selectChild,
      childLoading,
      childError,
      refreshChildren,
      notifications,
      notificationsLoading,
      notificationsError,
      refreshNotifications,
      readIds,
      unreadCount,
      markRead,
    }),
    [
      childList,
      child,
      selectedChildId,
      selectChild,
      childLoading,
      childError,
      refreshChildren,
      notifications,
      notificationsLoading,
      notificationsError,
      refreshNotifications,
      readIds,
      unreadCount,
      markRead,
    ],
  );

  return <AppStateContext.Provider value={value}>{children}</AppStateContext.Provider>;
}

export function useAppState(): AppStateValue {
  const ctx = useContext(AppStateContext);
  if (ctx === null) {
    throw new Error("useAppState must be used within an <AppStateProvider>.");
  }
  return ctx;
}
