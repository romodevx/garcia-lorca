import { afterEach, describe, expect, it, vi } from "vitest";

import {
  changeActivityOption,
  createActivity,
  endEnrollments,
  enrollStudents,
  getActivity,
  listActivities,
  listEnrollments,
  updateActivity,
  type ActivityInput,
} from "./api";

const BASE = "http://gw/";
const ACT = "a1000000-0000-0000-0000-000000000001";
const OPT = "0f000000-0000-0000-0000-000000000001";
const STU = "51000000-0000-0000-0000-000000000001";

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

function call(fetchMock: ReturnType<typeof vi.fn>, i = 0): [string, RequestInit] {
  return fetchMock.mock.calls[i] as [string, RequestInit];
}

const input: ActivityInput = {
  name: "Natación",
  schedule: [{ weekday: 2, start: "15:00", end: "16:30" }],
  periodicity: "monthly",
  billingStartsOn: "2026-10-01",
  firstDueOn: "2026-10-10",
  lateFeeCents: 0,
  active: true,
  options: [{ daysPerWeek: 1, amountCents: 60000 }, { daysPerWeek: null, amountCents: 90000 }],
};

const wire = { ...input, id: ACT, enrolledCount: 3, options: [{ id: OPT, daysPerWeek: 1, amountCents: 60000 }] };

describe("activities api (T17)", () => {
  it("lists the activities, inactive ones when asked", async () => {
    const fetchMock = stub({ activities: [wire] });
    const got = await listActivities(BASE, true);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/activities?includeInactive=true");
    expect(got[0]?.enrolledCount).toBe(3);
  });

  it("an unknown periodicity degrades to monthly instead of breaking the row", async () => {
    stub({ activities: [{ ...wire, periodicity: "daily" }] });
    const [got] = await listActivities(BASE);
    expect(got?.periodicity).toBe("monthly");
  });

  it("a missing flat-fee daysPerWeek reads as null (the flat fee)", async () => {
    stub({ ...wire, options: [{ id: OPT, amountCents: 90000 }] });
    const got = await getActivity(BASE, ACT);
    expect(got.options[0]?.daysPerWeek).toBeNull();
  });

  it("creates and edits with the contract's body", async () => {
    const fetchMock = stub(wire, 201);
    await createActivity(BASE, input);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/activities");
    expect(call(fetchMock)[1].method).toBe("POST");
    expect(JSON.parse(String(call(fetchMock)[1].body))).toEqual(input);
    await updateActivity(BASE, ACT, input);
    expect(call(fetchMock, 1)[0]).toBe(`http://gw/api/v1/activities/${ACT}`);
    expect(call(fetchMock, 1)[1].method).toBe("PUT");
  });

  it("lists the enrolments", async () => {
    const fetchMock = stub({ enrollments: [{ id: "e1", activityId: ACT, studentId: STU, optionId: OPT, startsOn: "2026-10-02" }] });
    const got = await listEnrollments(BASE, ACT);
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/activities/${ACT}/enrollments`);
    expect(got[0]?.studentId).toBe(STU);
  });

  it("enrolls each pupil with their option and says how many charges were issued", async () => {
    const fetchMock = stub({ enrollments: [], chargesCreated: 2 });
    const got = await enrollStudents(BASE, ACT, [{ studentId: STU, optionId: OPT }]);
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/activities/${ACT}/enrollments`);
    expect(JSON.parse(String(call(fetchMock)[1].body))).toEqual({ enrollments: [{ studentId: STU, optionId: OPT }] });
    expect(got.chargesCreated).toBe(2);
  });

  it("ends enrolments by student id", async () => {
    const fetchMock = stub({ ended: 1 });
    const got = await endEnrollments(BASE, ACT, [STU]);
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/activities/${ACT}/enrollments/end`);
    expect(JSON.parse(String(call(fetchMock)[1].body))).toEqual({ studentIds: [STU] });
    expect(got).toBe(1);
  });

  it("changes one pupil's option", async () => {
    const fetchMock = stub({ id: "e1", activityId: ACT, studentId: STU, optionId: OPT, startsOn: "2026-10-02" });
    await changeActivityOption(BASE, ACT, STU, OPT);
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/activities/${ACT}/enrollments/${STU}`);
    expect(call(fetchMock)[1].method).toBe("PUT");
    expect(JSON.parse(String(call(fetchMock)[1].body))).toEqual({ optionId: OPT });
  });

  it("shows the gateway's text/plain sentence as the error", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("Ya existe una actividad con ese nombre.", { status: 409 })));
    await expect(createActivity(BASE, input)).rejects.toThrow("Ya existe una actividad con ese nombre.");
  });

  it("an id that is not one never reaches the network", async () => {
    const fetchMock = stub({});
    await expect(getActivity(BASE, "../x")).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
