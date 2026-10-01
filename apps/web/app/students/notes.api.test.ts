import { afterEach, describe, expect, it, vi } from "vitest";

import { StudentsError, addNote, editNote, listNotes, voidNote } from "./api";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

const note = {
  id: "n-1",
  studentId: "st1",
  body: "Llegó con fiebre",
  authorUserId: "u-1",
  authorName: "Gabriela Rojas",
  createdAt: "2026-09-24T16:00:00Z",
  voided: false,
};

describe("notes", () => {
  it("lists a pupil's notes", async () => {
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse({ notes: [note] }));
    expect(await listNotes(BASE, "st1")).toEqual([note]);
    expect((mock.mock.calls[0] as [string])[0]).toBe("http://gw/api/v1/students/st1/notes");
  });

  it("writes a note with only its body: the author is whoever is signed in", async () => {
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse(note, 201));
    await addNote(BASE, "st1", "Llegó con fiebre");
    const [url, init] = mock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/students/st1/notes");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toEqual({ body: "Llegó con fiebre" });
  });

  it("voids a note, and reports a late void as forbidden", async () => {
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("only its author, within 15 minutes", { status: 403 }));
    const err = await voidNote(BASE, "st1", "n-1").catch((e: unknown) => e);
    const [url, init] = mock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/students/st1/notes/n-1/void");
    expect(init.method).toBe("POST");
    expect((err as StudentsError).status).toBe(403);
  });

  it("edits a note with only its new body: the editor is whoever is signed in", async () => {
    const edited = { ...note, body: "Fiebre de 38", updatedAt: "2026-09-25T16:00:00Z", updatedByName: "Dirección" };
    const mock = vi.spyOn(globalThis, "fetch").mockResolvedValue(jsonResponse(edited));
    expect(await editNote(BASE, "st1", "n-1", "Fiebre de 38")).toEqual(edited);
    const [url, init] = mock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://gw/api/v1/students/st1/notes/n-1");
    expect(init.method).toBe("PATCH");
    expect(JSON.parse(init.body as string)).toEqual({ body: "Fiebre de 38" });
  });

  it("reports an edit of someone else's note as forbidden", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response("only its author", { status: 403 }));
    const err = await editNote(BASE, "st1", "n-1", "x").catch((e: unknown) => e);
    expect((err as StudentsError).status).toBe(403);
  });
});
