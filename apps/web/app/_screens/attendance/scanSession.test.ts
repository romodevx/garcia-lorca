import { describe, expect, it } from "vitest";

import type { AttendanceSheet, AttendanceSheetRow } from "../../attendance/sheet";
import { draftOf, emptySession, openSession, touched, unregistered, withScan } from "./scanSession";

const row = (enrollmentId: string, status: AttendanceSheetRow["status"]): AttendanceSheetRow => ({ enrollmentId, studentId: `st-${enrollmentId}`, name: enrollmentId, status, note: "" });
const ROWS = [row("en1", "present"), row("en2", "present"), row("en3", "late"), row("en4", "excused")];
const UNSAVED: AttendanceSheet = { mode: "daily", date: "2026-09-10", editable: true, windowDays: 7, session: null, rows: ROWS };
const SAVED: AttendanceSheet = { ...UNSAVED, session: { takenAt: "2026-09-10T14:00:00Z", takenByName: "Mtra. Rojas" } };

describe("scanSession", () => {
  it("on a list never saved, only the present-by-default rows can be 'Sin registrar'", () => {
    const { session, restore } = openSession("k", UNSAVED, undefined);
    expect([...unregistered(session)]).toEqual(["en1", "en2"]);
    expect(restore).toBeUndefined();
  });

  it("on a list already saved, nobody is 'Sin registrar'", () => {
    expect(unregistered(openSession("k", SAVED, undefined).session).size).toBe(0);
  });

  it("a scanned or hand-changed row stops being 'Sin registrar'", () => {
    const opened = openSession("k", UNSAVED, undefined).session;
    expect([...unregistered(touched(withScan(opened, "en1"), ["en2"]))]).toEqual([]);
  });

  it("restores a draft verbatim, only for rows still on the list, and only on an editable list", () => {
    const draft = { statuses: { en1: "present" as const, en2: "late" as const, gone: "absent" as const }, scanned: ["en1", "gone"], touched: ["en2"] };

    const { session, restore } = openSession("k", UNSAVED, draft);
    expect(restore).toEqual({ en1: "present", en2: "late" });
    expect([...session.scanned]).toEqual(["en1"]);
    expect([...session.touched]).toEqual(["en2"]);
    expect([...unregistered(session)]).toEqual([]);

    expect(openSession("k", { ...UNSAVED, editable: false }, draft).restore).toBeUndefined();
  });

  it("a draft with nothing scanned on the list is not restored", () => {
    expect(openSession("k", UNSAVED, { statuses: { gone: "present" }, scanned: ["gone"], touched: [] }).restore).toBeUndefined();
  });

  it("the draft is every row's status plus the scanned and touched rows; none without a scan", () => {
    const session = touched(withScan(openSession("k", UNSAVED, undefined).session, "en1"), ["en3"]);
    expect(draftOf(session, ROWS)).toEqual({ statuses: { en1: "present", en2: "present", en3: "late", en4: "excused" }, scanned: ["en1"], touched: ["en3"] });
    expect(draftOf(emptySession("k"), ROWS)).toBeUndefined();
  });
});
