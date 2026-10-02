import { describe, expect, it, vi } from "vitest";

import { type GradeSheetClient, type PeriodSheet, type SavedPeriodGrade } from "../../grades/periodSheet";
import { GradeRequestError } from "../../grades/requestError";
import { GradeSaveStore } from "./gradeSaveStore";

interface Call {
  kind: "save" | "remove";
  value: number | null;
  settle: { resolve: () => void; reject: (e: unknown) => void };
}

/** A client whose every request waits until the test settles it. */
function manualClient(): { client: GradeSheetClient; calls: Call[] } {
  const calls: Call[] = [];
  const pending = <T,>(kind: Call["kind"], value: number | null, result: T): Promise<T> =>
    new Promise<T>((resolve, reject) => calls.push({ kind, value, settle: { resolve: () => resolve(result), reject } }));
  const client: GradeSheetClient = {
    load: vi.fn(),
    save: (i) => pending<SavedPeriodGrade>("save", i.value, { ...i, updatedAt: "x" }),
    remove: () => pending<void>("remove", null, undefined),
  };
  return { client, calls };
}

const KEY = { assignmentId: "a1", periodId: "p1", enrollmentId: "en1" };
const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

function sheet(value: number | null): PeriodSheet {
  return {
    assignmentId: "a1",
    periodId: "p1",
    periods: [{ id: "p1", ordinal: 1, name: "1.er trimestre", startsOn: "2026-08-24", endsOn: "2026-11-30" }],
    currentPeriodId: "p1",
    today: "2026-10-01",
    closedPeriodIds: [],
    scale: { min: 5, max: 10, decimals: 0, passing: 6 },
    gradingEnabled: true,
    editable: true,
    rows: [{ enrollmentId: "en1", studentId: "s1", name: "Ada", value, updatedAt: null, rowEditable: true }],
  };
}

describe("GradeSaveStore — one request per cell, latest wish wins", () => {
  it("8 then 9: the 9 waits for the 8, and only the 9 reads as saved", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);

    store.commit(KEY, 8, null);
    store.commit(KEY, 9, null);
    expect(calls).toHaveLength(1);
    expect(store.statusOf(KEY).kind).toBe("saving");

    calls[0]!.settle.resolve();
    await flush();
    expect(calls).toHaveLength(2);
    expect(calls[1]!.value).toBe(9);
    expect(store.statusOf(KEY).kind).toBe("saving");

    calls[1]!.settle.resolve();
    await flush();
    expect(store.statusOf(KEY).kind).toBe("saved");
    expect(store.confirmedOf(KEY, null)).toBe(9);
  });

  it("8 then clear: the delete follows the save, and the cell ends empty", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);

    store.commit(KEY, 8, null);
    store.commit(KEY, null, null);
    expect(calls).toHaveLength(1);

    calls[0]!.settle.resolve();
    await flush();
    expect(calls[1]!.kind).toBe("remove");
    calls[1]!.settle.resolve();
    await flush();
    expect(store.confirmedOf(KEY, 7)).toBeNull();
    expect(store.statusOf(KEY).kind).toBe("saved");
  });

  it("8, 9, back to 8: nothing more is sent once the 8 lands", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);

    store.commit(KEY, 8, null);
    store.commit(KEY, 9, null);
    store.commit(KEY, 8, null);
    calls[0]!.settle.resolve();
    await flush();

    expect(calls).toHaveLength(1);
    expect(store.statusOf(KEY).kind).toBe("saved");
  });

  it("a failed 8 with a 9 waiting sends the 9 instead of reporting the 8", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);

    store.commit(KEY, 8, null);
    store.commit(KEY, 9, null);
    calls[0]!.settle.reject(new Error("network"));
    await flush();

    expect(calls[1]!.value).toBe(9);
    calls[1]!.settle.resolve();
    await flush();
    expect(store.statusOf(KEY).kind).toBe("saved");
  });

  it("the same value twice sends once", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    calls[0]!.settle.resolve();
    await flush();
    store.commit(KEY, 8, null);
    expect(calls).toHaveLength(1);
  });
});

