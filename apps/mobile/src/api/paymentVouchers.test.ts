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
 * Records what is appended, as React Native's FormData receives it: Jest runs
 * on Node, whose FormData would flatten the { uri, name, type } part to a string.
 */
class RecordingFormData {
  readonly appended: [string, unknown][] = [];
  append(name: string, value: unknown): void {
    this.appended.push([name, value]);
  }
}

function partsOf(init: RequestInit): Record<string, unknown> {
  return Object.fromEntries((init.body as unknown as RecordingFormData).appended);
}

const scope = globalThis as unknown as { FormData: unknown };
const NativeFormData = scope.FormData;
beforeAll(() => {
  scope.FormData = RecordingFormData;
});
afterAll(() => {
  scope.FormData = NativeFormData;
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
      expect(partsOf(init)).toEqual({ file: PHOTO, amountCents: "250000", paidOn: "2026-10-20" });
      expect(result).toEqual({ voucher: PENDING, replayed: false });
    });

    it("leaves out what the family did not say", async () => {
      const fetchMock = jest.fn().mockResolvedValue(respond({ status: 201, body: { voucher: PENDING, replayed: false } }));
      globalThis.fetch = fetchMock as unknown as typeof fetch;

      await sendPaymentVoucher("s1", { file: PHOTO });

      expect(Object.keys(partsOf(fetchMock.mock.calls[0][1] as RequestInit))).toEqual(["file"]);
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
