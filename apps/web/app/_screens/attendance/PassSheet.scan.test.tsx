import { act, render, screen, waitFor, within } from "@testing-library/react";
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
];
/** A reload interrupted this: Alan scanned, the rest turned absent by Terminar, Grace left late. */
const DRAFT = { statuses: { en1: "absent", en2: "present", en3: "late" }, scanned: ["en2"], touched: [] };
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

describe("PassSheet — scan button", () => {
  it("is hidden without the scan capability (module_student_card off)", async () => {
    renderSheet({ scan: undefined });
    await screen.findByRole("button", { name: "Ada Lovelace: Presente" });
    expect(screen.queryByRole("button", { name: "Escanear QR" })).not.toBeInTheDocument();
  });

  it("is offered only on today's editable list", async () => {
    load.mockResolvedValue({ ...structuredClone(sheet), editable: false, readOnlyReason: "window" });
    renderSheet();
    await screen.findByText(/Solo lectura/);
    expect(screen.queryByRole("button", { name: "Escanear QR" })).not.toBeInTheDocument();
  });
});

describe("PassSheet — scanning", () => {
  it("opening the scanner leaves every present-by-default row 'Sin registrar' until scanned; a late one keeps its status", async () => {
    const user = userEvent.setup();
    renderSheet();
    await openScanner(user);

    expect(row("Ada Lovelace: Sin registrar")).toBeInTheDocument();
    expect(row("Grace Hopper: Retardo")).toBeInTheDocument();
    expect(screen.getByText("Escaneados 0 de 3")).toBeInTheDocument();
  });

  it("a good card marks the student present, flashes the name, beeps and vibrates — without leaving the camera", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en3", "Grace Hopper"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:grace");

    expect(scan).toHaveBeenCalledWith({ groupId: "g1", date: TODAY, code: "GLCARD:grace" });
    expect(row("Grace Hopper: Presente")).toBeInTheDocument();
    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Grace Hopper");
    expect(screen.getByRole("status")).toHaveTextContent("Presente: Grace Hopper");
    expect(screen.getByText("Escaneados 1 de 3")).toBeInTheDocument();
    expect(oscillator).toHaveBeenCalledTimes(1);
    expect(navigator.vibrate).toHaveBeenCalledWith(60);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("the same code read again within 2.5 s is ignored", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:ada");
    await read("GLCARD:ada");

    expect(scan).toHaveBeenCalledTimes(1);
  });

  it("a student already scanned is 'Ya registrado' with a soft beep", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:ada");
    await read("GLCARD:ada-reprint");

    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Ya registrado: Ada Lovelace");
    expect(screen.getByText("Escaneados 1 de 3")).toBeInTheDocument();
    expect(oscillator).toHaveBeenCalledTimes(2);
    expect(navigator.vibrate).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["foreign", "Este alumno no pertenece a este grupo"],
    ["invalid", "Código no válido"],
    ["suspended", "Credencial suspendida"],
  ] as const)("a %s card shows the server's warning in red, buzzes, and registers nothing", async (reason, message) => {
    const user = userEvent.setup();
    scan.mockResolvedValue({ kind: "rejected", reason, message });
    renderSheet();
    await openScanner(user);

    await read("GLCARD:x");

    expect(screen.getByTestId("scan-flash")).toHaveTextContent(message);
    expect(screen.getByRole("status")).toHaveTextContent(message);
    expect(screen.getByText("Escaneados 0 de 3")).toBeInTheDocument();
    expect(oscillator).toHaveBeenCalledTimes(2);
    expect(navigator.vibrate).toHaveBeenCalledWith([80, 60, 80]);
  });

  it("a student the server resolves but who is not on this list is refused", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en-other", "Otro Alumno"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:other");

    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Este alumno no pertenece a este grupo");
    expect(screen.getByText("Escaneados 0 de 3")).toBeInTheDocument();
  });

  it("a network failure is a warning, not a crash", async () => {
    const user = userEvent.setup();
    scan.mockRejectedValue(new TypeError("Failed to fetch"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:ada");

    expect(screen.getByTestId("scan-flash")).toHaveTextContent("No se pudo leer la credencial. Revisa tu conexión.");
  });
});

