import { afterEach, describe, expect, it, vi } from "vitest";

import { StudentsError } from "../students/api";
import {
  FamiliesError,
  addFamilyContact,
  addStudentsToFamily,
  createFamily,
  dismissFamilySuggestion,
  dissolveFamily,
  familyRefusal,
  getFamily,
  listFamilies,
  listFamilyContacts,
  listFamilySuggestions,
  removeStudentFromFamily,
  renameFamily,
  revokeFamilyAccess,
} from "./api";

const BASE = "http://gw/";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function refusal(status: number, text: string, code?: string): Response {
  return new Response(text, { status, headers: code === undefined ? {} : { "X-Error-Code": code } });
}

function stubFetch(response: Response): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockResolvedValue(response);
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function call(fetchMock: ReturnType<typeof vi.fn>): { url: string; init: RequestInit; body: unknown } {
  const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
  return { url, init, body: typeof init.body === "string" ? JSON.parse(init.body) : undefined };
}

const family = { id: "f1", name: "GARCÍA LÓPEZ", studentIds: ["s1", "s2"], accountCount: 1, createdAt: "2026-10-20T15:04:05Z" };

describe("families api — reads", () => {
  it("lists the school's families from the envelope, with the session", async () => {
    const fetchMock = stubFetch(jsonResponse({ families: [family] }));
    const got = await listFamilies(BASE);
    expect(got).toEqual([family]);
    const { url, init } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families");
    expect(init.credentials).toBe("include");
  });

  it("lists the suggestions", async () => {
    const suggestion = { signature: "abc", proposedName: "PÉREZ RUIZ", studentIds: ["s3", "s4"], reasons: ["same_surnames"], confidence: "medium" };
    const fetchMock = stubFetch(jsonResponse({ suggestions: [suggestion] }));
    expect(await listFamilySuggestions(BASE)).toEqual([suggestion]);
    expect(call(fetchMock).url).toBe("http://gw/api/v1/families/suggestions");
  });

  it("reads one family with its members, escaping the id", async () => {
    const fetchMock = stubFetch(jsonResponse({ ...family, members: [] }));
    const got = await getFamily(BASE, "f/1");
    expect(got.members).toEqual([]);
    expect(call(fetchMock).url).toBe("http://gw/api/v1/families/f%2F1");
  });

  it("reads the family's adults", async () => {
    const person = { fullName: "ANA LÓPEZ", relationship: "mother", phone: "", email: "ana@example.com", hasApp: false, contacts: [{ studentId: "s1", contactId: "c1" }] };
    const fetchMock = stubFetch(jsonResponse({ people: [person] }));
    expect(await listFamilyContacts(BASE, "f1")).toEqual([person]);
    expect(call(fetchMock).url).toBe("http://gw/api/v1/families/f1/contacts");
  });

  it("throws a FamiliesError on a failed read", async () => {
    stubFetch(refusal(404, "not found"));
    await expect(listFamilies(BASE)).rejects.toBeInstanceOf(FamiliesError);
  });
});

