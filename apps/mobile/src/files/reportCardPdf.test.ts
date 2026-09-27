import * as Sharing from "expo-sharing";
import { clearTokens, saveTokens } from "../api/tokenStore";
import { clearPdfCache, downloadPdf, pdfFileName, PdfError, sharePdf } from "./reportCardPdf";

type DownloadOptions = { headers?: Record<string, string>; idempotent?: boolean; onProgress?: (p: { bytesWritten: number; totalBytes: number }) => void };
type Target = { uri: string; name: string };

// The rejections expo-file-system 56 really raises for a non-2xx response
// (UnableToDownloadException): ios/FileSystemDownload.swift and
// android/.../FileSystemDownload.kt word the status differently.
const IOS_401 = "Unable to download a file: response has status 401";
const ANDROID_401 = "Unable to download a file: response has status: 401";
const IOS_404 = "Unable to download a file: response has status 404";
const ANDROID_404 = "Unable to download a file: response has status: 404";
const ANDROID_500 = "Unable to download a file: response has status: 500";
const IOS_OFFLINE = "Unable to download a file: The Internet connection appears to be offline.";

const mockDownload = jest.fn();
const mockCreate = jest.fn();
const mockDeleted: string[] = [];
const mockCache = { exists: true, entries: [] as string[], failDelete: false };

jest.mock("expo-file-system", () => {
  function uriOf(parts: Array<string | { uri: string }>): string {
    return parts.map((p) => (typeof p === "string" ? p : p.uri)).join("/");
  }
  function remove(uri: string): void {
    if (mockCache.failDelete) throw new Error("Unable to delete file or directory: busy");
    mockDeleted.push(uri);
  }
  class MockFile {
    uri: string;
    name: string;
    constructor(...parts: Array<string | { uri: string }>) {
      this.uri = uriOf(parts);
      this.name = this.uri.split("/").pop() ?? "";
    }
    delete(): void {
      remove(this.uri);
    }
    static downloadFileAsync(...args: unknown[]): unknown {
      return mockDownload(...args);
    }
  }
  class MockDirectory {
    uri: string;
    constructor(...parts: Array<string | { uri: string }>) {
      this.uri = uriOf(parts);
    }
    get exists(): boolean {
      return mockCache.exists;
    }
    create(options: unknown): void {
      mockCreate(options);
    }
    list(): MockFile[] {
      return mockCache.entries.map((name) => new MockFile(this, name));
    }
    delete(): void {
      remove(this.uri);
    }
  }
  return { Paths: { cache: { uri: "file:///cache" } }, Directory: MockDirectory, File: MockFile };
});

jest.mock("expo-sharing", () => ({
  isAvailableAsync: jest.fn(async () => true),
  shareAsync: jest.fn(async () => undefined),
}));

/** Resolves like the native download: the File it wrote at the requested target. */
function writesTarget(): void {
  mockDownload.mockImplementation(async (_url: string, target: Target) => target);
}

function refreshSucceeds(): void {
  globalThis.fetch = jest.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ accessToken: "a2", refreshToken: "r2" }),
  })) as unknown as typeof fetch;
}

const NAME = "boleta-s1-1er-trimestre-v2.pdf";

