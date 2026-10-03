import { afterEach, describe, expect, it, vi } from "vitest";

import { cancelAbsence, createAbsence, getDay, listAbsences, listUncovered, updateAbsence } from "./api";

const BASE = "http://gw/";
const CYCLE = "c1000000-0000-4000-8000-000000000001";
const TEACHER = "a1000000-0000-4000-8000-000000000009";
const ABSENCE = "f2000000-0000-4000-8000-000000000001";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stub(body: unknown, status = 200): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation(() =>
    Promise.resolve(new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })),
  );
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

const call = (m: ReturnType<typeof vi.fn>, i = 0): [string, RequestInit] => m.mock.calls[i] as [string, RequestInit];

const absence = {
  id: ABSENCE, cycleId: CYCLE, teacherId: TEACHER, startsOn: "2026-10-05", endsOn: "2026-10-07",
  allDay: false, startsAt: "08:00", endsAt: "10:00", cancelled: false, kind: "sick", note: "gripe",
};

describe("teacher absences api (T19)", () => {
  it("registers an absence and reads how many classes it leaves uncovered", async () => {
    const fetchMock = stub({ absence, uncoveredCount: 4 }, 201);
    const got = await createAbsence(BASE, {
      teacherId: TEACHER, kind: "sick", startsOn: "2026-10-05", endsOn: "2026-10-07", allDay: false, startsAt: "08:00", endsAt: "10:00", note: "gripe",
    });
    const [url, init] = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/schedule/absences");
    expect(init.method).toBe("POST");
    expect(JSON.parse(init.body as string)).toMatchObject({ teacherId: TEACHER, allDay: false, startsAt: "08:00" });
    expect(got.uncoveredCount).toBe(4);
    expect(got.absence.kind).toBe("sick");
  });

  it("edits with PUT and cancels with POST …/cancel — never DELETE", async () => {
    const fetchMock = stub({ absence, uncoveredCount: 1 });
    await updateAbsence(BASE, ABSENCE, { teacherId: TEACHER, kind: "other", startsOn: "2026-10-05", endsOn: "2026-10-05", allDay: true });
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/schedule/absences/${ABSENCE}`);
    expect(call(fetchMock)[1].method).toBe("PUT");
    await cancelAbsence(BASE, ABSENCE);
    expect(call(fetchMock, 1)[0]).toBe(`http://gw/api/v1/schedule/absences/${ABSENCE}/cancel`);
    expect(call(fetchMock, 1)[1].method).toBe("POST");
  });

  it("lists absences of a range; the private fields are optional (a reader gets none)", async () => {
    const reader = { ...absence, kind: undefined, note: undefined };
    const fetchMock = stub({ absences: [reader] });
    const got = await listAbsences(BASE, { cycleId: CYCLE, from: "2026-10-05", to: "2026-10-09", teacherId: TEACHER });
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/schedule/absences?cycleId=${CYCLE}&from=2026-10-05&to=2026-10-09&teacherId=${TEACHER}`);
    expect(got[0]?.kind).toBeUndefined();
    expect(got[0]?.startsAt).toBe("08:00");
  });

  it("lists the classes to cover with their date", async () => {
    const cls = {
      blockId: "b", groupId: "g", subjectId: "s", teacherId: TEACHER, startsAt: "08:00", endsAt: "08:50", status: "uncovered",
      baseTeacherId: TEACHER, baseStartsAt: "08:00", baseEndsAt: "08:50", overrideId: null, reason: null, absence,
    };
    const fetchMock = stub({ classes: [{ date: "2026-10-05", class: cls }] });
    const got = await listUncovered(BASE, CYCLE, "2026-10-05", "2026-10-09");
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/schedule/uncovered?cycleId=${CYCLE}&from=2026-10-05&to=2026-10-09`);
    expect(got[0]?.date).toBe("2026-10-05");
    expect(got[0]?.class.status).toBe("uncovered");
    expect(got[0]?.class.absence?.allDay).toBe(false);
  });

  it("a day carries its non-working label and uncovered classes keep their status", async () => {
    stub({ date: "2026-09-16", weekday: 3, nonWorking: "Independencia", classes: [{ blockId: "b", status: "uncovered", absence: null }] });
    const got = await getDay(BASE, CYCLE, "2026-09-16");
    expect(got.nonWorking).toBe("Independencia");
    expect(got.classes[0]?.status).toBe("uncovered");
    expect(got.classes[0]?.absence).toBeNull();
  });
});
