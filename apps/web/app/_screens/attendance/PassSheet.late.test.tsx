// T12 — Retardo is a first-class, one-tap choice on every row (the director's
// and the teacher's lists share this sheet), and a row a card turned present
// can still be switched to Retardo before the list is saved.
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
  { enrollmentId: "en4", studentId: "st4", name: "Linus Torvalds", status: "present" as const, note: "" },
];
const unsaved: AttendanceSheet = { mode: "daily", date: TODAY, editable: true, windowDays: 7, session: null, rows: ROWS };
const SESSION = { takenAt: "2026-09-10T14:00:00Z", takenByName: "Mtra. Rojas" };

const load = vi.fn<(q: { date: string }) => Promise<AttendanceSheet>>();
const save = vi.fn<(input: SaveSheetInput) => Promise<void>>();
const scan = vi.fn<(input: ScanCardInput) => Promise<CardScanResult>>();

class FakeAudioContext {
  currentTime = 0;
  destination = {};
  state = "running";
  createOscillator() {
    return { type: "", frequency: { value: 0 }, connect: vi.fn(), start: vi.fn(), stop: vi.fn() };
  }
  createGain() {
    return { gain: { setValueAtTime: vi.fn(), exponentialRampToValueAtTime: vi.fn() }, connect: vi.fn() };
  }
  resume = () => Promise.resolve();
  close = () => Promise.resolve();
}

const ok = (enrollmentId: string, name: string): CardScanResult => ({ kind: "ok", student: { enrollmentId, studentId: `st-${enrollmentId}`, name } });

function renderSheet() {
  return render(<PassSheet title="1° A" groupId="g1" today={TODAY} restrictDates load={load} save={save} scan={scan} />);
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
  load.mockImplementation((q) => Promise.resolve({ ...structuredClone(unsaved), date: q.date }));
  save.mockResolvedValue(undefined);
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  vi.restoreAllMocks();
  scanner.decode = undefined;
  window.sessionStorage.clear();
});

describe("PassSheet — Retardo in one tap (T12)", () => {
  it("every row offers Retardo without opening Más; a tap marks it, another returns to present", async () => {
    const user = userEvent.setup();
    renderSheet();

    const late = await screen.findByRole("button", { name: "Retardo de Alan Turing" });
    expect(late).toHaveAttribute("aria-pressed", "false");
    expect(screen.queryByRole("textbox", { name: "Nota de Alan Turing" })).toBeNull();

    await user.click(late);
    expect(row("Alan Turing: Retardo")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retardo de Alan Turing" })).toHaveAttribute("aria-pressed", "true");

    await user.click(screen.getByRole("button", { name: "Retardo de Alan Turing" }));
    expect(row("Alan Turing: Presente")).toBeInTheDocument();
  });

  it("a row a card turned present switches to Retardo in one tap, and the save carries it", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en2", "Alan Turing"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:alan");
    await user.click(screen.getByRole("button", { name: "Terminar" }));
    expect(row("Alan Turing: Presente")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Retardo de Alan Turing" }));
    expect(row("Alan Turing: Retardo")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0]?.[0].marks).toContainEqual({ enrollmentId: "en2", status: "late" });
  });

  it("a saved list can be corrected from present to Retardo after the scan", async () => {
    const user = userEvent.setup();
    load.mockResolvedValue({ ...structuredClone(unsaved), session: SESSION });
    renderSheet();

    await user.click(await screen.findByRole("button", { name: "Retardo de Ada Lovelace" }));
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0]?.[0].marks).toContainEqual({ enrollmentId: "en1", status: "late" });
  });
});
