// Regression: ISSUE-001 — a denied camera permission said "No encontramos una cámara"
// Found by /qa on 2026-09-26
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-26-t4.md
//
// qr-scanner swallows getUserMedia's NotAllowedError and throws its own
// string "Camera not found." for EVERY failure, so the error name never reaches
// us. The real cause has to be asked of the browser afterwards.
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { QrScanDialog } from "./QrScanDialog";

const start = vi.hoisted(() => vi.fn<() => Promise<void>>());

vi.mock("qr-scanner", () => ({
  default: class {
    start = start;
    destroy = vi.fn();
  },
}));

const DENIED = "Permite el acceso a la cámara en la configuración del navegador y vuelve a intentar.";
const NO_CAMERA = "No encontramos una cámara en este dispositivo.";

function stubBrowser(permission: PermissionState | "unsupported", cameras: number): void {
  const query = permission === "unsupported"
    ? vi.fn().mockRejectedValue(new TypeError("camera is not a valid permission name"))
    : vi.fn().mockResolvedValue({ state: permission });
  Object.defineProperty(navigator, "permissions", { value: { query }, configurable: true });
  const devices = Array.from({ length: cameras }, () => ({ kind: "videoinput" }));
  Object.defineProperty(navigator, "mediaDevices", { value: { enumerateDevices: vi.fn().mockResolvedValue(devices) }, configurable: true });
}

const renderDialog = () => render(<QrScanDialog scannedCount={0} total={3} announcement="" onCode={vi.fn()} onFinish={vi.fn()} />);

beforeEach(() => {
  Object.defineProperty(window, "isSecureContext", { value: true, configurable: true });
  start.mockRejectedValue("Camera not found.");
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("QrScanDialog — what the library's 'Camera not found.' really was", () => {
  it("a denied permission asks the teacher to allow the camera", async () => {
    stubBrowser("denied", 1);
    renderDialog();
    expect(await screen.findByText(DENIED)).toBeInTheDocument();
  });

  it("a dismissed prompt on a device with a camera is still a permission problem", async () => {
    stubBrowser("prompt", 1);
    renderDialog();
    expect(await screen.findByText(DENIED)).toBeInTheDocument();
  });

  it("without the Permissions API, a device with a camera is a permission problem", async () => {
    stubBrowser("unsupported", 2);
    renderDialog();
    expect(await screen.findByText(DENIED)).toBeInTheDocument();
  });

  it("a device with no camera at all says so", async () => {
    stubBrowser("prompt", 0);
    renderDialog();
    expect(await screen.findByText(NO_CAMERA)).toBeInTheDocument();
  });
});
