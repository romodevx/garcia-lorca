import AsyncStorage from "@react-native-async-storage/async-storage";
import { listPaymentVouchers, sendPaymentVoucher, VoucherSendError, type VoucherFile } from "./paymentVouchers";
import { clearTokens, saveTokens } from "./tokenStore";

interface FakeResponse {
  status: number;
  body?: unknown;
  headers?: Record<string, string>;
}

function respond({ status, body = {}, headers = {} }: FakeResponse): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: { get: (name: string) => headers[name.toLowerCase()] ?? null },
    json: async () => body,
    text: async () => (typeof body === "string" ? body : JSON.stringify(body)),
  } as unknown as Response;
}

const PHOTO: VoucherFile = { uri: "file:///cache/IMG_0004.jpg", name: "IMG_0004.jpg", type: "image/jpeg" };
const PENDING = { id: "v1", status: "pending", createdAt: "2026-10-21T15:00:00Z" };

/**
 * The phone's request path, not Node's. Expo SDK 56 replaces `fetch` with
 * expo/fetch, which turns a FormData body into bytes with
 * `normalizeBodyInitAsync` — and refuses React Native's `{ uri, name, type }`
 * part ("Unsupported FormDataPart implementation"), which is how the upload
 * died on the device before leaving it. So these tests serialise the body with
 * expo/fetch's own `normalizeBodyInitAsync`; a part it cannot send fails here too.
 */
// eslint-disable-next-line @typescript-eslint/no-require-imports
const { normalizeBodyInitAsync } = require("expo/src/winter/fetch/RequestUtils") as {
  normalizeBodyInitAsync: (body: unknown) => Promise<{ body: Uint8Array | null; overriddenHeaders?: [string, string][] }>;
};

/**
 * FormData as Expo's runtime patches React Native's: `append` keeps objects as
 * they are and `entries` yields [name, value]. (Expo's patch itself does not
 * install under Jest's transform, so this mirrors its contract.)
 */
class DeviceFormData {
  private readonly parts: [string, unknown][] = [];
  append(name: string, value: unknown): void {
    this.parts.push([name, typeof value === "object" ? value : String(value)]);
  }
  entries(): [string, unknown][] {
    return this.parts;
  }
}

jest.mock("expo-file-system", () => ({
  File: class {
    path: string;
    constructor(path: string) {
      this.path = path;
    }
    async bytes(): Promise<Uint8Array> {
      return new TextEncoder().encode(`<bytes of ${this.path}>`);
    }
  },
}));

/** What expo/fetch would put on the wire for this request. */
async function wireOf(init: RequestInit): Promise<{ text: string; contentType: string | undefined }> {
  const { body, overriddenHeaders } = await normalizeBodyInitAsync(init.body);
  return {
    text: new TextDecoder().decode(body ?? new Uint8Array()),
    contentType: overriddenHeaders?.find(([key]) => key === "Content-Type")?.[1],
  };
}

/** The multipart parts on the wire: name → { filename?, type?, value }. */
function partsOf(text: string): Record<string, { filename?: string; type?: string; value: string }> {
  const boundary = text.slice(0, text.indexOf("\r\n"));
  const parts = text.split(boundary).slice(1, -1);
  return Object.fromEntries(
    parts.map((raw) => {
      const [head = "", value = ""] = raw.replace(/^\r\n/, "").replace(/\r\n$/, "").split("\r\n\r\n");
      const name = /name="([^"]+)"/.exec(head)?.[1] ?? "";
      const filename = /filename="([^"]+)"/.exec(head)?.[1];
      const type = /content-type: (.+)/.exec(head)?.[1];
      return [name, { ...(filename ? { filename } : {}), ...(type ? { type } : {}), value }];
    }),
  );
}

const scope = globalThis as unknown as { FormData: unknown };
const NodeFormData = scope.FormData;
beforeAll(() => {
  scope.FormData = DeviceFormData;
});
afterAll(() => {
  scope.FormData = NodeFormData;
});

