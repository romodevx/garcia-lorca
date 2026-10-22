import * as DocumentPicker from "expo-document-picker";
import * as ImagePicker from "expo-image-picker";
import type { VoucherFile } from "../api";

/** Where a family takes its voucher from. */
export type VoucherSource = "camera" | "library" | "pdf";

export type VoucherPick =
  | { kind: "picked"; file: VoucherFile }
  | { kind: "canceled" }
  | { kind: "denied" }
  | { kind: "too_large" };

/** The gateway's cap (ADR 0040); checked here too so a family does not wait for a 413. */
export const MAX_VOUCHER_BYTES = 5 * 1024 * 1024;

// Below 1 the picker re-encodes as JPEG: a bank's screenshot stays legible,
// an iPhone's HEIC photo becomes a JPEG the school accepts, and a 12 MP
// camera shot drops far under the cap.
const PHOTO_QUALITY = 0.7;
const PHOTO_OPTIONS: ImagePicker.ImagePickerOptions = { mediaTypes: ["images"], quality: PHOTO_QUALITY };
const DEFAULT_PHOTO_NAME = "comprobante.jpg";
const DEFAULT_PHOTO_TYPE = "image/jpeg";
const PDF_TYPE = "application/pdf";

function checked(file: VoucherFile, size: number | null | undefined): VoucherPick {
  return (size ?? 0) > MAX_VOUCHER_BYTES ? { kind: "too_large" } : { kind: "picked", file };
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
  return checked(
    { uri: asset.uri, name: asset.fileName ?? DEFAULT_PHOTO_NAME, type: asset.mimeType ?? DEFAULT_PHOTO_TYPE },
    asset.fileSize,
  );
}

async function pickPdf(): Promise<VoucherPick> {
  // Copied into the cache so the upload can read it from a file:// uri.
  const result = await DocumentPicker.getDocumentAsync({ type: PDF_TYPE, copyToCacheDirectory: true });
  const asset = result.canceled ? undefined : result.assets[0];
  if (asset === undefined) return { kind: "canceled" };
  return checked({ uri: asset.uri, name: asset.name, type: asset.mimeType ?? PDF_TYPE }, asset.size);
}

/** Lets the family pick its voucher: a photo (camera or gallery) or a PDF. */
export function pickVoucherFile(source: VoucherSource): Promise<VoucherPick> {
  return source === "pdf" ? pickPdf() : pickPhoto(source);
}