describe("families api — writes", () => {
  it("creates a family with a name and its pupils", async () => {
    const fetchMock = stubFetch(jsonResponse(family, 201));
    await createFamily(BASE, { name: "GARCÍA LÓPEZ", studentIds: ["s1", "s2"] });
    const { url, init, body } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families");
    expect(init.method).toBe("POST");
    expect(body).toEqual({ name: "GARCÍA LÓPEZ", studentIds: ["s1", "s2"] });
  });

  it("leaves an empty name out so the server takes the surnames", async () => {
    const fetchMock = stubFetch(jsonResponse(family, 201));
    await createFamily(BASE, { name: "  ", studentIds: ["s1"] });
    expect(call(fetchMock).body).toEqual({ studentIds: ["s1"] });
  });

  it("renames with PATCH", async () => {
    const fetchMock = stubFetch(jsonResponse({ ...family, name: "LOS GARCÍA" }));
    const got = await renameFamily(BASE, "f1", "LOS GARCÍA");
    expect(got.name).toBe("LOS GARCÍA");
    const { url, init, body } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families/f1");
    expect(init.method).toBe("PATCH");
    expect(body).toEqual({ name: "LOS GARCÍA" });
  });

  it("dissolves with DELETE", async () => {
    const fetchMock = stubFetch(new Response(null, { status: 204 }));
    await dissolveFamily(BASE, "f1");
    const { url, init } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families/f1");
    expect(init.method).toBe("DELETE");
  });

  it("dismisses a suggestion by its pupils", async () => {
    const fetchMock = stubFetch(new Response(null, { status: 204 }));
    await dismissFamilySuggestion(BASE, ["s3", "s4"]);
    const { url, body } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families/suggestions/dismiss");
    expect(body).toEqual({ studentIds: ["s3", "s4"] });
  });

  it("adds pupils", async () => {
    const fetchMock = stubFetch(jsonResponse(family));
    await addStudentsToFamily(BASE, "f1", ["s5"]);
    const { url, init, body } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families/f1/students");
    expect(init.method).toBe("POST");
    expect(body).toEqual({ studentIds: ["s5"] });
  });

  it("removes a pupil and returns the family that is left", async () => {
    const fetchMock = stubFetch(jsonResponse({ ...family, studentIds: ["s2"] }));
    const got = await removeStudentFromFamily(BASE, "f1", "s1");
    expect(got?.studentIds).toEqual(["s2"]);
    const { url, init } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families/f1/students/s1");
    expect(init.method).toBe("DELETE");
  });

  it("removing the last pupil answers null: the family is gone", async () => {
    stubFetch(new Response(null, { status: 204 }));
    expect(await removeStudentFromFamily(BASE, "f1", "s1")).toBeNull();
  });

  it("records one adult, leaving out what was not captured", async () => {
    const fetchMock = stubFetch(jsonResponse({ contacts: [] }, 201));
    await addFamilyContact(BASE, "f1", { relationship: "mother", fullName: "ANA LÓPEZ", phone: "", email: "ana@example.com", isEmergency: true });
    const { url, body } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families/f1/contacts");
    expect(body).toEqual({ relationship: "mother", fullName: "ANA LÓPEZ", email: "ana@example.com", isEmergency: true });
  });

  it("revokes an account from the whole family", async () => {
    const fetchMock = stubFetch(jsonResponse({ contacts: 2 }));
    expect(await revokeFamilyAccess(BASE, "f1", "u1")).toBe(2);
    const { url, body } = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/families/f1/access/revoke");
    expect(body).toEqual({ guardianUserId: "u1" });
  });
});

describe("families api — refusals", () => {
  it.each([
    ["FAMILY_ACCOUNT_LIMIT", "Esta familia ya tiene 2 cuentas de la app"],
    ["FAMILY_BUSY", "Otra persona está cambiando esta familia; intenta de nuevo"],
  ])("%s reads in Spanish and keeps its code", async (code, copy) => {
    stubFetch(refusal(409, "una familia tiene como máximo dos cuentas de la app", code));
    const err = await createFamily(BASE, { name: "", studentIds: ["s1"] }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(FamiliesError);
    expect((err as FamiliesError).code).toBe(code);
    expect((err as FamiliesError).message).toBe(copy);
  });

  it("FAMILY_MANAGED_ACCESS says where the access is managed", async () => {
    stubFetch(refusal(409, "x", "FAMILY_MANAGED_ACCESS"));
    const err = (await revokeFamilyAccess(BASE, "f1", "u1").catch((e: unknown) => e)) as FamiliesError;
    expect(err.message).toMatch(/familia/i);
  });

  it("a 409 without a code shows the server's Spanish sentence", async () => {
    stubFetch(refusal(409, "ese alumno ya pertenece a otra familia"));
    const err = (await addStudentsToFamily(BASE, "f1", ["s9"]).catch((e: unknown) => e)) as FamiliesError;
    expect(err.message).toBe("ese alumno ya pertenece a otra familia");
  });

  it("familyRefusal reads the invite's StudentsError too", () => {
    expect(familyRefusal(new StudentsError(409, "whatever", "FAMILY_ACCOUNT_LIMIT"), "fallback")).toBe("Esta familia ya tiene 2 cuentas de la app");
    expect(familyRefusal(new Error("boom"), "No se pudo.")).toBe("No se pudo.");
  });
});
