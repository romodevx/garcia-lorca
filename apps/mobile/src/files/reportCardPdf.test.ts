import * as Sharing from "expo-sharing";
import { clearTokens, saveTokens } from "../api/tokenStore";
import { clearPdfCache, downloadPdf, fallbackPdfName, PdfError, sharePdf } from "./reportCardPdf";

type DownloadOptions = { headers?: Record<string, string>; idempotent?: boolean; onProgress?: (p: { bytesWritten: number; totalBytes: number }) => void };
type Folder = { uri: string };
type Written = { uri: string; name: string };

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
const mockRenamed: string[] = [];
/** Folder uri → names inside it. */
const mockCache = { exists: true, entries: {} as Record<string, string[]>, failDelete: false, failRename: false };

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
    rename(next: string): void {
      if (mockCache.failRename) throw new Error("Unable to move file: exists");
      mockRenamed.push(next);
      this.uri = `${this.uri.slice(0, this.uri.lastIndexOf("/"))}/${next}`;
      this.name = next;
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
      mockCreate(this.uri, options);
    }
    list(): MockFile[] {
      return (mockCache.entries[this.uri] ?? []).map((name) => new MockFile(this, name));
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

const { File: MockFile } = jest.requireMock("expo-file-system") as { File: new (...parts: Array<string | Folder>) => Written };

/** Resolves like the native download into a folder: the File it named `name`. */
function writes(name: string): void {
  mockDownload.mockImplementation(async (_url: string, folder: Folder) => new MockFile(folder, name));
}

function refreshSucceeds(): void {
  globalThis.fetch = jest.fn(async () => ({
    ok: true,
    status: 200,
    json: async () => ({ accessToken: "a2", refreshToken: "r2" }),
  })) as unknown as typeof fetch;
}

const ROOT = "file:///cache/boletas";
const SERVER_NAME = "boleta-A0123-1er-trimestre-v2.pdf";
const REQUEST = { path: "/api/v1/x/pdf", studentId: "s1", fallbackName: "boleta-1er-trimestre-v2.pdf" };

describe("downloadPdf", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    mockDeleted.length = 0;
    mockRenamed.length = 0;
    Object.assign(mockCache, { exists: true, entries: {}, failDelete: false, failRename: false });
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("downloads into the child's own folder with the Bearer token, keeping the server's filename", async () => {
    writes(SERVER_NAME);

    const uri = await downloadPdf(REQUEST);

    const [url, folder, options] = mockDownload.mock.calls[0] as [string, Folder, DownloadOptions];
    expect(url).toMatch(/\/api\/v1\/x\/pdf$/);
    expect(folder.uri).toBe(`${ROOT}/s1`);
    expect(options.headers).toEqual({ Authorization: "Bearer a1" });
    expect(options.idempotent).toBe(true);
    expect(mockCreate).toHaveBeenCalledWith(`${ROOT}/s1`, { idempotent: true, intermediates: true });
    expect(uri).toBe(`${ROOT}/s1/${SERVER_NAME}`);
    expect(mockRenamed).toEqual([]);
  });

  it("names the file boleta-<periodo>-v<N>.pdf (no ids) when the server gave no filename", async () => {
    writes("pdf");

    const uri = await downloadPdf(REQUEST);

    expect(mockRenamed).toEqual(["boleta-1er-trimestre-v2.pdf"]);
    expect(uri).toBe(`${ROOT}/s1/boleta-1er-trimestre-v2.pdf`);
    expect(uri).not.toMatch(/boleta-s1/);
  });

  it("downloads the same boleta a second time onto the same file", async () => {
    writes(SERVER_NAME);
    await downloadPdf(REQUEST);
    mockCache.entries = { [ROOT]: ["s1"], [`${ROOT}/s1`]: [SERVER_NAME] };

    const uri = await downloadPdf(REQUEST);

    expect(uri).toBe(`${ROOT}/s1/${SERVER_NAME}`);
    expect((mockDownload.mock.calls[1] as [string, Folder, DownloadOptions])[2].idempotent).toBe(true);
  });

  it("keeps siblings apart: each child downloads into a separate folder", async () => {
    writes("pdf");

    const mine = await downloadPdf(REQUEST);
    const sibling = await downloadPdf({ ...REQUEST, studentId: "s2" });

    expect(mine).toBe(`${ROOT}/s1/boleta-1er-trimestre-v2.pdf`);
    expect(sibling).toBe(`${ROOT}/s2/boleta-1er-trimestre-v2.pdf`);
  });

  it("keeps only the current boleta: other children's folders and older files go first", async () => {
    writes(SERVER_NAME);
    mockCache.entries = { [ROOT]: ["s2", "s1"], [`${ROOT}/s1`]: ["boleta-A0123-1er-trimestre-v1.pdf"] };

    await downloadPdf(REQUEST);

    expect(mockDeleted).toEqual([`${ROOT}/s2`, `${ROOT}/s1/boleta-A0123-1er-trimestre-v1.pdf`]);
  });

  it("keeps a student id with path characters inside the boletas folder", async () => {
    writes(SERVER_NAME);

    await downloadPdf({ ...REQUEST, studentId: "../x" });

    expect((mockDownload.mock.calls[0] as [string, Folder])[1].uri).toBe(`${ROOT}/x`);
  });

  it("fails as storage when the phone cannot clear the folder", async () => {
    mockCache.entries = { [ROOT]: ["s2"] };
    mockCache.failDelete = true;

    await expect(downloadPdf(REQUEST)).rejects.toMatchObject({ kind: "storage" });
    expect(mockDownload).not.toHaveBeenCalled();
  });

  it("fails as storage when the downloaded file cannot be renamed", async () => {
    writes("pdf");
    mockCache.failRename = true;

    await expect(downloadPdf(REQUEST)).rejects.toMatchObject({ kind: "storage" });
  });

  it("reports progress as a fraction, or null when the size is unknown", async () => {
    mockDownload.mockImplementation(async (_u: string, folder: Folder, options: DownloadOptions) => {
      options.onProgress?.({ bytesWritten: 50, totalBytes: 200 });
      options.onProgress?.({ bytesWritten: 50, totalBytes: -1 });
      return new MockFile(folder, SERVER_NAME);
    });
    const onProgress = jest.fn();

    await downloadPdf({ ...REQUEST, onProgress });

    expect(onProgress.mock.calls).toEqual([[0.25], [null]]);
  });

  it.each([
    ["iOS", IOS_401],
    ["Android", ANDROID_401],
  ])("refreshes an expired session once and retries with the new token (%s)", async (_platform, message) => {
    refreshSucceeds();
    writes(SERVER_NAME);
    mockDownload.mockRejectedValueOnce(new Error(message));

    await downloadPdf(REQUEST);

    expect(mockDownload).toHaveBeenCalledTimes(2);
    expect((mockDownload.mock.calls[1] as [string, unknown, DownloadOptions])[2].headers).toEqual({ Authorization: "Bearer a2" });
  });

  // The refresh keeps the session on a 5xx and rejects with an ApiError; the
  // download must read that as the school's server, not as "no connection".
  it("fails as server (not network) when the refresh hits a 5xx", async () => {
    globalThis.fetch = jest.fn(async () => ({ ok: false, status: 503, json: async () => ({}) })) as unknown as typeof fetch;
    mockDownload.mockRejectedValueOnce(new Error(ANDROID_401));

    await expect(downloadPdf(REQUEST)).rejects.toMatchObject({ kind: "server" });
    expect(mockDownload).toHaveBeenCalledTimes(1);
  });

  it("fails as network when the refresh cannot reach the gateway", async () => {
    globalThis.fetch = jest.fn(async () => {
      throw new TypeError("Network request failed");
    }) as unknown as typeof fetch;
    mockDownload.mockRejectedValueOnce(new Error(ANDROID_401));

    await expect(downloadPdf(REQUEST)).rejects.toMatchObject({ kind: "network" });
  });

  it("fails as session when the retry is still refused", async () => {
    refreshSucceeds();
    mockDownload.mockRejectedValue(new Error(ANDROID_401));

    await expect(downloadPdf(REQUEST)).rejects.toMatchObject({ kind: "session" });
    expect(mockDownload).toHaveBeenCalledTimes(2);
  });

  it.each([
    ["iOS 404", IOS_404, "not_found"],
    ["Android 404", ANDROID_404, "not_found"],
    ["Android 500", ANDROID_500, "server"],
    ["iOS offline", IOS_OFFLINE, "network"],
  ])("classifies %s", async (_case, message, kind) => {
    mockDownload.mockRejectedValueOnce(new Error(message));

    const failure = downloadPdf(REQUEST);

    await expect(failure).rejects.toBeInstanceOf(PdfError);
    await expect(failure).rejects.toMatchObject({ kind });
    expect(mockDownload).toHaveBeenCalledTimes(1);
  });
});

describe("clearPdfCache", () => {
  beforeEach(() => {
    mockDeleted.length = 0;
    Object.assign(mockCache, { exists: true, entries: {}, failDelete: false });
  });

  it("deletes the whole boletas folder", () => {
    clearPdfCache();
    expect(mockDeleted).toEqual([ROOT]);
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

describe("fallbackPdfName", () => {
  it("slugs the term name without accents, punctuation or ids", () => {
    expect(fallbackPdfName({ termOrdinal: 1, termName: "1.er Trimestre", version: 2 })).toBe("boleta-1er-trimestre-v2.pdf");
    expect(fallbackPdfName({ termOrdinal: 0, termName: "2.º periodo", version: 1 })).toBe("boleta-2o-periodo-v1.pdf");
  });

  it("falls back to the term ordinal, then to a generic word, when the name slugs to nothing", () => {
    expect(fallbackPdfName({ termOrdinal: 3, termName: "", version: 1 })).toBe("boleta-t3-v1.pdf");
    expect(fallbackPdfName({ termOrdinal: 0, termName: "", version: 1 })).toBe("boleta-periodo-v1.pdf");
  });
});