describe("GradeSaveStore — what is not saved", () => {
  it("counts failed, rejected and invalid cells, per scope, and keeps their text", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);
    const other = { ...KEY, enrollmentId: "en2" };
    const secondPeriod = { ...KEY, periodId: "p2" };

    store.commit(KEY, 8, null);
    store.commit(other, 9, null);
    store.markInvalid(secondPeriod, "11");
    calls[0]!.settle.reject(new Error("network"));
    calls[1]!.settle.reject(new GradeRequestError(400, "Solo se permiten calificaciones enteras"));
    await flush();

    expect(store.unsavedCount()).toBe(3);
    expect(store.unsavedCount({ assignmentId: "a1", periodId: "p1" })).toBe(2);
    expect(store.unsavedCount({ assignmentId: "a2" })).toBe(0);
    expect(store.retryableCount()).toBe(1);
    expect(store.draftOf(KEY)).toBe("8");
    expect(store.draftOf(secondPeriod)).toBe("11");
    expect(store.statusOf(other)).toEqual({ kind: "rejected", message: "Solo se permiten calificaciones enteras" });
  });

  it("retryAll resends every failed cell's value", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    calls[0]!.settle.reject(new Error("network"));
    await flush();

    store.retryAll();
    expect(calls[1]!.value).toBe(8);
    calls[1]!.settle.resolve();
    await flush();
    expect(store.unsavedCount()).toBe(0);
  });

  it("a later valid commit clears an invalid mark", () => {
    const { client } = manualClient();
    const store = new GradeSaveStore(client);
    store.markInvalid(KEY, "11");
    store.commit(KEY, 9, null);
    expect(store.unsavedCount()).toBe(0);
  });

  it("pending counts requests in flight", () => {
    const { client } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    expect(store.pendingCount()).toBe(1);
  });
});

describe("GradeSaveStore — loading a period", () => {
  it("settled waits for the period's saves in flight", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    let done = false;
    void store.settled({ assignmentId: "a1", periodId: "p1" }).then(() => { done = true; });
    await flush();
    expect(done).toBe(false);

    calls[0]!.settle.resolve();
    await flush();
    expect(done).toBe(true);
  });

  it("settled does not wait for other periods", async () => {
    const { client } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    await expect(store.settled({ assignmentId: "a1", periodId: "p2" })).resolves.toBeUndefined();
  });

  it("adopt takes the server's values for clean cells and keeps unsaved drafts", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    calls[0]!.settle.resolve();
    await flush();

    store.adopt(sheet(9));
    expect(store.confirmedOf(KEY, 9)).toBe(9);
    expect(store.statusOf(KEY).kind).toBe("idle");

    store.markInvalid(KEY, "11");
    store.adopt(sheet(9));
    expect(store.draftOf(KEY)).toBe("11");
  });
});

describe("GradeSaveStore — refusals by code", () => {
  const refused = (code: string, text: string): GradeRequestError => new GradeRequestError(400, text, code);

  it("PERIOD_PREVIOUS_GROUP locks the row with the reason and is not unsaved", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);

    store.commit(KEY, 8, null);
    calls[0]!.settle.reject(refused("PERIOD_PREVIOUS_GROUP", "Ese periodo corresponde al grupo anterior del alumno"));
    await flush();

    expect(store.statusOf(KEY)).toEqual({ kind: "locked", reason: "Ese periodo corresponde al grupo anterior del alumno" });
    expect(store.unsavedCount()).toBe(0);
    expect(store.draftOf(KEY)).toBeUndefined();
  });

  it("a LEVEL_* refusal is a notice for the whole sheet, not a row error", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);

    store.commit(KEY, 8, null);
    calls[0]!.settle.reject(refused("LEVEL_SCALE_NOT_CONFIGURED", "El nivel no tiene escala"));
    await flush();

    expect(store.sheetNotice({ assignmentId: "a1", periodId: "p1" })).toBe("El nivel no tiene escala");
    expect(store.sheetNotice({ assignmentId: "a1", periodId: "p2" })).toBeUndefined();
    expect(store.statusOf(KEY).kind).not.toBe("rejected");
    expect(store.unsavedCount()).toBe(0);
  });

  it("a validation code stays on the row, in the server's words", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);

    store.commit(KEY, 8, null);
    calls[0]!.settle.reject(refused("GRADE_DECIMALS", "Solo se permiten calificaciones enteras"));
    await flush();

    expect(store.statusOf(KEY)).toEqual({ kind: "rejected", message: "Solo se permiten calificaciones enteras" });
  });

  it("a reloaded read-only row drops the draft kept for it", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    calls[0]!.settle.reject(new Error("offline"));
    await flush();
    expect(store.unsavedCount()).toBe(1);

    const loaded = sheet(null);
    store.adopt({ ...loaded, rows: loaded.rows.map((r) => ({ ...r, rowEditable: false, rowReadOnlyReason: "Ese periodo corresponde al grupo anterior del alumno" })) });

    expect(store.unsavedCount()).toBe(0);
    expect(store.draftOf(KEY)).toBeUndefined();
  });
});