describe("payment vouchers API (ADR 0040)", () => {
  beforeEach(async () => {
    await AsyncStorage.clear();
    await clearTokens();
    await saveTokens({ accessToken: "a1", refreshToken: "r1" });
  });

  describe("sendPaymentVoucher", () => {
    it("posts the file, amount and day as multipart with the Bearer token", async () => {
      const fetchMock = jest.fn().mockResolvedValue(respond({ status: 201, body: { voucher: PENDING, replayed: false } }));
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      const result = await sendPaymentVoucher("s 1", { file: PHOTO, amountCents: 250000, paidOn: "2026-10-20" });

      const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
      expect(url).toMatch(/\/api\/v1\/family\/students\/s%201\/payment-vouchers$/);
      expect(init.method).toBe("POST");
      expect((init.headers as Record<string, string>).Authorization).toBe("Bearer a1");
      // The boundary is the platform's to write: a hand-set Content-Type lacks it.
      expect((init.headers as Record<string, string>)["Content-Type"]).toBeUndefined();
      const wire = await wireOf(init);
      expect(wire.contentType).toMatch(/^multipart\/form-data; boundary=/);
      expect(partsOf(wire.text)).toEqual({
        file: { filename: "IMG_0004.jpg", type: "image/jpeg", value: "<bytes of file:///cache/IMG_0004.jpg>" },
        amountCents: { value: "250000" },
        paidOn: { value: "2026-10-20" },
      });
      expect(result).toEqual({ voucher: PENDING, replayed: false });
    });

    it("leaves out what the family did not say", async () => {
      const fetchMock = jest.fn().mockResolvedValue(respond({ status: 201, body: { voucher: PENDING, replayed: false } }));
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      await sendPaymentVoucher("s1", { file: PHOTO });

      expect(Object.keys(partsOf((await wireOf(fetchMock.mock.calls[0][1] as RequestInit)).text))).toEqual(["file"]);
    });

    it("reports the same file sent again as replayed", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(respond({ status: 200, body: { voucher: PENDING, replayed: true } })) as unknown as typeof fetch;

      await expect(sendPaymentVoucher("s1", { file: PHOTO })).resolves.toEqual({ voucher: PENDING, replayed: true });
    });

    it.each([
      [{ status: 413, body: "the voucher is larger than 5 MB" }, "too_large"],
      [{ status: 409, body: "ya hay tres", headers: { "x-error-code": "VOUCHER_LIMIT" } }, "limit"],
      [{ status: 409, body: "otro" }, "conflict"],
      [{ status: 400, body: "paidOn must be a date like 2026-10-20" }, "invalid"],
      [{ status: 401 }, "session"],
      [{ status: 404 }, "unavailable"],
      [{ status: 503 }, "server"],
    ] as [FakeResponse, string][])("turns HTTP %j into the %s refusal", async (res, reason) => {
      globalThis.fetch = jest.fn().mockResolvedValue(respond(res)) as unknown as typeof fetch;

      await expect(sendPaymentVoucher("s1", { file: PHOTO })).rejects.toMatchObject({ reason });
    });

    it("carries how long to wait on a 429", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(respond({ status: 429, headers: { "retry-after": "90" } })) as unknown as typeof fetch;

      const error = await sendPaymentVoucher("s1", { file: PHOTO }).catch((e: unknown) => e);

      expect(error).toBeInstanceOf(VoucherSendError);
      expect(error).toMatchObject({ reason: "rate_limited", retryAfterSeconds: 90 });
    });

    it("calls a failed connection a network refusal", async () => {
      globalThis.fetch = jest.fn().mockRejectedValue(new TypeError("Network request failed")) as unknown as typeof fetch;

      await expect(sendPaymentVoucher("s1", { file: PHOTO })).rejects.toMatchObject({ reason: "network" });
    });
  });

  describe("listPaymentVouchers", () => {
    it("reads the child's vouchers", async () => {
      const fetchMock = jest.fn().mockResolvedValue(respond({ status: 200, body: { vouchers: [PENDING] } }));
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      await expect(listPaymentVouchers("s1")).resolves.toEqual([PENDING]);
      expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/api\/v1\/family\/students\/s1\/payment-vouchers$/);
    });

    it("reads a null list as empty", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(respond({ status: 200, body: { vouchers: null } })) as unknown as typeof fetch;

      await expect(listPaymentVouchers("s1")).resolves.toEqual([]);
    });

    it("throws the status on a refusal, so a 404 can hide the feature", async () => {
      globalThis.fetch = jest.fn().mockResolvedValue(respond({ status: 404 })) as unknown as typeof fetch;

      await expect(listPaymentVouchers("s1")).rejects.toMatchObject({ status: 404 });
    });
  });
});
