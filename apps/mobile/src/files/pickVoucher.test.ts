import * as DocumentPicker from "expo-document-picker";
import * as ImagePicker from "expo-image-picker";
import { pickVoucherFile } from "./pickVoucher";

jest.mock("expo-image-picker", () => ({
  requestCameraPermissionsAsync: jest.fn(),
  launchCameraAsync: jest.fn(),
  launchImageLibraryAsync: jest.fn(),
  UIImagePickerPreferredAssetRepresentationMode: { Automatic: "automatic", Compatible: "compatible", Current: "current" },
}));
jest.mock("expo-document-picker", () => ({ getDocumentAsync: jest.fn() }));
/** What the disk says each file weighs, for a picker that does not say. */
const mockDiskSizes = new Map<string, number>();
jest.mock("expo-file-system", () => ({
  File: class {
    size: number;
    constructor(path: string) {
      this.size = mockDiskSizes.get(path) ?? 0;
    }
  },
}));

const camera = jest.mocked(ImagePicker.launchCameraAsync);
const library = jest.mocked(ImagePicker.launchImageLibraryAsync);
const cameraPermission = jest.mocked(ImagePicker.requestCameraPermissionsAsync);
const documents = jest.mocked(DocumentPicker.getDocumentAsync);

const MB = 1024 * 1024;

function photo(extra: object = {}): ImagePicker.ImagePickerResult {
  return {
    canceled: false,
    assets: [{ uri: "file:///cache/IMG_0004.jpg", width: 1, height: 1, fileName: "IMG_0004.jpg", mimeType: "image/jpeg", fileSize: 2 * MB, ...extra }],
  } as ImagePicker.ImagePickerResult;
}

describe("pickVoucherFile", () => {
  afterEach(() => {
    jest.clearAllMocks();
    mockDiskSizes.clear();
  });

  it("takes a compressed photo from the gallery, as a JPEG part", async () => {
    library.mockResolvedValue(photo());

    await expect(pickVoucherFile("library")).resolves.toEqual({
      kind: "picked",
      file: { uri: "file:///cache/IMG_0004.jpg", name: "IMG_0004.jpg", type: "image/jpeg" },
    });
    // quality < 1 re-encodes as JPEG: no HEIC, and far under 5 MB.
    expect(library).toHaveBeenCalledWith(expect.objectContaining({ mediaTypes: ["images"], quality: expect.any(Number) }));
    expect(library.mock.calls[0]?.[0]?.quality).toBeLessThan(1);
  });

  // iOS's PHPicker hands over the HEIC as it is, quality or not, unless asked
  // for the most compatible representation.
  it("asks iOS's gallery for a compatible (JPEG) representation", async () => {
    library.mockResolvedValue(photo());

    await pickVoucherFile("library");

    expect(library.mock.calls[0]?.[0]?.preferredAssetRepresentationMode).toBe("compatible");
  });

  it.each([["image/heic"], ["image/webp"], ["image/gif"]])("refuses a %s photo with its own reason", async (mimeType) => {
    library.mockResolvedValue(photo({ mimeType, fileName: "IMG_0004.x" }));

    await expect(pickVoucherFile("library")).resolves.toEqual({ kind: "unsupported" });
  });

  it.each([
    ["captura.PNG", "image/png"],
    ["foto.jpeg", "image/jpeg"],
    ["foto.jpg", "image/jpeg"],
  ])("deduces the type of %s from its extension when the platform gives none", async (fileName, type) => {
    library.mockResolvedValue(photo({ mimeType: undefined, fileName }));

    await expect(pickVoucherFile("library")).resolves.toMatchObject({ kind: "picked", file: { type } });
  });

  it("refuses a nameless, typeless file whose extension says HEIC", async () => {
    library.mockResolvedValue(photo({ mimeType: undefined, fileName: "IMG_0004.HEIC" }));

    await expect(pickVoucherFile("library")).resolves.toEqual({ kind: "unsupported" });
  });

  it("names a photo the platform did not name", async () => {
    camera.mockResolvedValue(photo({ fileName: null, mimeType: undefined }));
    cameraPermission.mockResolvedValue({ granted: true } as ImagePicker.PermissionResponse);

    await expect(pickVoucherFile("camera")).resolves.toMatchObject({ file: { name: "comprobante.jpg", type: "image/jpeg" } });
  });

  it("asks for the camera first and says so when the family refuses", async () => {
    cameraPermission.mockResolvedValue({ granted: false } as ImagePicker.PermissionResponse);

    await expect(pickVoucherFile("camera")).resolves.toEqual({ kind: "denied" });
    expect(camera).not.toHaveBeenCalled();
  });

  it("picks a PDF into the cache", async () => {
    documents.mockResolvedValue({
      canceled: false,
      assets: [{ uri: "file:///cache/pago.pdf", name: "pago.pdf", mimeType: "application/pdf", size: MB, lastModified: 0 }],
    });

    await expect(pickVoucherFile("pdf")).resolves.toEqual({
      kind: "picked",
      file: { uri: "file:///cache/pago.pdf", name: "pago.pdf", type: "application/pdf" },
    });
    expect(documents).toHaveBeenCalledWith({ type: "application/pdf", copyToCacheDirectory: true });
  });

  it("refuses a file over 5 MB before sending it", async () => {
    documents.mockResolvedValue({
      canceled: false,
      assets: [{ uri: "file:///cache/big.pdf", name: "big.pdf", mimeType: "application/pdf", size: 6 * MB, lastModified: 0 }],
    });

    await expect(pickVoucherFile("pdf")).resolves.toEqual({ kind: "too_large" });
  });

  // Some pickers leave the size out; the 5 MB cap must not be skipped then.
  it("weighs on disk a photo whose size the picker did not give, and refuses it over 5 MB", async () => {
    library.mockResolvedValue(photo({ fileSize: undefined }));
    mockDiskSizes.set("file:///cache/IMG_0004.jpg", 6 * MB);

    await expect(pickVoucherFile("library")).resolves.toEqual({ kind: "too_large" });
  });

  it("accepts a sizeless PDF that weighs under 5 MB on disk", async () => {
    documents.mockResolvedValue({
      canceled: false,
      assets: [{ uri: "file:///cache/pago.pdf", name: "pago.pdf", mimeType: "application/pdf", lastModified: 0 }],
    });
    mockDiskSizes.set("file:///cache/pago.pdf", MB);

    await expect(pickVoucherFile("pdf")).resolves.toMatchObject({ kind: "picked" });
  });

  it("reports a cancelled picker as nothing picked", async () => {
    library.mockResolvedValue({ canceled: true, assets: null });

    await expect(pickVoucherFile("library")).resolves.toEqual({ kind: "canceled" });
  });
});
