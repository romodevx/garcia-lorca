import { Directory, File, Paths } from "expo-file-system";
import * as Sharing from "expo-sharing";
import { refreshOnce } from "../api/client";
import { apiUrl } from "../api/config";
import { getAccessToken } from "../api/tokenStore";

/**
 * Downloads an authenticated PDF (a boleta) into the app's cache and hands it
 * to the phone's native share / preview sheet.
 *
 * The download is native (expo-file-system `File.downloadFileAsync`) so the
 * bytes never cross the JS bridge. Native code names the file from the
 * server's Content-Disposition; when it cannot (it falls back to the URL's last
 * segment, "pdf") the file is renamed to the caller's fallback name. Native
 * downloads bypass {@link apiFetch}, so an expired access token is handled
 * here the same way: refresh once, retry once.
 */

export type PdfErrorKind = "network" | "not_found" | "session" | "unavailable";

/** A download or share failure, classified so the screen can say something useful. */
export class PdfError extends Error {
  constructor(public readonly kind: PdfErrorKind, message?: string) {
    super(message ?? kind);
    this.name = "PdfError";
  }
}

export interface PdfDownloadRequest {
  /** Authenticated API path of the PDF. */
  path: string;
  /** Used when the server's filename did not reach the native layer. */
  fallbackName: string;
  /** Fraction 0–1, or null when the server did not send a size. */
  onProgress?: (fraction: number | null) => void;
}

const CACHE_FOLDER = "boletas";
const PDF_EXTENSION = ".pdf";
const PDF_MIME = "application/pdf";
const PDF_UTI = "com.adobe.pdf";
const HTTP_UNAUTHORIZED = /HTTP 401\b/;
const HTTP_NOT_FOUND = /HTTP 404\b/;

function bearer(): Record<string, string> {
  const token = getAccessToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function messageOf(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

function classify(failure: unknown): PdfError {
  const message = messageOf(failure);
  if (HTTP_UNAUTHORIZED.test(message)) return new PdfError("session", message);
  if (HTTP_NOT_FOUND.test(message)) return new PdfError("not_found", message);
  return new PdfError("network", message);
}

async function downloadOnce(request: PdfDownloadRequest, destination: Directory): Promise<File> {
  return File.downloadFileAsync(apiUrl(request.path), destination, {
    headers: bearer(),
    idempotent: true,
    onProgress: ({ bytesWritten, totalBytes }) =>
      request.onProgress?.(totalBytes > 0 ? bytesWritten / totalBytes : null),
  });
}

async function downloadWithSessionRetry(request: PdfDownloadRequest, destination: Directory): Promise<File> {
  try {
    return await downloadOnce(request, destination);
  } catch (failure) {
    const error = classify(failure);
    if (error.kind !== "session" || !(await refreshOnce())) {
      throw error;
    }
  }
  try {
    return await downloadOnce(request, destination);
  } catch (failure) {
    throw classify(failure);
  }
}

/** Downloads the PDF into the app cache and answers its local file URI. */
export async function downloadPdf(request: PdfDownloadRequest): Promise<string> {
  const destination = new Directory(Paths.cache, CACHE_FOLDER);
  destination.create({ idempotent: true, intermediates: true });
  const file = await downloadWithSessionRetry(request, destination);
  if (!file.name.toLowerCase().endsWith(PDF_EXTENSION)) {
    file.rename(request.fallbackName);
  }
  return file.uri;
}

/** Opens the native share / preview sheet for a local PDF. */
export async function sharePdf(uri: string, title: string): Promise<void> {
  if (!(await Sharing.isAvailableAsync())) {
    throw new PdfError("unavailable");
  }
  await Sharing.shareAsync(uri, { mimeType: PDF_MIME, UTI: PDF_UTI, dialogTitle: title });
}

/** "1.er Trimestre", v2 → "boleta-1er-trimestre-v2.pdf". */
export function fallbackPdfName(termName: string, version: number): string {
  const slug = termName
    .normalize("NFKD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/\./g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return `boleta-${slug || "periodo"}-v${version}${PDF_EXTENSION}`;
}
