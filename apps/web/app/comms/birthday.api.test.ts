import { afterEach, describe, expect, it, vi } from "vitest";
import { getBirthdayTemplate, updateBirthdayTemplate } from "./api";

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const json = (body: unknown, status = 200): Response =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });

describe("birthday greeting (T11)", () => {
  it("reads the school's greeting", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ title: "¡Feliz cumpleaños, {nombre}!", body: "Hola {nombre}", isDefault: true, updatedAt: "" }));
    vi.stubGlobal("fetch", fetchMock);

    const got = await getBirthdayTemplate(BASE);

    expect(got).toEqual({ title: "¡Feliz cumpleaños, {nombre}!", body: "Hola {nombre}", isDefault: true, updatedAt: "" });
    expect((fetchMock.mock.calls[0] as [string])[0]).toBe("http://gw/api/v1/comms/birthday-template");
  });

  it("saves title and message with PUT", async () => {
    const fetchMock = vi.fn().mockResolvedValue(json({ title: "T", body: "B", isDefault: false, updatedAt: "2026-10-01T13:00:00Z" }));
    vi.stubGlobal("fetch", fetchMock);

    const saved = await updateBirthdayTemplate(BASE, { title: "T", body: "B" });

    expect(saved.isDefault).toBe(false);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/comms/birthday-template");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({ title: "T", body: "B" });
  });

  it("surfaces the gateway's words on a refusal", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("the message is at most 500 characters", { status: 400 })));
    await expect(updateBirthdayTemplate(BASE, { title: "T", body: "B" })).rejects.toThrow("500");
  });
});
