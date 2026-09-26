// Regression: ISSUE-002 — "Terminar" with nothing scanned marked the whole class absent
// Found by /qa on 2026-09-26
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-26-t4.md
//
// With the camera denied or broken, "Terminar" is the only way out of the
// dialog; it turned every row absent and one "Guardar lista" away from a
// notice to every family.
import { act, render, screen, waitFor, within } from "@testing-library/react";
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

const row = (label: string): HTMLElement => screen.getByRole("button", { name: label });

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
  scanner.decode = undefined;
  window.sessionStorage.clear();
});

describe("PassSheet — finishing with nothing scanned", () => {
  it("closes the camera and leaves the list exactly as it was", async () => {
    const user = userEvent.setup();
    renderSheet();
    await openScanner(user);

    await user.click(screen.getByRole("button", { name: "Terminar" }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(row("Ada Lovelace: Presente")).toBeInTheDocument();
    expect(row("Alan Turing: Presente")).toBeInTheDocument();
    expect(row("Grace Hopper: Retardo")).toBeInTheDocument();
    expect(screen.queryByText(/Revisa la lista/)).not.toBeInTheDocument();
    expect(screen.queryByText("Cambios sin guardar")).not.toBeInTheDocument();
  });

  it("after one scan, Terminar still turns the rest absent", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");

    await user.click(screen.getByRole("button", { name: "Terminar" }));

    expect(row("Ada Lovelace: Presente")).toBeInTheDocument();
    expect(row("Alan Turing: Ausente")).toBeInTheDocument();
    expect(within(screen.getByRole("status")).getByText(/1 alumno quedó como ausente/)).toBeInTheDocument();
  });
});
