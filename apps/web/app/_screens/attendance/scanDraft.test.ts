import { afterEach, describe, expect, it, vi } from "vitest";

import { clearScanDraft, readScanDraft, scanDraftKey, writeScanDraft } from "./scanDraft";

const TARGET = { groupId: "g1", subjectId: "s1", date: "2026-09-10" };

afterEach(() => {
  window.sessionStorage.clear();
  vi.restoreAllMocks();
});

describe("scanDraft", () => {
  it("is keyed by group, date and subject", () => {
    expect(scanDraftKey(TARGET)).toBe("attendance-scan:g1:2026-09-10:s1");
    expect(scanDraftKey({ groupId: "g1", date: "2026-09-10" })).toBe("attendance-scan:g1:2026-09-10:");
  });

  it("round-trips the scanned enrollments and clears them", () => {
    writeScanDraft(TARGET, ["en1", "en2"]);
    expect(readScanDraft(TARGET)).toEqual(["en1", "en2"]);
    expect(readScanDraft({ ...TARGET, date: "2026-09-11" })).toEqual([]);

    clearScanDraft(TARGET);
    expect(readScanDraft(TARGET)).toEqual([]);
  });

  it("an empty scan set removes the draft instead of storing []", () => {
    writeScanDraft(TARGET, ["en1"]);
    writeScanDraft(TARGET, []);
    expect(window.sessionStorage.getItem(scanDraftKey(TARGET))).toBeNull();
  });

  it("ignores a corrupt draft", () => {
    window.sessionStorage.setItem(scanDraftKey(TARGET), "{not json");
    expect(readScanDraft(TARGET)).toEqual([]);
    window.sessionStorage.setItem(scanDraftKey(TARGET), JSON.stringify({ scanned: [1, "en2"] }));
    expect(readScanDraft(TARGET)).toEqual(["en2"]);
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
    expect(() => writeScanDraft(TARGET, ["en1"])).not.toThrow();
    expect(readScanDraft(TARGET)).toEqual([]);
    expect(() => clearScanDraft(TARGET)).not.toThrow();
  });
});
