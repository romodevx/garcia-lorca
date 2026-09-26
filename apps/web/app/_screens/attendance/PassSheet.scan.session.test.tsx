// The scan session's rules (code review of feature/4-asistencia-qr, 2026-09-26):
// "Sin registrar" is only for present-by-default rows of a list nobody saved
// yet and nobody changed by hand; Terminar never overwrites a late, excused or
// absent row; a lookup from a closed session is dropped; the draft follows the
// list it belongs to and holds the whole list, not just the scans.
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
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
const YESTERDAY = "2026-09-09";
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
const stored = (date = TODAY): unknown => JSON.parse(window.sessionStorage.getItem(scanDraftKey({ ...TARGET, date })) ?? "null");

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

describe("PassSheet — scanning a list already saved", () => {
  it("after saving, a second session only turns the latecomer present: Terminar changes nobody else", async () => {
    const user = userEvent.setup();
    scan.mockImplementation(({ code }) => Promise.resolve(code === "GLCARD:alan" ? ok("en2", "Alan Turing") : ok("en1", "Ada Lovelace")));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");
    await user.click(screen.getByRole("button", { name: "Terminar" }));
    const savedRows = ROWS.map((r) => (r.enrollmentId === "en2" || r.enrollmentId === "en4" ? { ...r, status: "absent" as const } : r));
    load.mockResolvedValue({ ...structuredClone(unsaved), session: SESSION, rows: savedRows });
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));
    await screen.findByText(/Lista guardada a las/);

    await openScanner(user);
    expect(screen.queryByRole("button", { name: /Sin registrar$/ })).not.toBeInTheDocument();
    await read("GLCARD:alan");
    await user.click(screen.getByRole("button", { name: "Terminar" }));

    expect(row("Alan Turing: Presente")).toBeInTheDocument();
    expect(row("Ada Lovelace: Presente")).toBeInTheDocument();
    expect(row("Grace Hopper: Retardo")).toBeInTheDocument();
    expect(row("Linus Torvalds: Ausente")).toBeInTheDocument();
    expect(screen.queryByText(/Revisa la lista/)).not.toBeInTheDocument();
  });

  it("on a list loaded as saved, Terminar marks nobody absent", async () => {
    const user = userEvent.setup();
    load.mockResolvedValue({ ...structuredClone(unsaved), session: SESSION });
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");

    await user.click(screen.getByRole("button", { name: "Terminar" }));

    expect(screen.queryByRole("button", { name: /: Ausente$/ })).not.toBeInTheDocument();
    expect(screen.queryByText("Cambios sin guardar")).not.toBeInTheDocument();
  });
});

describe("PassSheet — Terminar never overwrites a status someone chose", () => {
  it("keeps a loaded late row and a hand-set excused row; only 'Sin registrar' rows turn absent", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await user.click(await screen.findByRole("button", { name: "Más opciones de Alan Turing" }));
    await user.click(screen.getByRole("button", { name: "Justificada" }));
    await openScanner(user);

    expect(row("Grace Hopper: Retardo")).toBeInTheDocument();
    expect(row("Alan Turing: Justificada")).toBeInTheDocument();
    expect(row("Linus Torvalds: Sin registrar")).toBeInTheDocument();
    await read("GLCARD:ada");
    await user.click(screen.getByRole("button", { name: "Terminar" }));

    expect(row("Alan Turing: Justificada")).toBeInTheDocument();
    expect(row("Grace Hopper: Retardo")).toBeInTheDocument();
    expect(row("Linus Torvalds: Ausente")).toBeInTheDocument();
    expect(screen.getByText("Revisa la lista: 1 alumno quedó como ausente.")).toBeInTheDocument();
  });

  it("a row the teacher put back to present after Terminar stays present on the next Terminar", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");
    await user.click(screen.getByRole("button", { name: "Terminar" }));
    await user.click(row("Alan Turing: Ausente"));

    await openScanner(user);
    await user.click(screen.getByRole("button", { name: "Terminar" }));

    expect(row("Alan Turing: Presente")).toBeInTheDocument();
    expect(row("Linus Torvalds: Ausente")).toBeInTheDocument();
  });
});

