/**
 * Taking the file in: one .xlsx, at most 2 MB, read as base64 in the browser.
 * The gateway checks all of this again (it is the boundary); checking here
 * spares a person a 2 MB upload only to hear it was the wrong kind of file.
 */
import { NOT_XLSX, ONE_FILE, TOO_LARGE } from "./copy";

/** The gateway's own limit (ADR 0033): 2 MB decoded. */
export const MAX_FILE_BYTES = 2 * 1024 * 1024;

const XLSX = /\.xlsx$/i;

/** The file to read, or why none can be; `notice` when extra files were dropped. */
export interface PickedFile {
  file?: File;
  notice?: string;
  error?: string;
}

export function pickFile(files: readonly File[]): PickedFile {
  const [file] = files;
  if (file === undefined) return {};
  if (!XLSX.test(file.name)) return { error: NOT_XLSX };
  if (file.size > MAX_FILE_BYTES) return { error: TOO_LARGE };
  return files.length > 1 ? { file, notice: ONE_FILE } : { file };
}

/** The file's bytes as base64, the shape StudentImportRequest.fileBase64 takes. */
export function readAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => {
      const url = typeof reader.result === "string" ? reader.result : "";
      resolve(url.slice(url.indexOf(",") + 1));
    };
    reader.onerror = () => reject(reader.error ?? new Error("could not read the file"));
    reader.readAsDataURL(file);
  });
}

const STUDENTS_IMPORT = "students:import";

/**
 * Whether the principal may migrate pupils (owner-only). Unknown permissions
 * offer nothing. UX only — the gateway's 403 is the boundary.
 */
export function canImportStudents(permissions: readonly string[] | undefined): boolean {
  return permissions !== undefined && permissions.includes(STUDENTS_IMPORT);
}
