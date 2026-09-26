import { useEffect, useRef, useState } from "react";

/**
 * The rear camera, decoding QR codes continuously into `onCode`.
 *
 * `qr-scanner` is imported ON DEMAND: the camera is opened by a few teachers a
 * few times a day, so its code (and its decoding worker) never reaches the
 * main bundle. It decodes off the main thread and uses the native
 * BarcodeDetector where the browser has one (Android Chrome), falling back to
 * its worker elsewhere (iOS Safari has no BarcodeDetector).
 *
 * The camera is released on every exit — close, unmount, a retry — by
 * destroying the scanner AND stopping the stream's tracks: a light left on
 * after the dialog closed is exactly what a teacher would notice.
 */

export type CameraStatus = "starting" | "live" | "error";

export interface QrCameraState {
  status: CameraStatus;
  error?: string;
  retry: () => void;
}

const SCANS_PER_SECOND = 8;
const INSECURE = "La cámara necesita una conexión segura (https).";
const DENIED = "Permite el acceso a la cámara en la configuración del navegador y vuelve a intentar.";
const NO_CAMERA = "No encontramos una cámara en este dispositivo.";
const BUSY = "La cámara está ocupada por otra aplicación. Ciérrala y vuelve a intentar.";
const FAILED = "No se pudo abrir la cámara. Vuelve a intentar.";
/** What qr-scanner throws (as a string) when no stream could be opened. */
const LIBRARY_NO_CAMERA = "Camera not found.";

/** A camera failure, in words the teacher can act on. */
export function cameraErrorMessage(cause: unknown): string {
  if (cause === LIBRARY_NO_CAMERA) return NO_CAMERA;
  const name = typeof cause === "object" && cause !== null ? (cause as { name?: unknown }).name : undefined;
  if (name === "NotAllowedError" || name === "SecurityError") return DENIED;
  if (name === "NotFoundError" || name === "OverconstrainedError") return NO_CAMERA;
  if (name === "NotReadableError") return BUSY;
  return FAILED;
}

/** The camera permission, or undefined where the Permissions API cannot tell (older Safari, Firefox). */
async function cameraPermission(): Promise<PermissionState | undefined> {
  try {
    const status = await navigator.permissions?.query({ name: "camera" as PermissionName });
    return status?.state;
  } catch {
    return undefined;
  }
}

async function cameraCount(): Promise<number> {
  try {
    const devices = (await navigator.mediaDevices?.enumerateDevices()) ?? [];
    return devices.filter((d) => d.kind === "videoinput").length;
  } catch {
    return 0;
  }
}

/**
 * qr-scanner swallows getUserMedia's own error and throws "Camera not found."
 * for every failure, a denied permission included, so the cause is asked of
 * the browser afterwards: a camera that exists but was not allowed is a
 * permission problem, not a missing camera.
 */
async function explainLibraryFailure(): Promise<string> {
  const permission = await cameraPermission();
  if (permission === "denied") return DENIED;
  if ((await cameraCount()) === 0) return NO_CAMERA;
  return permission === "granted" ? FAILED : DENIED;
}

/** Why the camera did not open, in words the teacher can act on. */
async function describeFailure(cause: unknown): Promise<string> {
  return cause === LIBRARY_NO_CAMERA ? explainLibraryFailure() : cameraErrorMessage(cause);
}

function stopTracks(video: HTMLVideoElement | null): void {
  const stream = (video as unknown as { srcObject?: MediaStream | null } | null)?.srcObject;
  stream?.getTracks().forEach((t) => t.stop());
  if (video !== null) (video as unknown as { srcObject: MediaStream | null }).srcObject = null;
}

/** `video` is the mounted element (a callback ref's state), so the camera starts once it exists. */
export function useQrCamera(video: HTMLVideoElement | null, onCode: (code: string) => void): QrCameraState {
  const [status, setStatus] = useState<CameraStatus>("starting");
  const [error, setError] = useState<string | undefined>(undefined);
  const [attempt, setAttempt] = useState(0);
  const latestOnCode = useRef(onCode);
  latestOnCode.current = onCode;

  useEffect(() => {
    if (video === null) return;
    if (!window.isSecureContext) {
      setStatus("error");
      setError(INSECURE);
      return;
    }
    let cancelled = false;
    let scanner: { destroy: () => void } | undefined;
    setStatus("starting");
    setError(undefined);

    void (async () => {
      try {
        const { default: QrScanner } = await import("qr-scanner");
        if (cancelled) return;
        const instance = new QrScanner(video, (result) => latestOnCode.current(result.data), {
          preferredCamera: "environment",
          maxScansPerSecond: SCANS_PER_SECOND,
          returnDetailedScanResult: true,
        });
        scanner = instance;
        await instance.start();
        if (!cancelled) setStatus("live");
      } catch (cause) {
        if (cancelled) return;
        const message = await describeFailure(cause);
        if (cancelled) return;
        setStatus("error");
        setError(message);
      }
    })();

    return () => {
      cancelled = true;
      scanner?.destroy();
      stopTracks(video);
    };
  }, [video, attempt]);

  return { status, error, retry: () => setAttempt((n) => n + 1) };
}
