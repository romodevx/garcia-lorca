import * as Sharing from "expo-sharing";
import { clearTokens, saveTokens } from "../api/tokenStore";
import { downloadPdf, fallbackPdfName, PdfError, sharePdf } from "./reportCardPdf";

type DownloadOptions = { headers?: Record<string, string>; idempotent?: boolean; onProgress?: (p: { bytesWritten: number; totalBytes: number }) => void };

const mockDownload = jest.fn();
const mockRename = jest.fn();
const mockCreate = jest.fn();

jest.mock("expo-file-system", () => {
  class MockDirectory {
    uri: string;
    constructor(...parts: Array<string | { uri: string }>) {
      this.uri = parts.map((p) => (typeof p === "string" ? p : p.uri)).join("/");
    }
    create(options: unknown): void {
      mockCreate(options);
    }
  }
  return {
    Paths: { cache: { uri: "file:///cache" } },
    Directory: MockDirectory,
    File: { downloadFileAsync: (...args: unknown[]) => mockDownload(...args) },
  };
});

jest.mock("expo-sharing", () => ({
  isAvailableAsync: jest.fn(async () => true),
  shareAsync: jest.fn(async () => undefined),
}));

function downloaded(name: string): { uri: string; name: string; rename: (n: string) => void } {
  const file = {
    uri: `file:///cache/boletas/${name}`,
    name,
    rename(next: string) {
      mockRename(next);
      file.name = next;
      file.uri = `file:///cache/boletas/${next}`;
    },
  };
  return file;
}

describe("downloadPdf", () => {
  beforeEach(async () => {
    jest.clearAllMocks();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });
  afterEach(async () => {
    await clearTokens();
  });

  it("downloads to the app cache with the Bearer token and keeps the server's filename", async () => {
    mockDownload.mockResolvedValue(downloaded("boleta-mateo-1er-trimestre-v2.pdf"));

    const uri = await downloadPdf({ path: "/api/v1/x/pdf", fallbackName: "boleta-x-v2.pdf" });

    const [url, destination, options] = mockDownload.mock.calls[0] as [string, { uri: string }, DownloadOptions];
    expect(url).toMatch(/\/api\/v1\/x\/pdf$/);
    expect(destination.uri).toBe("file:///cache/boletas");
    expect(options.headers).toEqual({ Authorization: "Bearer a1" });
    expect(options.idempotent).toBe(true);
    expect(mockCreate).toHaveBeenCalledWith({ idempotent: true, intermediates: true });
    expect(uri).toBe("file:///cache/boletas/boleta-mateo-1er-trimestre-v2.pdf");
    expect(mockRename).not.toHaveBeenCalled();
  });

  it("names the file boleta-<periodo>-v<N>.pdf when the server gave no filename", async () => {
    mockDownload.mockResolvedValue(downloaded("pdf"));

    const uri = await downloadPdf({ path: "/api/v1/x/pdf", fallbackName: "boleta-1er-trimestre-v1.pdf" });

    expect(mockRename).toHaveBeenCalledWith("boleta-1er-trimestre-v1.pdf");
    expect(uri).toBe("file:///cache/boletas/boleta-1er-trimestre-v1.pdf");
  });

  it("reports progress as a fraction, or null when the size is unknown", async () => {
    mockDownload.mockImplementation(async (_u: string, _d: unknown, options: DownloadOptions) => {
      options.onProgress?.({ bytesWritten: 50, totalBytes: 200 });
      options.onProgress?.({ bytesWritten: 50, totalBytes: -1 });
      return downloaded("a.pdf");
    });
    const onProgress = jest.fn();

    await downloadPdf({ path: "/p", fallbackName: "b.pdf", onProgress });

    expect(onProgress.mock.calls).toEqual([[0.25], [null]]);
  });

  it("refreshes an expired session once and retries with the new token", async () => {
    globalThis.fetch = jest.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ accessToken: "a2", refreshToken: "r2" }),
    })) as unknown as typeof fetch;
    mockDownload
      .mockRejectedValueOnce(new Error("UnableToDownloadException: server returned HTTP 401"))
      .mockResolvedValueOnce(downloaded("a.pdf"));

    await downloadPdf({ path: "/p", fallbackName: "b.pdf" });

    expect(mockDownload).toHaveBeenCalledTimes(2);
    expect((mockDownload.mock.calls[1] as [string, unknown, DownloadOptions])[2].headers).toEqual({ Authorization: "Bearer a2" });
  });

  it("classifies a 404 as not found and anything else as a network failure", async () => {
    mockDownload.mockRejectedValueOnce(new Error("server returned HTTP 404"));
    await expect(downloadPdf({ path: "/p", fallbackName: "b.pdf" })).rejects.toMatchObject({ kind: "not_found" });

    mockDownload.mockRejectedValueOnce(new Error("The Internet connection appears to be offline."));
    await expect(downloadPdf({ path: "/p", fallbackName: "b.pdf" })).rejects.toBeInstanceOf(PdfError);
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
  it("slugs the term name without accents or punctuation", () => {
    expect(fallbackPdfName("1.er Trimestre", 2)).toBe("boleta-1er-trimestre-v2.pdf");
    expect(fallbackPdfName("2.º periodo", 1)).toBe("boleta-2o-periodo-v1.pdf");
    expect(fallbackPdfName("", 1)).toBe("boleta-periodo-v1.pdf");
  });
});
