import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AttendanceRequestError, type AttendanceSheet, type SaveSheetInput } from "../../attendance/sheet";
import { PassSheet } from "./PassSheet";
import { lastWeekdayOnOrBefore, NO_CLASSES_TEXT, readOnlyText } from "./sheetModel";

// ISSUE-T14-101: attendance opened on a Saturday and saved it.
const SATURDAY = "2026-10-17";
const FRIDAY = "2026-10-16";

const sheetOf = (over: Partial<AttendanceSheet> = {}): AttendanceSheet => ({
  mode: "daily",
  date: FRIDAY,
  editable: true,
  windowDays: 7,
  session: null,
  rows: [{ enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present", note: "" }],
  ...over,
});

const load = vi.fn<(q: { groupId: string; date: string; subjectId?: string }) => Promise<AttendanceSheet>>();
const save = vi.fn<(input: SaveSheetInput) => Promise<void>>();

const renderSheet = (today: string, restrictDates = true) =>
  render(<PassSheet title="1° A" groupId="g1" today={today} restrictDates={restrictDates} load={load} save={save} />);

beforeEach(() => {
  load.mockImplementation(async (q) => sheetOf({ date: q.date }));
  save.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("lastWeekdayOnOrBefore", () => {
  it.each([
    ["2026-10-17", "2026-10-16"], // Saturday → Friday
    ["2026-10-18", "2026-10-16"], // Sunday → Friday
    ["2026-10-19", "2026-10-19"], // Monday stays
    ["2026-10-16", "2026-10-16"], // Friday stays
    ["2026-11-01", "2026-10-30"], // Sunday across a month
  ])("%s → %s", (day, want) => {
    expect(lastWeekdayOnOrBefore(day)).toBe(want);
  });
});

describe("readOnlyText — no classes", () => {
  it("says the day has no classes, for the teacher and the director", () => {
    expect(readOnlyText("no_classes", 7, true)).toBe(NO_CLASSES_TEXT);
    expect(readOnlyText("no_classes", 7, false)).toBe(NO_CLASSES_TEXT);
    expect(NO_CLASSES_TEXT).toBe("Ese día no hay clases.");
  });
});

describe("PassSheet — days without classes", () => {
  it("opens on Friday when today is Saturday (teacher and director)", async () => {
    const { unmount } = renderSheet(SATURDAY);
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.getByLabelText("Fecha")).toHaveValue(FRIDAY);
    expect(load).toHaveBeenCalledWith({ groupId: "g1", date: FRIDAY });
    unmount();

    renderSheet(SATURDAY, false);
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.getByLabelText("Fecha")).toHaveValue(FRIDAY);
  });

  it("marks a picked day without classes on the date field and locks the list", async () => {
    load.mockImplementation(async (q) =>
      q.date === SATURDAY ? sheetOf({ date: q.date, editable: false, readOnlyReason: "no_classes" }) : sheetOf({ date: q.date }),
    );
    renderSheet(SATURDAY);
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });

    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: SATURDAY } });

    await waitFor(() => expect(screen.getByLabelText("Fecha")).toHaveAttribute("aria-invalid", "true"));
    expect(screen.getAllByText(/Ese día no hay clases\./).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Guardar lista" })).not.toBeInTheDocument();
  });

  it("shows the server's refusal when a save lands on a day without classes", async () => {
    save.mockRejectedValue(new AttendanceRequestError(400, "Ese día no hay clases."));
    renderSheet(FRIDAY);
    fireEvent.click(await screen.findByRole("button", { name: "Guardar lista" }));
    expect(await screen.findByText("Ese día no hay clases.")).toBeInTheDocument();
  });
});