describe("PassSheet — a lookup that outlives its session", () => {
  it("is dropped when it answers after Terminar", async () => {
    const user = userEvent.setup();
    let answer: (result: CardScanResult) => void = () => undefined;
    scan.mockImplementation(() => new Promise((resolve) => { answer = resolve; }));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:alan");
    await user.click(screen.getByRole("button", { name: "Terminar" }));

    await act(async () => answer(ok("en2", "Alan Turing")));

    expect(stored()).toBeNull();
    await openScanner(user);
    expect(screen.getByText("Escaneados 0 de 4")).toBeInTheDocument();
    expect(row("Alan Turing: Sin registrar")).toBeInTheDocument();
  });
});

describe("PassSheet — the draft follows its list", () => {
  it("discarding the changes by switching the day clears that day's draft", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");
    await user.click(screen.getByRole("button", { name: "Terminar" }));
    expect(stored()).not.toBeNull();

    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: YESTERDAY } });

    await waitFor(() => expect(stored()).toBeNull());
  });

  it("leaving the list clears its draft", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    const { unmount } = renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");
    expect(stored()).not.toBeNull();

    unmount();

    expect(stored()).toBeNull();
  });

  it("a save that lands after the day changed never clears the new day's draft", async () => {
    const user = userEvent.setup();
    vi.spyOn(window, "confirm").mockReturnValue(true);
    const yesterdayDraft = { statuses: { en1: "present", en2: "absent", en3: "late", en4: "present" }, scanned: ["en1"], touched: [] };
    window.sessionStorage.setItem(scanDraftKey({ ...TARGET, date: YESTERDAY }), JSON.stringify(yesterdayDraft));
    let finishSave: () => void = () => undefined;
    save.mockImplementation(() => new Promise<void>((resolve) => { finishSave = resolve; }));
    renderSheet();
    await user.click(await screen.findByRole("button", { name: "Ada Lovelace: Presente" }));
    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: YESTERDAY } });
    await screen.findByText("Recuperamos tus escaneos sin guardar");
    await act(async () => finishSave());

    expect(stored(YESTERDAY)).toEqual(yesterdayDraft);
    expect(screen.getByText("Recuperamos tus escaneos sin guardar")).toBeInTheDocument();
    expect(row("Alan Turing: Ausente")).toBeInTheDocument();
  });
});

describe("PassSheet — the draft is the whole list", () => {
  it("a reload after Terminar and a correction brings the list back exactly as it was", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    const { unmount } = renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");
    await user.click(screen.getByRole("button", { name: "Terminar" }));
    await user.click(row("Alan Turing: Ausente"));
    const draft = window.sessionStorage.getItem(scanDraftKey(TARGET));
    expect(JSON.parse(draft ?? "null")).toEqual({
      statuses: { en1: "present", en2: "present", en3: "late", en4: "absent" },
      scanned: ["en1"],
      touched: ["en2"],
    });

    unmount();
    window.sessionStorage.setItem(scanDraftKey(TARGET), draft ?? "");
    renderSheet();

    expect(await screen.findByText("Recuperamos tus escaneos sin guardar")).toBeInTheDocument();
    expect(row("Ada Lovelace: Presente")).toBeInTheDocument();
    expect(row("Alan Turing: Presente")).toBeInTheDocument();
    expect(row("Grace Hopper: Retardo")).toBeInTheDocument();
    expect(row("Linus Torvalds: Ausente")).toBeInTheDocument();
  });
});

describe("PassSheet — a card already read in this session", () => {
  it("shown again after leaving the camera is 'Ya registrado' without asking the server", async () => {
    const user = userEvent.setup();
    const now = vi.spyOn(Date, "now");
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);

    now.mockReturnValue(100_000);
    await read("GLCARD:ada");
    now.mockReturnValue(104_000);
    await read("GLCARD:ada");

    expect(scan).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Ya registrado: Ada Lovelace");
  });
});
