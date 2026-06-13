import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import {
  announcements as ANNOUNCEMENTS,
  attendance as ATTENDANCE,
  child as CHILD,
  levels as LEVELS,
  type AttendanceStatus,
  type Child,
  type Level,
  type LevelId,
} from "../data/mock";

/**
 * App-wide interactive state for the parent app: the persistent child context,
 * the selected level (drives grade rendering), and the mutable session state
 * that screens update live — paid status, attendance excuse overrides, and the
 * set of read announcements. Mirrors the prototype's top-level shell state.
 *
 * This is in-memory mock state; the real implementation will hydrate it from
 * the gateway and persist mutations through the API client.
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

  /** Set of announcement ids the user has read. */
  readIds: ReadonlySet<number>;
  /** Count of still-unread announcements. */
  unreadCount: number;
  markRead: (id: number) => void;
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
  const [readIds, setReadIds] = useState<ReadonlySet<number>>(() => new Set());

  const pay = useCallback(() => setPaid(true), []);

  const attendanceStatus = useCallback(
    (day: number): AttendanceStatus | undefined => overrides[day] ?? ATTENDANCE.days[day],
    [overrides],
  );

  const excuseDay = useCallback((day: number) => {
    setOverrides((prev) => ({ ...prev, [day]: "excused" }));
  }, []);

  const markRead = useCallback((id: number) => {
    setReadIds((prev) => {
      const next = new Set(prev);
      next.add(id);
      return next;
    });
  }, []);

  const markAllRead = useCallback(() => {
    setReadIds(new Set(ANNOUNCEMENTS.map((a) => a.id)));
  }, []);

  const unreadCount = useMemo(
    () => ANNOUNCEMENTS.filter((a) => a.unread && !readIds.has(a.id)).length,
    [readIds],
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
