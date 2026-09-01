import { afterEach, describe, expect, it, vi } from "vitest";
import { announce } from "./api";

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("announce", () => {
  it("sends the scope, the audience and a key that makes a retry harmless", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ created: 28, audience: 28 }), { status: 201, headers: { "Content-Type": "application/json" } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    const res = await announce(BASE, {
      scope: "group", targetId: "g1", cycleId: "c1",
      title: "Junta", body: "Jueves 8:00", idempotencyKey: "k1",
    });

    expect(res).toEqual({ created: 28, audience: 28 });
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/comms/announcements");
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body.scope).toBe("group");
    expect(body.idempotencyKey).toBe("k1");
  });

  it("leaves out the target for a whole-school announcement", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ created: 400, audience: 400 }), { status: 201, headers: { "Content-Type": "application/json" } }),
    );
    vi.stubGlobal("fetch", fetchMock);

    await announce(BASE, { scope: "cycle", cycleId: "c1", title: "Aviso", body: "Mañana", idempotencyKey: "k2" });

    const body = JSON.parse((fetchMock.mock.calls[0] as [string, RequestInit])[1].body as string) as Record<string, unknown>;
    expect("targetId" in body).toBe(false);
  });

  // "Nadie coincide con ese destino" is the sentence the office needs to read.
  // A generic failure would leave them believing the message went out.
  it("surfaces the gateway's own words when nobody matched", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("nadie coincide con ese destino", { status: 400 })));

    await expect(
      announce(BASE, { scope: "group", targetId: "g9", cycleId: "c1", title: "Junta", body: "Jueves", idempotencyKey: "k3" }),
    ).rejects.toThrow("nadie coincide con ese destino");
  });
});
