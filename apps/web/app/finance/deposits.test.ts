import { afterEach, describe, expect, it, vi } from "vitest";
import { listDeposits, previewDeposit, registerDeposit, voidDeposit } from "./api";

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function lastCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const deposit = {
  id: "dep-1",
  studentId: "stu-1",
  amountCents: 670000,
  method: "cash",
  reference: "",
  paidAt: "2026-10-10T00:00:00Z",
  calendarVersion: "abc123",
};

describe("registerDeposit", () => {
  it("sends the amount as integer cents and the day the money changed hands", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false }, 201),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await registerDeposit(BASE, {
      studentId: "stu-1",
      amountCents: 670000,
      method: "cash",
      reference: "",
      paidAt: "2026-10-10",
      idempotencyKey: "k1",
    });

    expect(res.deposit.amountCents).toBe(670000);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/deposits");
    expect(init.method).toBe("POST");
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body.paidAt).toBe("2026-10-10");
    expect(body.idempotencyKey).toBe("k1");
    expect(Number.isInteger(body.amountCents)).toBe(true);
  });

  it("omits an empty paidAt so the server stamps today, as it always did", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ deposit, allocations: [], surcharges: [], unallocatedCents: 0, replayed: false }, 201),
    );
    vi.stubGlobal("fetch", fetchMock);

    await registerDeposit(BASE, { studentId: "stu-1", amountCents: 100000, method: "cash", reference: "" });

    const body = JSON.parse(lastCall(fetchMock)[1].body as string) as Record<string, unknown>;
    expect("paidAt" in body).toBe(false);
  });

  it("surfaces the gateway's own words when the money is refused", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("la fecha del pago no puede estar en el futuro", { status: 400 })));

    await expect(
      registerDeposit(BASE, { studentId: "stu-1", amountCents: 100000, method: "cash", reference: "", paidAt: "2027-01-01" }),
    ).rejects.toThrow("la fecha del pago no puede estar en el futuro");
  });
});

describe("previewDeposit", () => {
  it("asks what the money would cover without a method or a key — nothing is taken", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      jsonResponse({ allocations: [{ id: "p1", chargeId: "ch1", amountCents: 320000, method: "cash", reference: "", paidAt: "" }], surcharges: [], unallocatedCents: 350000 }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await previewDeposit(BASE, { studentId: "stu-1", amountCents: 670000, paidAt: "2026-10-10" });

    expect(res.allocations).toHaveLength(1);
    expect(res.unallocatedCents).toBe(350000);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/deposits/preview");
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect("idempotencyKey" in body).toBe(false);
  });
});

describe("listDeposits", () => {
  it("unwraps the envelope", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ deposits: [deposit] })));

    const res = await listDeposits(BASE, "stu-1");

    expect(res).toHaveLength(1);
    expect(res[0]?.id).toBe("dep-1");
  });
});

describe("voidDeposit", () => {
  it("sends the reason, which the counter cannot skip", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
    vi.stubGlobal("fetch", fetchMock);

    await voidDeposit(BASE, "dep-1", "importe capturado mal");

    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/finance/deposits/dep-1/void");
    expect(JSON.parse(init.body as string)).toEqual({ reason: "importe capturado mal" });
  });
});
