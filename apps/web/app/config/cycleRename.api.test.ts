import { afterEach, describe, expect, it, vi } from "vitest";

import { ConfigError, createCycle, renameCycle } from "./api";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

describe("cycles (QA CC-101 / CC-103)", () => {
  it("renameCycle PATCHes the name and returns the cycle", async () => {
    const renamed = { id: "c1", name: "2027-2028", status: "draft", startsOn: "2027-08-23", endsOn: "2028-07-07" };
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(renamed), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(renameCycle(BASE, "c1", "2027-2028")).resolves.toEqual(renamed);
    const [url, init] = fetchMock.mock.calls.at(-1) as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/config/cycles/c1");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body as string)).toEqual({ name: "2027-2028" });
  });

  it("an overlapping cycle comes back as a conflict carrying CYCLE_OVERLAP and the server's sentence", async () => {
    const sentence = 'Las fechas se traslapan con el ciclo "2026-2027" (del 24 de agosto de 2026 al 9 de julio de 2027)';
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(sentence, { status: 409, headers: { "X-Error-Code": "CYCLE_OVERLAP" } })));

    const err = await createCycle(BASE, { name: "2027", startsOn: "2027-01-10", endsOn: "2027-12-20" }).catch((e: unknown) => e);

    expect(err).toBeInstanceOf(ConfigError);
    expect((err as ConfigError).code).toBe("CYCLE_OVERLAP");
    expect((err as ConfigError).detail).toBe(sentence);
  });
});
