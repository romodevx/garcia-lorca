// The scanner never overwrites a status someone chose, and never loses a stored
// draft before its list arrives (code review of feature/4-asistencia-qr,
// 2026-09-26). A card turns a row present only when it is "Sin registrar",
// absent or already present; a late or excused row is kept and reported as
// "Ya registrado: <Nombre> (Retardo|Justificada)".
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { CardScanResult, ScanCardInput } from "../../attendance/scan";
import type { AttendanceSheet, SaveSheetInput } from "../../attendance/sheet";
import { PassSheet } from "./PassSheet";
import { scanDraftKey } from "./scanDraft";

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
const TARGET = { groupId: "g1", date: TODAY };

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
const stored = (): unknown => JSON.parse(window.sessionStorage.getItem(scanDraftKey(TARGET)) ?? "null");
const DRAFT = { statuses: { en1: "present", en2: "absent", en3: "late", en4: "present" }, scanned: ["en1"], touched: [] };

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

describe("PassSheet — a scan keeps a late or excused row", () => {
  it("a row set to Retardo by hand stays Retardo and reports 'Ya registrado'", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en2", "Alan Turing"));
    renderSheet();
    await user.click(await screen.findByRole("button", { name: "Más opciones de Alan Turing" }));
    await user.click(screen.getByRole("button", { name: "Retardo" }));
    await openScanner(user);

    await read("GLCARD:alan");

    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Ya registrado: Alan Turing (Retardo)");
    expect(row("Alan Turing: Retardo")).toBeInTheDocument();
  });

  it("a loaded Justificada on a saved list stays Justificada", async () => {
    const user = userEvent.setup();
    const rows = ROWS.map((r) => (r.enrollmentId === "en2" ? { ...r, status: "excused" as const } : r));
    load.mockResolvedValue({ ...structuredClone(unsaved), session: SESSION, rows });
    scan.mockResolvedValue(ok("en2", "Alan Turing"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:alan");

    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Ya registrado: Alan Turing (Justificada)");
    expect(row("Alan Turing: Justificada")).toBeInTheDocument();
    expect(screen.queryByText("Cambios sin guardar")).not.toBeInTheDocument();
  });

  it("a latecomer saved as absent turns present", async () => {
    const user = userEvent.setup();
    const rows = ROWS.map((r) => (r.enrollmentId === "en2" ? { ...r, status: "absent" as const } : r));
    load.mockResolvedValue({ ...structuredClone(unsaved), session: SESSION, rows });
    scan.mockResolvedValue(ok("en2", "Alan Turing"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:alan");

    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Alan Turing");
    expect(screen.getByTestId("scan-flash")).not.toHaveTextContent("Ya registrado");
    expect(row("Alan Turing: Presente")).toBeInTheDocument();
  });
});

describe("PassSheet — the stored draft survives until its list arrives", () => {
  it("a list that fails to load leaves the stored draft in sessionStorage", async () => {
    window.sessionStorage.setItem(scanDraftKey(TARGET), JSON.stringify(DRAFT));
    load.mockRejectedValue(new Error("offline"));
    renderSheet();

    await waitFor(() => expect(load).toHaveBeenCalled());
    await act(async () => undefined);

    expect(stored()).toEqual(DRAFT);
  });

  it("a second reload before the list arrives still restores the draft", async () => {
    window.sessionStorage.setItem(scanDraftKey(TARGET), JSON.stringify(DRAFT));
    load.mockImplementation(() => new Promise<AttendanceSheet>(() => undefined));
    const first = renderSheet();
    await waitFor(() => expect(load).toHaveBeenCalled());
    expect(stored()).toEqual(DRAFT);

    // A reload never runs React cleanups: drop the tree without unmounting.
    first.container.remove();
    load.mockImplementation((q) => Promise.resolve({ ...structuredClone(unsaved), date: q.date }));
    renderSheet();

    expect(await screen.findByText("Recuperamos tus escaneos sin guardar")).toBeInTheDocument();
    expect(row("Alan Turing: Ausente")).toBeInTheDocument();
  });
});
