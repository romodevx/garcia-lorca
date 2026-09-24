import { afterEach, describe, expect, it, vi } from "vitest";
import {
  deletePhoto,
  getMedical,
  getPhotoBlob,
  removeContact,
  StudentsError,
  type StudentProfile,
  updateContact,
  updateMedical,
  updateStudent,
  uploadPhoto,
} from "./api";

const BASE = "http://gw/";

afterEach(() => vi.restoreAllMocks());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

function stubFetch(res: Response) {
  return vi.spyOn(globalThis, "fetch").mockResolvedValue(res);
}

function callOf(mock: ReturnType<typeof stubFetch>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const profile: StudentProfile = {
  birthDate: "2019-03-12",
  sex: "F",
  email: "ada@correo.com",
  phones: [{ kind: "celular", number: "5512345678" }],
  address: {
    street: "Juárez",
    extNumber: "12",
    intNumber: "",
    neighborhood: "Centro",
    postalCode: "01000",
    municipality: "Cuauhtémoc",
    state: "Ciudad de México",
  },
};

describe("updateStudent", () => {
  it("PUTs names, CURP and profile to the pupil's own path", async () => {
    const mock = stubFetch(jsonResponse({ id: "st1", profile }));
    const out = await updateStudent(BASE, "st1", {
      firstName: "Ada",
      lastName: "Lovelace",
      curp: "AAAA000000AAAAAA01",
      profile,
    });
    const [url, init] = callOf(mock);
    expect(url).toBe("http://gw/api/v1/students/st1");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({
      firstName: "Ada",
      lastName: "Lovelace",
      curp: "AAAA000000AAAAAA01",
      profile,
    });
    expect(out.profile).toEqual(profile);
  });

  it("throws a StudentsError carrying the server's reason", async () => {
    stubFetch(new Response("a phone has ten digits", { status: 400 }));
    const err = await updateStudent(BASE, "st1", {
      firstName: "Ada",
      lastName: "L",
      curp: "X",
      profile,
    }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(StudentsError);
    expect((err as StudentsError).isInvalid).toBe(true);
    expect((err as StudentsError).detail).toBe("a phone has ten digits");
  });

  it("escapes the id in the path", async () => {
    const mock = stubFetch(jsonResponse({ id: "a/b" }));
    await updateStudent(BASE, "a/b", { firstName: "", lastName: "", curp: "", profile });
    expect(callOf(mock)[0]).toBe("http://gw/api/v1/students/a%2Fb");
  });
});

describe("medical", () => {
  it("reads the record", async () => {
    const mock = stubFetch(jsonResponse({ bloodType: "O+", allergies: "nuez", conditions: "" }));
    expect(await getMedical(BASE, "st1")).toEqual({ bloodType: "O+", allergies: "nuez", conditions: "" });
    expect(callOf(mock)[0]).toBe("http://gw/api/v1/students/st1/medical");
  });

  it("writes the record with PUT", async () => {
    const record = { bloodType: "A-", allergies: "", conditions: "asma" };
    const mock = stubFetch(jsonResponse(record));
    await updateMedical(BASE, "st1", record);
    const [url, init] = callOf(mock);
    expect(url).toBe("http://gw/api/v1/students/st1/medical");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual(record);
  });
});

describe("photo", () => {
  it("returns null when the pupil has no photo", async () => {
    stubFetch(new Response("", { status: 404 }));
    expect(await getPhotoBlob(BASE, "st1")).toBeNull();
  });

  it("returns the image as a blob", async () => {
    stubFetch(new Response(new Uint8Array([0xff, 0xd8]), { status: 200, headers: { "Content-Type": "image/jpeg" } }));
    const blob = await getPhotoBlob(BASE, "st1");
    expect(blob?.size).toBe(2);
  });

  it("uploads the raw bytes with PUT", async () => {
    const mock = stubFetch(jsonResponse({ contentType: "image/jpeg", sizeBytes: 2 }));
    const image = new Blob([new Uint8Array([0xff, 0xd8])], { type: "image/jpeg" });
    await uploadPhoto(BASE, "st1", image);
    const [url, init] = callOf(mock);
    expect(url).toBe("http://gw/api/v1/students/st1/photo");
    expect(init.method).toBe("PUT");
    expect(init.body).toBe(image);
    expect((init.headers as Record<string, string>)["Content-Type"]).toBe("image/jpeg");
  });

  it("surfaces a photo that is too large", async () => {
    stubFetch(new Response("the photo is larger than 2 MB", { status: 413 }));
    const err = await uploadPhoto(BASE, "st1", new Blob(["x"])).catch((e: unknown) => e);
    expect((err as StudentsError).status).toBe(413);
  });

  it("deletes with DELETE", async () => {
    const mock = stubFetch(new Response(null, { status: 204 }));
    await deletePhoto(BASE, "st1");
    expect(callOf(mock)[1].method).toBe("DELETE");
  });
});

describe("contacts edit", () => {
  it("PUTs the contact under its pupil", async () => {
    const mock = stubFetch(jsonResponse({ id: "c1", isEmergency: true }));
    await updateContact(BASE, "st1", "c1", {
      relationship: "mother",
      fullName: "Marisol Ríos",
      phone: "5512345678",
      email: "",
      isEmergency: true,
    });
    const [url, init] = callOf(mock);
    expect(url).toBe("http://gw/api/v1/students/st1/contacts/c1");
    expect(init.method).toBe("PUT");
    expect(JSON.parse(init.body as string)).toEqual({
      relationship: "mother",
      fullName: "Marisol Ríos",
      phone: "5512345678",
      email: "",
      isEmergency: true,
    });
  });

  it("removes with DELETE and reports a 409 for a contact with app access", async () => {
    const mock = stubFetch(new Response("revoke access first", { status: 409 }));
    const err = await removeContact(BASE, "st1", "c1").catch((e: unknown) => e);
    expect(callOf(mock)[1].method).toBe("DELETE");
    expect((err as StudentsError).isConflict).toBe(true);
  });
});
