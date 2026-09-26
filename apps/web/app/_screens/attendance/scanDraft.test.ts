import { afterEach, describe, expect, it, vi } from "vitest";

import { clearScanDraft, readScanDraft, scanDraftKey, writeScanDraft, type ScanDraft } from "./scanDraft";

const TARGET = { groupId: "g1", subjectId: "s1", date: "2026-09-10" };
const DRAFT: ScanDraft = { statuses: { en1: "present", en2: "absent", en3: "late" }, scanned: ["en1"], touched: ["en3"] };

afterEach(() => {
  window.sessionStorage.clear();
  vi.restoreAllMocks();
});

describe("scanDraft", () => {
  it("is keyed by group, date and subject", () => {
    expect(scanDraftKey(TARGET)).toBe("attendance-scan:g1:2026-09-10:s1");
    expect(scanDraftKey({ groupId: "g1", date: "2026-09-10" })).toBe("attendance-scan:g1:2026-09-10:");
  });

  it("round-trips every row's status, the scanned and the hand-changed rows, and clears them", () => {
    writeScanDraft(TARGET, DRAFT);
    expect(readScanDraft(TARGET)).toEqual(DRAFT);
    expect(readScanDraft({ ...TARGET, date: "2026-09-11" })).toBeUndefined();

    clearScanDraft(TARGET);
    expect(readScanDraft(TARGET)).toBeUndefined();
  });

  it("a draft with nothing scanned is removed instead of stored", () => {
    writeScanDraft(TARGET, DRAFT);
    writeScanDraft(TARGET, { ...DRAFT, scanned: [] });
    expect(window.sessionStorage.getItem(scanDraftKey(TARGET))).toBeNull();
    writeScanDraft(TARGET, DRAFT);
    writeScanDraft(TARGET, undefined);
    expect(window.sessionStorage.getItem(scanDraftKey(TARGET))).toBeNull();
  });

  it("ignores a corrupt draft and drops malformed entries", () => {
    window.sessionStorage.setItem(scanDraftKey(TARGET), "{not json");
    expect(readScanDraft(TARGET)).toBeUndefined();
    window.sessionStorage.setItem(scanDraftKey(TARGET), JSON.stringify({ scanned: ["en2"] }));
    expect(readScanDraft(TARGET)).toBeUndefined();
    window.sessionStorage.setItem(
      scanDraftKey(TARGET),
      JSON.stringify({ statuses: { en1: "present", en2: "weird", en3: 4 }, scanned: [1, "en1"], touched: "nope" }),
    );
    expect(readScanDraft(TARGET)).toEqual({ statuses: { en1: "present" }, scanned: ["en1"], touched: [] });
  });

  it("never throws when storage is blocked", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    expect(() => writeScanDraft(TARGET, DRAFT)).not.toThrow();
    expect(readScanDraft(TARGET)).toBeUndefined();
    expect(() => clearScanDraft(TARGET)).not.toThrow();
  });
});
