import { Directory, File, Paths } from "expo-file-system";
import * as Sharing from "expo-sharing";
import { refreshOnce } from "../api/client";
import { apiUrl } from "../api/config";
import { ApiError } from "../api/errors";
import { getAccessToken } from "../api/tokenStore";

/**
 * Downloads an authenticated PDF (a boleta) into the app's cache and hands it
 * to the phone's native share / preview sheet.
 *
 * The download is native (expo-file-system `File.downloadFileAsync`) so the
 * bytes never cross the JS bridge. Each child has a folder
 * (`cache/boletas/<studentId>/`), so siblings never collide; the file keeps
 * the server's Content-Disposition name (`boleta-<matrícula>-<periodo>-v<N>.pdf`)
 * or, when that did not reach the native layer, is renamed to
 * {@link fallbackPdfName} — never an id, since the parent sees the name when
 * sharing. Everything else under `boletas/` is deleted first, so the cache holds
 * only the boleta being opened and a re-download overwrites it
 * (and {@link clearPdfCache} empties it on sign-out). Native downloads bypass {@link apiFetch}, so an expired access
 * token is handled here the same way: refresh once, retry once.
 */

export type PdfErrorKind = "network" | "server" | "not_found" | "session" | "storage" | "unavailable";

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
  /** The child the boleta belongs to: names its cache folder, never the file. */
  studentId: string;
  /** Used when the server's filename did not reach the native layer, from {@link fallbackPdfName}. */
  fallbackName: string;
  /** Fraction 0–1, or null when the server did not send a size. */
  onProgress?: (fraction: number | null) => void;
}

const CACHE_FOLDER = "boletas";
const PDF_EXTENSION = ".pdf";
const PDF_MIME = "application/pdf";
const PDF_UTI = "com.adobe.pdf";
const HTTP_UNAUTHORIZED = 401;
const HTTP_NOT_FOUND = 404;
/**
 * expo-file-system 56 rejects a non-2xx download with UnableToDownloadException:
 * iOS "Unable to download a file: response has status 401", Android
 * "Unable to download a file: response has status: 401".
 */
const HTTP_STATUS = /status:?\s*(\d{3})\b/;

function bearer(): Record<string, string> {
  const token = getAccessToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function messageOf(failure: unknown): string {
  return failure instanceof Error ? failure.message : String(failure);
}

function httpStatusOf(message: string): number | null {
  const match = HTTP_STATUS.exec(message);
  return match ? Number(match[1]) : null;
}

function classify(failure: unknown): PdfError {
  if (failure instanceof PdfError) return failure;
  const message = messageOf(failure);
  // A refresh that failed on a 5xx rejects with an ApiError, which carries its
  // status as a field rather than in the native layer's message.
  const status = failure instanceof ApiError ? failure.status : httpStatusOf(message);
  if (status === HTTP_UNAUTHORIZED) return new PdfError("session", message);
  if (status === HTTP_NOT_FOUND) return new PdfError("not_found", message);
  if (status !== null) return new PdfError("server", message);
  return new PdfError("network", message);
}

function cacheFolder(): Directory {
  return new Directory(Paths.cache, CACHE_FOLDER);
}

function storageError(failure: unknown): PdfError {
  return new PdfError("storage", messageOf(failure));
}

/**
 * Empties `boletas/` except this child's (now empty) folder and answers that
 * folder. A path-like student id is slugged so it stays inside `boletas/`.
 */
function prepareStudentFolder(studentId: string): Directory {
  try {
    const root = cacheFolder();
    const folderName = slug(studentId) || "alumno";
    root.create({ idempotent: true, intermediates: true });
    for (const entry of root.list()) {
      if (entry.name !== folderName) entry.delete();
    }
    const folder = new Directory(root, folderName);
    folder.create({ idempotent: true, intermediates: true });
    for (const entry of folder.list()) entry.delete();
    return folder;
  } catch (failure) {
    throw storageError(failure);
  }
}

/** Gives a file the native layer named from the URL ("pdf") its readable fallback name. */
function ensurePdfName(file: File, fallbackName: string): File {
  if (file.name.toLowerCase().endsWith(PDF_EXTENSION)) return file;
  try {
    file.rename(fallbackName);
    return file;
  } catch (failure) {
    throw storageError(failure);
  }
}

async function downloadOnce(request: PdfDownloadRequest, folder: Directory): Promise<File> {
  return File.downloadFileAsync(apiUrl(request.path), folder, {
    headers: bearer(),
    idempotent: true,
    onProgress: ({ bytesWritten, totalBytes }) =>
      request.onProgress?.(totalBytes > 0 ? bytesWritten / totalBytes : null),
  });
}

/** Rotates the session; a refresh that fails (5xx, offline) is classified like a download. */
async function refreshOrClassify(): Promise<boolean> {
  try {
    return await refreshOnce();
  } catch (failure) {
    throw classify(failure);
  }
}

async function downloadWithSessionRetry(request: PdfDownloadRequest, folder: Directory): Promise<File> {
  try {
    return await downloadOnce(request, folder);
  } catch (failure) {
    const error = classify(failure);
    if (error.kind !== "session" || !(await refreshOrClassify())) {
      throw error;
    }
  }
  try {
    return await downloadOnce(request, folder);
  } catch (failure) {
    throw classify(failure);
  }
}

/** Downloads the PDF into the child's cache folder and answers its local file URI. */
export async function downloadPdf(request: PdfDownloadRequest): Promise<string> {
  const folder = prepareStudentFolder(request.studentId);
  const file = await downloadWithSessionRetry(request, folder);
  return ensurePdfName(file, request.fallbackName).uri;
}

/**
 * Deletes every downloaded boleta — called on sign-out, so a shared or lost
 * phone keeps no child's grades behind. Throws when the phone refuses.
 */
export function clearPdfCache(): void {
  const folder = cacheFolder();
  if (folder.exists) folder.delete();
}

/** Opens the native share / preview sheet for a local PDF. */
export async function sharePdf(uri: string, title: string): Promise<void> {
  if (!(await Sharing.isAvailableAsync())) {
    throw new PdfError("unavailable");
  }
  await Sharing.shareAsync(uri, { mimeType: PDF_MIME, UTI: PDF_UTI, dialogTitle: title });
}

export interface PdfFileNameParts {
  /** 0 when the card predates term ordinals. */
  termOrdinal: number;
  termName: string;
  version: number;
}

function slug(text: string): string {
  return text
    .normalize("NFKD")
    .replace(/[\u0300-\u036f]/g, "")
    .toLowerCase()
    .replace(/\./g, "")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
}

/** "1.er Trimestre", v2 → "boleta-1er-trimestre-v2.pdf" — no ids: the parent sees this name when sharing. */
export function fallbackPdfName({ termOrdinal, termName, version }: PdfFileNameParts): string {
  const term = slug(termName) || (termOrdinal > 0 ? `t${termOrdinal}` : "periodo");
  return `boleta-${term}-v${version}${PDF_EXTENSION}`;
}
