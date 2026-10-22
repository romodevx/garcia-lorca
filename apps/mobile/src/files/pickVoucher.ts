import * as DocumentPicker from "expo-document-picker";
import * as ImagePicker from "expo-image-picker";
import type { VoucherFile } from "../api";

/** Where a family takes its voucher from. */
export type VoucherSource = "camera" | "library" | "pdf";

export type VoucherPick =
  | { kind: "picked"; file: VoucherFile }
  | { kind: "canceled" }
  | { kind: "denied" }
  | { kind: "too_large" }
  /** HEIC, WebP, GIF…: the school takes JPEG, PNG and PDF only (ADR 0040). */
  | { kind: "unsupported" };

/** The gateway's cap (ADR 0040); checked here too so a family does not wait for a 413. */
export const MAX_VOUCHER_BYTES = 5 * 1024 * 1024;

// Below 1 the picker re-encodes as JPEG: a bank's screenshot stays legible,
// an iPhone's HEIC photo becomes a JPEG the school accepts, and a 12 MP
// camera shot drops far under the cap.
const PHOTO_QUALITY = 0.7;
//
// The quality alone is not enough on iOS: its gallery (PHPicker) hands over the
// HEIC as it is unless asked for the most compatible representation (JPEG).
const PHOTO_OPTIONS: ImagePicker.ImagePickerOptions = {
  mediaTypes: ["images"],
  quality: PHOTO_QUALITY,
  preferredAssetRepresentationMode: ImagePicker.UIImagePickerPreferredAssetRepresentationMode.Compatible,
};
const DEFAULT_PHOTO_NAME = "comprobante.jpg";
const DEFAULT_PHOTO_TYPE = "image/jpeg";
const PDF_TYPE = "application/pdf";

/** What the school accepts, by type, and the extensions that name each. */
const ALLOWED_TYPES: ReadonlySet<string> = new Set(["image/jpeg", "image/png", PDF_TYPE]);
const TYPE_BY_EXTENSION: Readonly<Record<string, string>> = { jpg: "image/jpeg", jpeg: "image/jpeg", png: "image/png", pdf: PDF_TYPE };

/**
 * The part's type: the platform's when it gives one, else the extension's, else
 * `fallback` for a name with no extension at all. Null when it is not a type
 * the school takes — an unknown extension included.
 */
function allowedType(mimeType: string | null | undefined, name: string, fallback: string): string | null {
  const type = mimeType ? mimeType.toLowerCase() : typeFromName(name, fallback);
  return type !== null && ALLOWED_TYPES.has(type) ? type : null;
}

function typeFromName(name: string, fallback: string): string | null {
  const dot = name.lastIndexOf(".");
  if (dot < 0) return fallback;
  return TYPE_BY_EXTENSION[name.slice(dot + 1).toLowerCase()] ?? null;
}

function checked(uri: string, name: string, type: string | null, size: number | null | undefined): VoucherPick {
  if (type === null) return { kind: "unsupported" };
  return (size ?? 0) > MAX_VOUCHER_BYTES ? { kind: "too_large" } : { kind: "picked", file: { uri, name, type } };
}

async function pickPhoto(source: "camera" | "library"): Promise<VoucherPick> {
  if (source === "camera") {
    const permission = await ImagePicker.requestCameraPermissionsAsync();
    if (!permission.granted) return { kind: "denied" };
  }
  // The gallery opens the system picker, which needs no photo permission.
  const result = source === "camera" ? await ImagePicker.launchCameraAsync(PHOTO_OPTIONS) : await ImagePicker.launchImageLibraryAsync(PHOTO_OPTIONS);
  const asset = result.canceled ? undefined : result.assets[0];
  if (asset === undefined) return { kind: "canceled" };
  const name = asset.fileName ?? DEFAULT_PHOTO_NAME;
  return checked(asset.uri, name, allowedType(asset.mimeType, name, DEFAULT_PHOTO_TYPE), asset.fileSize);
}

async function pickPdf(): Promise<VoucherPick> {
  // Copied into the cache so the upload can read it from a file:// uri.
  const result = await DocumentPicker.getDocumentAsync({ type: PDF_TYPE, copyToCacheDirectory: true });
  const asset = result.canceled ? undefined : result.assets[0];
  if (asset === undefined) return { kind: "canceled" };
  return checked(asset.uri, asset.name, allowedType(asset.mimeType, asset.name, PDF_TYPE), asset.size);
}

/** Lets the family pick its voucher: a photo (camera or gallery) or a PDF. */
export function pickVoucherFile(source: VoucherSource): Promise<VoucherPick> {
  return source === "pdf" ? pickPdf() : pickPhoto(source);
}
