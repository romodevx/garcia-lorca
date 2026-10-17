import { afterEach, describe, expect, it, vi } from "vitest";

import { deleteNonWorkingPeriod, deleteSchoolEvent } from "./config/api";
import { deleteBlock } from "./schedule/api";
import { removeContact, voidNote } from "./students/api";

/**
 * Gate B1 on #142: a deferred delete committed on `pagehide` went out as a
 * plain fetch, which the unload cancels — the delete was lost. Every API call
 * an undo toast defers takes `keepalive`, and the session still rides along
 * (credentials: "include").
 */
const BASE = "http://gw";
const ID = "70000000-0000-4000-8000-000000000001";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stubFetch(body: unknown = {}, status = 200): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation(() =>
    Promise.resolve(status === 204 ? new Response(null, { status }) : new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const initOf = (fetchMock: ReturnType<typeof vi.fn>): RequestInit => (fetchMock.mock.calls[0] as [string, RequestInit])[1];

describe("deferred deletes survive the page going away", () => {
  it.each([
    ["deleteBlock", () => deleteBlock(BASE, ID, undefined, { keepalive: true })],
    ["deleteNonWorkingPeriod", () => deleteNonWorkingPeriod(BASE, ID, { keepalive: true })],
    ["deleteSchoolEvent", () => deleteSchoolEvent(BASE, ID, { keepalive: true })],
    ["removeContact", () => removeContact(BASE, "st-1", ID, { keepalive: true })],
  ])("%s sends keepalive with the session cookie", async (_name, call) => {
    const fetchMock = stubFetch(undefined, 204);

    await call();

    expect(initOf(fetchMock).keepalive).toBe(true);
    expect(initOf(fetchMock).credentials).toBe("include");
  });

  it("voidNote sends keepalive with the session cookie", async () => {
    const fetchMock = stubFetch({ id: ID });

    await voidNote(BASE, "st-1", ID, { keepalive: true });

    expect(initOf(fetchMock).keepalive).toBe(true);
    expect(initOf(fetchMock).credentials).toBe("include");
  });

  it("a delete inside the page does not ask for keepalive", async () => {
    const fetchMock = stubFetch(undefined, 204);

    await deleteSchoolEvent(BASE, ID);

    expect(initOf(fetchMock).keepalive).toBeUndefined();
  });
});
