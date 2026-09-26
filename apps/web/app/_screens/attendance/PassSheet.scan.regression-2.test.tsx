// Regression: ISSUE-003 — a card held in front of the camera re-fired every 2.5 s
// Found by /qa on 2026-09-26
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-26-t4.md
//
// The debounce counted from the FIRST sighting, so a card held for 3 s beeped
// "Ya registrado" (or buzzed its refusal again, with a second server call).
// A card counts as new only after it has been out of view for the window.
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { CardScanResult, ScanCardInput } from "../../attendance/scan";
import type { AttendanceSheet, SaveSheetInput } from "../../attendance/sheet";
import { PassSheet } from "./PassSheet";

const scanner = vi.hoisted(() => ({
  decode: undefined as undefined | ((result: { data: string }) => void),
  destroy: vi.fn(),
}));

vi.mock("qr-scanner", () => ({
  default: class {
    constructor(_video: HTMLVideoElement, onDecode: (r: { data: string }) => void) {
      scanner.decode = onDecode;
    }
    start = () => Promise.resolve();
    destroy = scanner.destroy;
  },
}));

const TODAY = "2026-09-10";

const ROWS = [
  { enrollmentId: "en1", studentId: "st1", name: "Ada Lovelace", status: "present" as const, note: "" },
  { enrollmentId: "en2", studentId: "st2", name: "Alan Turing", status: "present" as const, note: "" },
  { enrollmentId: "en3", studentId: "st3", name: "Grace Hopper", status: "late" as const, note: "" },
];
const sheet: AttendanceSheet = { mode: "daily", date: TODAY, editable: true, windowDays: 7, session: null, rows: ROWS };

const load = vi.fn<() => Promise<AttendanceSheet>>();
const save = vi.fn<(input: SaveSheetInput) => Promise<void>>();
const scan = vi.fn<(input: ScanCardInput) => Promise<CardScanResult>>();
const oscillator = vi.fn();

class FakeAudioContext {
  currentTime = 0;
  destination = {};
  state = "running";
  createOscillator() {
    oscillator();
    return { type: "", frequency: { value: 0 }, connect: vi.fn(), start: vi.fn(), stop: vi.fn() };
  }
  createGain() {
    return { gain: { setValueAtTime: vi.fn(), exponentialRampToValueAtTime: vi.fn() }, connect: vi.fn() };
  }
  resume = () => Promise.resolve();
  close = () => Promise.resolve();
}

const ok = (enrollmentId: string, name: string): CardScanResult => ({ kind: "ok", student: { enrollmentId, studentId: `st-${enrollmentId}`, name } });

function renderSheet(props: Partial<Parameters<typeof PassSheet>[0]> = {}) {
  return render(<PassSheet title="1° A" groupId="g1" today={TODAY} restrictDates load={load} save={save} scan={scan} {...props} />);
}

async function openScanner(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole("button", { name: "Escanear QR" }));
  await screen.findByRole("dialog", { name: "Escanear credenciales" });
  await waitFor(() => expect(scanner.decode).toBeDefined());
}

async function read(code: string): Promise<void> {
  await act(async () => {
    scanner.decode?.({ data: code });
  });
}

beforeEach(() => {
  Object.defineProperty(window, "isSecureContext", { value: true, configurable: true });
  Object.defineProperty(navigator, "vibrate", { value: vi.fn(() => true), configurable: true });
  vi.stubGlobal("AudioContext", FakeAudioContext);
  load.mockResolvedValue(structuredClone(sheet));
  save.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  vi.restoreAllMocks();
  scanner.decode = undefined;
  window.sessionStorage.clear();
});

describe("PassSheet — a card held in front of the camera", () => {
  it("is read once while it stays in view, however long", async () => {
    const user = userEvent.setup();
    const now = vi.spyOn(Date, "now");
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);

    for (const t of [0, 1000, 2000, 3000, 4000, 5000]) {
      now.mockReturnValue(100_000 + t);
      await read("GLCARD:ada");
    }

    expect(scan).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Ada Lovelace");
  });

  it("is read again once it was out of view for the window", async () => {
    const user = userEvent.setup();
    const now = vi.spyOn(Date, "now");
    scan.mockResolvedValue({ kind: "rejected", reason: "foreign", message: "Este alumno no pertenece a este grupo" });
    renderSheet();
    await openScanner(user);

    now.mockReturnValue(100_000);
    await read("GLCARD:other");
    now.mockReturnValue(103_000);
    await read("GLCARD:other");

    expect(scan).toHaveBeenCalledTimes(2);
  });
});
