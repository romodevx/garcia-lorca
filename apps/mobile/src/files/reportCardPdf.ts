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
 * bytes never cross the JS bridge. It writes straight to a deterministic file
 * named by student, term and version, overwriting a previous download of the
 * same boleta; any other file in the folder is deleted first, so the cache
 * holds only the boleta being opened (and {@link clearPdfCache} empties it on
 * sign-out). Native downloads bypass {@link apiFetch}, so an expired access
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
  /** The cache file to write, from {@link pdfFileName}. */
  fileName: string;
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
  const status = httpStatusOf(message);
  if (status === HTTP_UNAUTHORIZED) return new PdfError("session", message);
  if (status === HTTP_NOT_FOUND) return new PdfError("not_found", message);
  if (status !== null) return new PdfError("server", message);
  return new PdfError("network", message);
}

function cacheFolder(): Directory {
  return new Directory(Paths.cache, CACHE_FOLDER);
}

/** Creates the cache folder, deletes everything in it but `fileName`, and answers that target. */
function prepareTarget(fileName: string): File {
  try {
    const folder = cacheFolder();
    folder.create({ idempotent: true, intermediates: true });
    for (const entry of folder.list()) {
      if (entry.name !== fileName) entry.delete();
    }
    return new File(folder, fileName);
  } catch (failure) {
    throw new PdfError("storage", messageOf(failure));
  }
}

async function downloadOnce(request: PdfDownloadRequest, target: File): Promise<File> {
  return File.downloadFileAsync(apiUrl(request.path), target, {
    headers: bearer(),
    idempotent: true,
    onProgress: ({ bytesWritten, totalBytes }) =>
      request.onProgress?.(totalBytes > 0 ? bytesWritten / totalBytes : null),
  });
}

async function downloadWithSessionRetry(request: PdfDownloadRequest, target: File): Promise<File> {
  try {
    return await downloadOnce(request, target);
  } catch (failure) {
    const error = classify(failure);
    if (error.kind !== "session" || !(await refreshOnce())) {
      throw error;
    }
  }
  try {
    return await downloadOnce(request, target);
  } catch (failure) {
    throw classify(failure);
  }
}

/** Downloads the PDF into the app cache and answers its local file URI. */
export async function downloadPdf(request: PdfDownloadRequest): Promise<string> {
  const target = prepareTarget(request.fileName);
  const file = await downloadWithSessionRetry(request, target);
  return file.uri;
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
  studentId: string;
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

/** Student s1, "1.er Trimestre", v2 → "boleta-s1-1er-trimestre-v2.pdf" (unique per child, term and version). */
export function pdfFileName({ studentId, termOrdinal, termName, version }: PdfFileNameParts): string {
  const term = slug(termName) || (termOrdinal > 0 ? `t${termOrdinal}` : "periodo");
  return `boleta-${slug(studentId)}-${term}-v${version}${PDF_EXTENSION}`;
}