describe("downloadPdf", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    mockDeleted.length = 0;
    Object.assign(mockCache, { exists: true, entries: [], failDelete: false });
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("downloads straight to its named file in the cache, with the Bearer token, overwriting", async () => {
    writesTarget();

    const uri = await downloadPdf({ path: "/api/v1/x/pdf", fileName: NAME });

    const [url, target, options] = mockDownload.mock.calls[0] as [string, Target, DownloadOptions];
    expect(url).toMatch(/\/api\/v1\/x\/pdf$/);
    expect(target.uri).toBe(`file:///cache/boletas/${NAME}`);
    expect(options.headers).toEqual({ Authorization: "Bearer a1" });
    expect(options.idempotent).toBe(true);
    expect(mockCreate).toHaveBeenCalledWith({ idempotent: true, intermediates: true });
    expect(uri).toBe(`file:///cache/boletas/${NAME}`);
  });

  it("downloads the same boleta a second time onto the same file", async () => {
    writesTarget();
    await downloadPdf({ path: "/p", fileName: NAME });
    mockCache.entries = [NAME];

    const uri = await downloadPdf({ path: "/p", fileName: NAME });

    expect(uri).toBe(`file:///cache/boletas/${NAME}`);
    expect(mockDeleted).toEqual([]);
    expect((mockDownload.mock.calls[1] as [string, Target, DownloadOptions])[2].idempotent).toBe(true);
  });

  it("keeps only the current boleta: a sibling's earlier download is deleted first", async () => {
    writesTarget();
    mockCache.entries = ["boleta-s2-1er-trimestre-v1.pdf", NAME, "pdf"];

    await downloadPdf({ path: "/p", fileName: NAME });

    expect(mockDeleted).toEqual(["file:///cache/boletas/boleta-s2-1er-trimestre-v1.pdf", "file:///cache/boletas/pdf"]);
  });

  it("fails as storage when the phone cannot write the file", async () => {
    mockCache.entries = ["old.pdf"];
    mockCache.failDelete = true;

    await expect(downloadPdf({ path: "/p", fileName: NAME })).rejects.toMatchObject({ kind: "storage" });
    expect(mockDownload).not.toHaveBeenCalled();
  });

  it("reports progress as a fraction, or null when the size is unknown", async () => {
    mockDownload.mockImplementation(async (_u: string, target: Target, options: DownloadOptions) => {
      options.onProgress?.({ bytesWritten: 50, totalBytes: 200 });
      options.onProgress?.({ bytesWritten: 50, totalBytes: -1 });
      return target;
    });
    const onProgress = jest.fn();

    await downloadPdf({ path: "/p", fileName: NAME, onProgress });

    expect(onProgress.mock.calls).toEqual([[0.25], [null]]);
  });

  it.each([
    ["iOS", IOS_401],
    ["Android", ANDROID_401],
  ])("refreshes an expired session once and retries with the new token (%s)", async (_platform, message) => {
    refreshSucceeds();
    mockDownload.mockRejectedValueOnce(new Error(message)).mockImplementationOnce(async (_u: string, target: Target) => target);

    await downloadPdf({ path: "/p", fileName: NAME });

    expect(mockDownload).toHaveBeenCalledTimes(2);
    expect((mockDownload.mock.calls[1] as [string, unknown, DownloadOptions])[2].headers).toEqual({ Authorization: "Bearer a2" });
  });

  it("fails as session when the retry is still refused", async () => {
    refreshSucceeds();
    mockDownload.mockRejectedValue(new Error(ANDROID_401));

    await expect(downloadPdf({ path: "/p", fileName: NAME })).rejects.toMatchObject({ kind: "session" });
    expect(mockDownload).toHaveBeenCalledTimes(2);
  });

  it.each([
    ["iOS 404", IOS_404, "not_found"],
    ["Android 404", ANDROID_404, "not_found"],
    ["Android 500", ANDROID_500, "server"],
    ["iOS offline", IOS_OFFLINE, "network"],
  ])("classifies %s", async (_case, message, kind) => {
    mockDownload.mockRejectedValueOnce(new Error(message));

    const failure = downloadPdf({ path: "/p", fileName: NAME });

    await expect(failure).rejects.toBeInstanceOf(PdfError);
    await expect(failure).rejects.toMatchObject({ kind });
    expect(mockDownload).toHaveBeenCalledTimes(1);
  });
});

describe("clearPdfCache", () => {
  beforeEach(() => {
    mockDeleted.length = 0;
    Object.assign(mockCache, { exists: true, entries: [], failDelete: false });
  });

  it("deletes the whole boletas folder", () => {
    clearPdfCache();
    expect(mockDeleted).toEqual(["file:///cache/boletas"]);
  });

  it("does nothing when nothing was ever downloaded", () => {
    mockCache.exists = false;
    clearPdfCache();
    expect(mockDeleted).toEqual([]);
  });
});

describe("sharePdf", () => {
  beforeEach(() => jest.clearAllMocks());

  it("opens the native share / preview sheet as a PDF", async () => {
    await sharePdf("file:///cache/boletas/a.pdf", "Boleta 1.er trimestre");

    expect(Sharing.shareAsync).toHaveBeenCalledWith("file:///cache/boletas/a.pdf", {
      mimeType: "application/pdf",
      UTI: "com.adobe.pdf",
      dialogTitle: "Boleta 1.er trimestre",
    });
  });

  it("fails as unavailable when the phone cannot share", async () => {
    (Sharing.isAvailableAsync as jest.Mock).mockResolvedValueOnce(false);

    await expect(sharePdf("file:///a.pdf", "t")).rejects.toMatchObject({ kind: "unavailable" });
    expect(Sharing.shareAsync).not.toHaveBeenCalled();
  });
});

describe("pdfFileName", () => {
  it("names the file by student, term and version, without accents or punctuation", () => {
    expect(pdfFileName({ studentId: "s1", termOrdinal: 1, termName: "1.er Trimestre", version: 2 })).toBe("boleta-s1-1er-trimestre-v2.pdf");
    expect(pdfFileName({ studentId: "s1", termOrdinal: 0, termName: "2.º periodo", version: 1 })).toBe("boleta-s1-2o-periodo-v1.pdf");
  });

  it("falls back to the term ordinal, then to a generic word, when the name slugs to nothing", () => {
    expect(pdfFileName({ studentId: "s1", termOrdinal: 3, termName: "", version: 1 })).toBe("boleta-s1-t3-v1.pdf");
    expect(pdfFileName({ studentId: "s1", termOrdinal: 0, termName: "", version: 1 })).toBe("boleta-s1-periodo-v1.pdf");
  });

  it("gives siblings different files for the same term", () => {
    const term = { termOrdinal: 1, termName: "1.er Trimestre", version: 1 };
    expect(pdfFileName({ studentId: "s1", ...term })).not.toBe(pdfFileName({ studentId: "s2", ...term }));
  });

  it("keeps a student id with path characters inside the folder", () => {
    expect(pdfFileName({ studentId: "../x", termOrdinal: 1, termName: "T1", version: 1 })).toBe("boleta-x-t1-v1.pdf");
  });
});
