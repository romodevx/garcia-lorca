import { isRejectedValue, type GradeSheet, type GradeSheetClient, type MonthlyGradeKey } from "../../grades/monthly";

export type CellStatus =
  | { kind: "idle" }
  | { kind: "saving" }
  /** The server confirmed the LATEST value asked for. */
  | { kind: "saved" }
  /** Network or server trouble: the same value can be sent again. */
  | { kind: "failed" }
  /** The server refused the value itself; its words go on the row. */
  | { kind: "rejected"; message: string };

type Grade = number | null;

/** A sheet (assignment + month) or a whole assignment. */
export interface SaveScope {
  assignmentId: string;
  month?: string;
}

interface Cell {
  key: MonthlyGradeKey;
  /** What the server holds, as far as this page knows. */
  confirmed: Grade;
  /** What the teacher last asked for. */
  desired: Grade;
  /** The value of the request in flight, when there is one. */
  sent: Grade | undefined;
  status: CellStatus;
  /** Text that failed the scale check on leaving the field — never sent. */
  invalidText: string | undefined;
}

const IDLE: CellStatus = { kind: "idle" };

const idOf = (k: MonthlyGradeKey): string => `${k.assignmentId}|${k.month}|${k.enrollmentId}`;
const inScope = (k: MonthlyGradeKey, s: SaveScope | undefined): boolean =>
  s === undefined || (k.assignmentId === s.assignmentId && (s.month === undefined || k.month === s.month));
const isUnsaved = (c: Cell): boolean => c.status.kind === "failed" || c.status.kind === "rejected" || c.invalidText !== undefined;
const format = (g: Grade): string => (g === null ? "" : String(g));

/**
 * Every grade cell's save, for the whole capture screen — it lives above the
 * month and assignment switches, so nothing typed is forgotten by one.
 *
 * Per cell (assignment, month, enrollment) at most ONE request is in flight.
 * A value asked for meanwhile replaces any earlier waiting one and is sent
 * when the current request settles; "saved" means the server confirmed the
 * latest value asked for. A plain external store: React reads it through
 * `useSyncExternalStore` (`subscribe` + `getVersion`).
 */
export class GradeSaveStore {
  private readonly cells = new Map<string, Cell>();
  private readonly listeners = new Set<() => void>();
  private version = 0;

  constructor(private readonly client: GradeSheetClient) {}

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  getVersion = (): number => this.version;

  /** Saves (or, for null, clears) a cell. `baseline` is the loaded value, for a cell not seen yet. */
  commit(key: MonthlyGradeKey, next: Grade, baseline: Grade): void {
    const cell = this.cellFor(key, baseline);
    cell.invalidText = undefined;
    // The same value again is a no-op — also on a failed cell, whose resend is the explicit retry.
    if (cell.sent === undefined && cell.desired === next) {
      this.notify();
      return;
    }
    cell.desired = next;
    if (cell.sent !== undefined) {
      this.notify();
      return;
    }
    this.send(cell);
  }

  retry(key: MonthlyGradeKey): void {
    const cell = this.cells.get(idOf(key));
    if (cell?.status.kind === "failed" && cell.sent === undefined) this.send(cell);
  }

  retryAll(): void {
    for (const cell of this.cells.values()) this.retry(cell.key);
  }

  /** The teacher is typing again: the server's old complaint no longer applies. */
  dismissRejection(key: MonthlyGradeKey): void {
    const cell = this.cells.get(idOf(key));
    if (cell?.status.kind !== "rejected") return;
    // What the server refused was never stored: compare the next commit with what was.
    cell.desired = cell.confirmed;
    cell.status = IDLE;
    this.notify();
  }

  markInvalid(key: MonthlyGradeKey, text: string, baseline: Grade = null): void {
    this.cellFor(key, baseline).invalidText = text;
    this.notify();
  }

  statusOf(key: MonthlyGradeKey): CellStatus {
    return this.cells.get(idOf(key))?.status ?? IDLE;
  }

  confirmedOf(key: MonthlyGradeKey, fallback: Grade): Grade {
    const cell = this.cells.get(idOf(key));
    return cell === undefined ? fallback : cell.confirmed;
  }

  /** The text to put back in a remounted field: an unsent or unconfirmed value. */
  draftOf(key: MonthlyGradeKey): string | undefined {
    const cell = this.cells.get(idOf(key));
    if (cell === undefined) return undefined;
    if (cell.invalidText !== undefined) return cell.invalidText;
    const unconfirmed = cell.sent !== undefined || cell.status.kind === "failed" || cell.status.kind === "rejected";
    return unconfirmed ? format(cell.desired) : undefined;
  }

  pendingCount(): number {
    return this.count((c) => c.sent !== undefined);
  }

  unsavedCount(scope?: SaveScope): number {
    return this.count((c) => inScope(c.key, scope) && isUnsaved(c));
  }

  retryableCount(): number {
    return this.count((c) => c.status.kind === "failed" && c.sent === undefined);
  }

  /** Resolves once no save of `scope` is in flight — a GET after that is not stale. */
  settled(scope: SaveScope): Promise<void> {
    const busy = (): boolean => [...this.cells.values()].some((c) => c.sent !== undefined && inScope(c.key, scope));
    if (!busy()) return Promise.resolve();
    return new Promise((resolve) => {
      const stop = this.subscribe(() => {
        if (busy()) return;
        stop();
        resolve();
      });
    });
  }

  /** A freshly loaded sheet is the truth for its clean cells; unsaved drafts stay. */
  adopt(sheet: GradeSheet): void {
    for (const row of sheet.rows) {
      const id = idOf({ assignmentId: sheet.assignmentId, month: sheet.month, enrollmentId: row.enrollmentId });
      const cell = this.cells.get(id);
      if (cell === undefined || cell.sent !== undefined) continue;
      if (isUnsaved(cell)) cell.confirmed = row.value;
      else this.cells.delete(id);
    }
    this.notify();
  }

  private cellFor(key: MonthlyGradeKey, baseline: Grade): Cell {
    const id = idOf(key);
    let cell = this.cells.get(id);
    if (cell === undefined) {
      cell = { key, confirmed: baseline, desired: baseline, sent: undefined, status: IDLE, invalidText: undefined };
      this.cells.set(id, cell);
    }
    return cell;
  }

  private send(cell: Cell): void {
    const value = cell.desired;
    cell.sent = value;
    cell.status = { kind: "saving" };
    this.notify();
    const work: Promise<unknown> = value === null ? this.client.remove(cell.key) : this.client.save({ ...cell.key, value });
    work.then(
      () => {
        cell.confirmed = value;
        this.settle(cell, value, { kind: "saved" });
      },
      (err: unknown) => this.settle(cell, value, isRejectedValue(err) ? { kind: "rejected", message: err.detail } : { kind: "failed" }),
    );
  }

  /** The request for `value` is over: send the newer wish, or report how it went. */
  private settle(cell: Cell, value: Grade, outcome: CellStatus): void {
    cell.sent = undefined;
    if (cell.desired !== value) {
      this.send(cell);
      return;
    }
    cell.status = outcome;
    this.notify();
  }

  private count(predicate: (c: Cell) => boolean): number {
    let n = 0;
    for (const cell of this.cells.values()) if (predicate(cell)) n += 1;
    return n;
  }

  private notify(): void {
    this.version += 1;
    for (const listener of [...this.listeners]) listener();
  }
}
