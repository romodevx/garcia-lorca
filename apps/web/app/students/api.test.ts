import { afterEach, describe, expect, it, vi } from "vitest";
import {
  enroll,
  getStudent,
  listEnrollments,
  listStudents,
  listStudentsWithRooms,
  transfer,
  updateStudent,
  withdraw,
  type StudentProfile,
} from "./api";

const BASE = "http://gw";

afterEach(() => vi.restoreAllMocks());

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** Returns the [url, init] of the single fetch call the helper made. */
function lastCall(mock: ReturnType<typeof vi.fn>): [string, RequestInit] {
  const call = mock.mock.calls[0] as [string, RequestInit];
  return [call[0], call[1]];
}

const student = {
  id: "st1",
  curp: "AAAA000000AAAAAA01",
  firstName: "Ada",
  lastName: "Lovelace",
  level: "Primaria",
  levelId: "",
  gradeGroup: "4° A",
  guardianName: "Marisol Ríos",
  admissionStatus: "accepted",
};

const enrollment = {
  id: "en1",
  studentId: "st1",
  cycleId: "c1",
  groupId: "grp1",
  termId: "",
  status: "active",
  enrolledAt: "2026-08-06T00:00:00Z",
};

describe("listStudents", () => {
  it("GETs with credentials and unwraps the {students} envelope", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ students: [student] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listStudents(BASE);

    expect(res).toEqual([student]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/students");
    expect(init.credentials).toBe("include");
    expect(init.method ?? "GET").toBe("GET");
  });

  it("filters by the status query param when given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ students: [student] }));
    vi.stubGlobal("fetch", fetchMock);

    await listStudents(BASE, "accepted");
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/students?status=accepted");
  });

  it("returns [] when the envelope key is absent", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({})));
    await expect(listStudents(BASE)).resolves.toEqual([]);
  });

  it("never asks for the classroom labels: they cost the gateway five catalog reads", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ students: [student] }));
    vi.stubGlobal("fetch", fetchMock);
    await listStudents(BASE);
    expect(lastCall(fetchMock)[0]).not.toContain("include");
  });

  it("throws on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 403 })));
    await expect(listStudents(BASE)).rejects.toThrow();
  });
});

describe("listStudentsWithRooms (CC-211)", () => {
  it("asks the gateway for each pupil's groupLabel and keeps it", async () => {
    const labelled = { ...student, groupLabel: "3° PRIMARIA · B" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ students: [labelled] }));
    vi.stubGlobal("fetch", fetchMock);

    await expect(listStudentsWithRooms(BASE)).resolves.toEqual([labelled]);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/students?include=groupLabel");
  });
});

describe("getStudent", () => {
  it("GETs /students/{id} with credentials and returns the student", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(student));
    vi.stubGlobal("fetch", fetchMock);

    const res = await getStudent(BASE, "st1");

    expect(res).toEqual(student);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/students/st1");
    expect(init.credentials).toBe("include");
  });
});

describe("enroll", () => {
  it("POSTs {studentId, groupId} and NEVER sends cycleId", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(enrollment, 201));
    vi.stubGlobal("fetch", fetchMock);

    const res = await enroll(BASE, { studentId: "st1", groupId: "grp1" });

    expect(res).toEqual(enrollment);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/enrollments");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    const body = JSON.parse(init.body as string);
    expect(body).toEqual({ studentId: "st1", groupId: "grp1" });
    expect(body).not.toHaveProperty("cycleId");
  });

  it("includes termId only when provided, still never cycleId", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(enrollment, 201));
    vi.stubGlobal("fetch", fetchMock);

    await enroll(BASE, { studentId: "st1", groupId: "grp1", termId: "t1" });
    const body = JSON.parse(lastCall(fetchMock)[1].body as string);
    expect(body).toEqual({ studentId: "st1", groupId: "grp1", termId: "t1" });
    expect(body).not.toHaveProperty("cycleId");
  });

  it("throws on a non-ok response", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 409 })));
    await expect(enroll(BASE, { studentId: "st1", groupId: "grp1" })).rejects.toThrow();
  });
});

describe("listEnrollments", () => {
  it("GETs with credentials and unwraps the {enrollments} envelope", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ enrollments: [enrollment] }));
    vi.stubGlobal("fetch", fetchMock);

    const res = await listEnrollments(BASE, { groupId: "grp1" });

    expect(res).toEqual([enrollment]);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/enrollments?groupId=grp1");
    expect(init.credentials).toBe("include");
  });

  it("applies studentId/cycleId/groupId filters and drops empty ones", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ enrollments: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listEnrollments(BASE, { studentId: "st1", cycleId: "c1", groupId: "grp1" });
    expect(lastCall(fetchMock)[0]).toBe(
      "http://gw/api/v1/enrollments?studentId=st1&cycleId=c1&groupId=grp1",
    );
  });

  it("sends no query string when no filters are given", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ enrollments: [] }));
    vi.stubGlobal("fetch", fetchMock);

    await listEnrollments(BASE);
    expect(lastCall(fetchMock)[0]).toBe("http://gw/api/v1/enrollments");
  });
});

describe("transfer", () => {
  it("POSTs {enrollmentId, toGroupId, reason} with credentials", async () => {
    const moved = { ...enrollment, groupId: "grp2", status: "active" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(moved));
    vi.stubGlobal("fetch", fetchMock);

    const res = await transfer(BASE, { enrollmentId: "en1", toGroupId: "grp2", reason: "cambio de turno" });

    expect(res).toEqual(moved);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/enrollments/transfer");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({
      enrollmentId: "en1",
      toGroupId: "grp2",
      reason: "cambio de turno",
    });
  });
});

describe("withdraw", () => {
  it("POSTs {enrollmentId} with credentials", async () => {
    const gone = { ...enrollment, status: "withdrawn" };
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(gone));
    vi.stubGlobal("fetch", fetchMock);

    const res = await withdraw(BASE, "en1");

    expect(res).toEqual(gone);
    const [url, init] = lastCall(fetchMock);
    expect(url).toBe("http://gw/api/v1/enrollments/withdraw");
    expect(init.method).toBe("POST");
    expect(init.credentials).toBe("include");
    expect(JSON.parse(init.body as string)).toEqual({ enrollmentId: "en1" });
  });
});

// T14 gate: a withdrawn pupil's refusal is a 409 like a duplicate CURP; the
// gateway's X-Error-Code tells them apart.
const PROFILE: StudentProfile = {
  birthDate: "", sex: "", email: "", phones: [],
  address: { street: "", extNumber: "", intNumber: "", neighborhood: "", postalCode: "", municipality: "", state: "" },
};

describe("StudentsError · baja", () => {
  it("reads STUDENT_WITHDRAWN from X-Error-Code", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("ese alumno está dado de baja: su ficha ya no se corrige", {
      status: 409, headers: { "X-Error-Code": "STUDENT_WITHDRAWN" },
    })));

    const err = await updateStudent(BASE, "st1", { firstName: "A", lastName: "B", curp: "AAAA000000AAAAAA01", profile: PROFILE }).catch((e: unknown) => e);
    expect(err).toMatchObject({ status: 409, isWithdrawn: true });
  });

  it("a plain 409 is not a baja", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("curp taken", { status: 409 })));

    const err = await updateStudent(BASE, "st1", { firstName: "A", lastName: "B", curp: "AAAA000000AAAAAA01", profile: PROFILE }).catch((e: unknown) => e);
    expect(err).toMatchObject({ status: 409, isWithdrawn: false });
  });
});
