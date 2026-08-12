import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { listFamilyNotifications, type FamilyNotification } from "../api";
import {
  attendance as ATTENDANCE,
  child as CHILD,
  levels as LEVELS,
  type AttendanceStatus,
  type Child,
  type Level,
  type LevelId,
} from "../data/mock";
import { isUnread } from "../screens/notificationView";

/**
 * App-wide interactive state for the parent app.
 *
 * The notifications slice is REAL: it hydrates from the gateway's self-scoped
 * family surface (`GET /api/v1/family/notifications`) on mount. Everything else
 * (child context, level, paid status, attendance overrides) is still in-memory
 * mock state pending its own family-scoped endpoints — see
 * .claude/docs/PLAN-mock-data-audit.md (B2–B6).
 */
interface AppStateValue {
  child: Child;
  level: Level;
  levelId: LevelId;
  setLevelId: (id: LevelId) => void;

  /** True once the (mock) tuition payment has been made. */
  paid: boolean;
  pay: () => void;

  /** Per-day attendance overrides (e.g. a justified absence). */
  overrides: Record<number, AttendanceStatus>;
  /** Returns the effective status for a day, applying overrides. */
  attendanceStatus: (day: number) => AttendanceStatus | undefined;
  /** Marks a day as excused (one-tap "Justificar"). */
  excuseDay: (day: number) => void;

  /** Family notifications loaded from the gateway (newest first). */
  notifications: FamilyNotification[];
  /** True while the first (or a manual) notification load is in flight. */
  notificationsLoading: boolean;
  /** True when the last notification load failed. */
  notificationsError: boolean;
  /** Re-fetches the notification inbox (pull-to-refresh / retry). */
  refreshNotifications: () => Promise<void>;

  /** Ids of notifications marked read locally this session. */
  readIds: ReadonlySet<string>;
  /** Count of still-unread notifications. */
  unreadCount: number;
  markRead: (id: string) => void;
  markAllRead: () => void;
}

const AppStateContext = createContext<AppStateValue | null>(null);

export interface AppStateProviderProps {
  children: ReactNode;
  /** Initial level (tests/demo). Defaults to primaria (numeric grading). */
  initialLevelId?: LevelId;
}

export function AppStateProvider({
  children,
  initialLevelId = "primaria",
}: AppStateProviderProps): ReactNode {
  const [levelId, setLevelId] = useState<LevelId>(initialLevelId);
  const [paid, setPaid] = useState(false);
  const [overrides, setOverrides] = useState<Record<number, AttendanceStatus>>({});

  const [notifications, setNotifications] = useState<FamilyNotification[]>([]);
  const [notificationsLoading, setNotificationsLoading] = useState(true);
  const [notificationsError, setNotificationsError] = useState(false);
  // Read state is session-local: the guardian role cannot reach the comms
  // mark-read endpoint (see notificationView isUnread). Ids are notification ids.
  const [readIds, setReadIds] = useState<ReadonlySet<string>>(() => new Set());

  const pay = useCallback(() => setPaid(true), []);

  const attendanceStatus = useCallback(
    (day: number): AttendanceStatus | undefined => overrides[day] ?? ATTENDANCE.days[day],
    [overrides],
  );

  const excuseDay = useCallback((day: number) => {
    setOverrides((prev) => ({ ...prev, [day]: "excused" }));
  }, []);

  const refreshNotifications = useCallback(async () => {
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
    void refreshNotifications();
  }, [refreshNotifications]);

  const markRead = useCallback((id: string) => {
    setReadIds((prev) => {
      const next = new Set(prev);
      next.add(id);
      return next;
    });
  }, []);

  const markAllRead = useCallback(() => {
    setReadIds((prev) => {
      const next = new Set(prev);
      for (const n of notifications) {
        next.add(n.id);
      }
      return next;
    });
  }, [notifications]);

  const unreadCount = useMemo(
    () => notifications.filter((n) => isUnread(n, readIds)).length,
    [notifications, readIds],
  );

  const value = useMemo<AppStateValue>(
    () => ({
      child: CHILD,
      level: LEVELS[levelId],
      levelId,
      setLevelId,
      paid,
      pay,
      overrides,
      attendanceStatus,
      excuseDay,
      notifications,
      notificationsLoading,
      notificationsError,
      refreshNotifications,
      readIds,
      unreadCount,
      markRead,
      markAllRead,
    }),
    [
      levelId,
      paid,
      pay,
      overrides,
      attendanceStatus,
      excuseDay,
      notifications,
      notificationsLoading,
      notificationsError,
      refreshNotifications,
      readIds,
      unreadCount,
      markRead,
      markAllRead,
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
