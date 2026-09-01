import { afterEach, describe, expect, it, vi } from "vitest";
import { addContact, listContacts } from "./api";

const BASE = "http://gw";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

const contact = {
  id: "gl-1", studentId: "st-1", guardianUserId: "", relationship: "other" as const,
  fullName: "Rosa Méndez", phone: "5551234567", email: "", hasAccount: false,
};

describe("contacts", () => {
  it("records somebody with a phone and no account", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(contact, 201));
    vi.stubGlobal("fetch", fetchMock);

    const got = await addContact(BASE, {
      studentId: "st-1", relationship: "other", fullName: "Rosa Méndez", phone: "5551234567",
    });

    expect(got.hasAccount).toBe(false);
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/contacts");
    const body = JSON.parse(init.body as string) as Record<string, unknown>;
    expect(body.fullName).toBe("Rosa Méndez");
    // An empty email is left out rather than sent blank.
    expect("email" in body).toBe(false);
  });

  it("lists the people the school can call about one pupil", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({ guardians: [contact] })));

    const got = await listContacts(BASE, "st-1");

    expect(got).toHaveLength(1);
    expect(got[0]?.fullName).toBe("Rosa Méndez");
  });

  it("surfaces the gateway's words when the contact is refused", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("a contact without an account needs a name", { status: 400 })));

    await expect(
      addContact(BASE, { studentId: "st-1", relationship: "other", fullName: "" }),
    ).rejects.toThrow("a contact without an account needs a name");
  });
});