describe("PassSheet — finishing", () => {
  it("Terminar turns every 'Sin registrar' row absent, closes the camera and asks for a review; Guardar sends the exceptions", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");

    await user.click(screen.getByRole("button", { name: "Terminar" }));

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(row("Ada Lovelace: Presente")).toBeInTheDocument();
    expect(row("Alan Turing: Ausente")).toBeInTheDocument();
    expect(row("Grace Hopper: Retardo")).toBeInTheDocument();
    expect(screen.getByText("Revisa la lista: 1 alumno quedó como ausente.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Escanear QR" })).toHaveFocus();

    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await waitFor(() => expect(save).toHaveBeenCalled());
    expect(save.mock.calls[0]?.[0].marks).toEqual([{ enrollmentId: "en2", status: "absent" }, { enrollmentId: "en3", status: "late" }]);
  });

  it("re-opening continues: the already present stay present", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");
    await user.click(screen.getByRole("button", { name: "Terminar" }));

    await openScanner(user);

    expect(screen.getByText("Escaneados 1 de 3")).toBeInTheDocument();
    expect(row("Ada Lovelace: Presente")).toBeInTheDocument();
    expect(row("Alan Turing: Sin registrar")).toBeInTheDocument();
  });

  it("Esc finishes like Terminar", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");

    await user.keyboard("{Escape}");

    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByText("Revisa la lista: 1 alumno quedó como ausente.")).toBeInTheDocument();
  });
});

describe("PassSheet — scan draft", () => {
  it("keeps the scans in sessionStorage, not on the server, until Guardar", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en2", "Alan Turing"));
    renderSheet();
    await openScanner(user);

    await read("GLCARD:alan");

    expect(JSON.parse(window.sessionStorage.getItem(scanDraftKey(TARGET)) ?? "{}")).toEqual({
      statuses: { en1: "present", en2: "present", en3: "late" },
      scanned: ["en2"],
      touched: [],
    });
    expect(save).not.toHaveBeenCalled();
  });

  it("restores the scans after a reload with a banner, and clears them after a successful save", async () => {
    const user = userEvent.setup();
    window.sessionStorage.setItem(scanDraftKey(TARGET), JSON.stringify(DRAFT));
    renderSheet();

    expect(await screen.findByText("Recuperamos tus escaneos sin guardar")).toBeInTheDocument();
    expect(row("Alan Turing: Presente")).toBeInTheDocument();
    expect(row("Ada Lovelace: Ausente")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await waitFor(() => expect(window.sessionStorage.getItem(scanDraftKey(TARGET))).toBeNull());
    expect(screen.queryByText("Recuperamos tus escaneos sin guardar")).not.toBeInTheDocument();
  });

  it("keeps the draft when the save fails", async () => {
    const user = userEvent.setup();
    save.mockRejectedValue(new Error("boom"));
    window.sessionStorage.setItem(scanDraftKey(TARGET), JSON.stringify(DRAFT));
    renderSheet();
    await screen.findByText("Recuperamos tus escaneos sin guardar");

    await user.click(screen.getByRole("button", { name: "Guardar lista" }));

    await screen.findByRole("alert");
    expect(window.sessionStorage.getItem(scanDraftKey(TARGET))).not.toBeNull();
  });

  it("the review banner lists who is absent in the list itself", async () => {
    const user = userEvent.setup();
    scan.mockResolvedValue(ok("en1", "Ada Lovelace"));
    renderSheet();
    await openScanner(user);
    await read("GLCARD:ada");
    await user.click(screen.getByRole("button", { name: "Terminar" }));

    const list = screen.getByRole("list", { name: "Pase de lista" });
    expect(within(list).getAllByRole("button", { name: /: Ausente$/ })).toHaveLength(1);
  });
});
