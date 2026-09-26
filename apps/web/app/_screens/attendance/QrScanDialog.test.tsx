import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { QrScanDialog, type QrScanDialogProps } from "./QrScanDialog";

const scanner = vi.hoisted(() => ({
  start: vi.fn<() => Promise<void>>(),
  destroy: vi.fn(),
  decode: undefined as undefined | ((result: { data: string }) => void),
  options: undefined as undefined | Record<string, unknown>,
  video: undefined as undefined | HTMLVideoElement,
}));

vi.mock("qr-scanner", () => ({
  default: class {
    constructor(video: HTMLVideoElement, onDecode: (r: { data: string }) => void, options: Record<string, unknown>) {
      scanner.video = video;
      scanner.decode = onDecode;
      scanner.options = options;
    }
    start = scanner.start;
    destroy = scanner.destroy;
  },
}));

const track = { stop: vi.fn() };

function setSecure(secure: boolean): void {
  Object.defineProperty(window, "isSecureContext", { value: secure, configurable: true });
}

const baseProps = (over: Partial<QrScanDialogProps> = {}): QrScanDialogProps => ({
  scannedCount: 12,
  total: 30,
  announcement: "",
  onCode: vi.fn(),
  onFinish: vi.fn(),
  ...over,
});

/** The trigger and the dialog, so focus return can be observed. */
function Harness({ onCode = vi.fn() }: { onCode?: (code: string) => void }): ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button type="button" onClick={() => setOpen(true)}>Escanear QR</button>
      {open && <QrScanDialog {...baseProps({ onCode, onFinish: () => setOpen(false) })} />}
    </>
  );
}

beforeEach(() => {
  setSecure(true);
  scanner.start.mockImplementation(async () => {
    if (scanner.video !== undefined) (scanner.video as unknown as { srcObject: unknown }).srcObject = { getTracks: () => [track] };
  });
});

afterEach(() => {
  vi.clearAllMocks();
  scanner.decode = undefined;
  scanner.video = undefined;
});

describe("QrScanDialog — dialog contract", () => {
  it("is a modal dialog labelled by its title, with the counter and the Terminar button focused", async () => {
    render(<QrScanDialog {...baseProps()} />);

    const dialog = screen.getByRole("dialog", { name: "Escanear credenciales" });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(screen.getByText("Escaneados 12 de 30")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByRole("button", { name: "Terminar" })).toHaveFocus());
  });

  it("Terminar and Esc finish; closing returns focus to the trigger", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const trigger = screen.getByRole("button", { name: "Escanear QR" });

    await user.click(trigger);
    await screen.findByRole("dialog");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    await user.click(await screen.findByRole("button", { name: "Terminar" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(trigger).toHaveFocus();
  });

  it("keeps Tab inside the dialog", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await user.click(screen.getByRole("button", { name: "Escanear QR" }));
    const finish = await screen.findByRole("button", { name: "Terminar" });
    await waitFor(() => expect(finish).toHaveFocus());

    await user.tab();
    expect(screen.getByRole("dialog")).toContainElement(document.activeElement as HTMLElement);
  });

  it("announces results in a polite live region and shows the flash card", () => {
    render(<QrScanDialog {...baseProps({ announcement: "Presente: Ada Lovelace", flash: { id: 1, tone: "ok", text: "Ada Lovelace" } })} />);

    expect(screen.getByRole("status")).toHaveTextContent("Presente: Ada Lovelace");
    expect(screen.getByTestId("scan-flash")).toHaveTextContent("Ada Lovelace");
  });
});

describe("QrScanDialog — camera", () => {
  it("starts the rear camera and hands every decoded code to onCode", async () => {
    const onCode = vi.fn();
    render(<QrScanDialog {...baseProps({ onCode })} />);

    await waitFor(() => expect(scanner.start).toHaveBeenCalled());
    expect(scanner.options).toMatchObject({ preferredCamera: "environment" });
    act(() => scanner.decode?.({ data: "GLCARD:abc" }));
    expect(onCode).toHaveBeenCalledWith("GLCARD:abc");
  });

  it("stops the camera tracks and destroys the scanner on close", async () => {
    const { unmount } = render(<QrScanDialog {...baseProps()} />);
    await waitFor(() => expect(scanner.start).toHaveBeenCalled());
    await act(async () => undefined);

    unmount();
    expect(scanner.destroy).toHaveBeenCalled();
    expect(track.stop).toHaveBeenCalled();
  });

  it("explains a denied permission and lets the teacher retry", async () => {
    const user = userEvent.setup();
    scanner.start.mockRejectedValueOnce(Object.assign(new Error("Permission denied"), { name: "NotAllowedError" }));
    render(<QrScanDialog {...baseProps()} />);

    expect(await screen.findByText("Permite el acceso a la cámara en la configuración del navegador y vuelve a intentar.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    await waitFor(() => expect(scanner.start).toHaveBeenCalledTimes(2));
  });

  it("says when there is no camera", async () => {
    scanner.start.mockRejectedValueOnce("Camera not found.");
    render(<QrScanDialog {...baseProps()} />);
    expect(await screen.findByText("No encontramos una cámara en este dispositivo.")).toBeInTheDocument();
  });

  it("refuses to start on an insecure connection", async () => {
    setSecure(false);
    render(<QrScanDialog {...baseProps()} />);
    expect(await screen.findByText("La cámara necesita una conexión segura (https).")).toBeInTheDocument();
    expect(scanner.start).not.toHaveBeenCalled();
  });
});
