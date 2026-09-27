import { describe, expect, it, vi } from "vitest";

import { GradeRequestError, type GradeSheet, type GradeSheetClient, type SavedMonthlyGrade } from "../../grades/monthly";
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
    save: (i) => pending<SavedMonthlyGrade>("save", i.value, { ...i, updatedAt: "x" }),
    remove: () => pending<void>("remove", null, undefined),
  };
  return { client, calls };
}

const KEY = { assignmentId: "a1", month: "2026-09", enrollmentId: "en1" };
const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

function sheet(value: number | null): GradeSheet {
  return {
    assignmentId: "a1",
    month: "2026-09",
    months: ["2026-09"],
    scale: { min: 5, max: 10, decimals: 1, passing: 6 },
    editable: true,
    rows: [{ enrollmentId: "en1", studentId: "s1", name: "Ada", value, updatedAt: null }],
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
    const october = { ...KEY, month: "2026-10" };

    store.commit(KEY, 8, null);
    store.commit(other, 9, null);
    store.markInvalid(october, "11");
    calls[0]!.settle.reject(new Error("network"));
    calls[1]!.settle.reject(new GradeRequestError(400, "Solo se permite 1 decimal"));
    await flush();

    expect(store.unsavedCount()).toBe(3);
    expect(store.unsavedCount({ assignmentId: "a1", month: "2026-09" })).toBe(2);
    expect(store.unsavedCount({ assignmentId: "a2" })).toBe(0);
    expect(store.retryableCount()).toBe(1);
    expect(store.draftOf(KEY)).toBe("8");
    expect(store.draftOf(october)).toBe("11");
    expect(store.statusOf(other)).toEqual({ kind: "rejected", message: "Solo se permite 1 decimal" });
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

describe("GradeSaveStore — loading a month", () => {
  it("settled waits for the month's saves in flight", async () => {
    const { client, calls } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    let done = false;
    void store.settled({ assignmentId: "a1", month: "2026-09" }).then(() => { done = true; });
    await flush();
    expect(done).toBe(false);

    calls[0]!.settle.resolve();
    await flush();
    expect(done).toBe(true);
  });

  it("settled does not wait for other months", async () => {
    const { client } = manualClient();
    const store = new GradeSaveStore(client);
    store.commit(KEY, 8, null);
    await expect(store.settled({ assignmentId: "a1", month: "2026-10" })).resolves.toBeUndefined();
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
