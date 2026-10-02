import { afterEach, describe, expect, it, vi } from "vitest";

import {
  createBlock,
  createOverride,
  deleteBlock,
  deleteOverride,
  getBoard,
  getDay,
  getOwnBoard,
  getOwnDay,
  listAvailableTeachers,
  listEvents,
  putSettings,
  ScheduleError,
  updateBlock,
} from "./api";

const BASE = "http://gw/";
const CYCLE = "c1000000-0000-4000-8000-000000000001";
const BLOCK = "b1000000-0000-4000-8000-000000000001";
const GROUP = "a1000000-0000-4000-8000-000000000001";
const SUBJECT = "d1000000-0000-4000-8000-000000000001";
const LEVEL = "e2000000-0000-4000-8000-000000000001";
const OVERRIDE = "f1000000-0000-4000-8000-000000000001";

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stub(body: unknown, status = 200, headers: Record<string, string> = {}): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn().mockImplementation(() => {
    if (status === 204) return Promise.resolve(new Response(null, { status }));
    const text = typeof body === "string" ? body : JSON.stringify(body);
    const type = typeof body === "string" ? "text/plain" : "application/json";
    return Promise.resolve(new Response(text, { status, headers: { "Content-Type": type, ...headers } }));
  });
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}

function call(fetchMock: ReturnType<typeof vi.fn>, i = 0): [string, RequestInit] {
  return fetchMock.mock.calls[i] as [string, RequestInit];
}

const block = {
  id: BLOCK, cycleId: CYCLE, assignmentId: "x", groupId: GROUP, subjectId: SUBJECT, teacherId: "t",
  weekday: 1, startsAt: "08:00", endsAt: "08:50",
};

const board = {
  cycle: { id: CYCLE, name: "2026-2027", startsOn: "2026-08-24", endsOn: "2027-07-09", active: true },
  cycles: [], started: true, settings: [], levels: [], grades: [], groups: [], subjects: [], teachers: [], assignments: [], blocks: [block],
};

describe("schedule api (T16)", () => {
  it("reads the active cycle's board when no cycle is named, and a named one otherwise", async () => {
    const fetchMock = stub(board);
    const got = await getBoard(BASE);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/schedule");
    expect(got.blocks[0]?.startsAt).toBe("08:00");
    await getBoard(BASE, CYCLE);
    expect(call(fetchMock, 1)[0]).toBe(`http://gw/api/v1/schedule?cycleId=${CYCLE}`);
  });

  it("missing arrays on the wire become empty lists", async () => {
    stub({ cycle: board.cycle, started: false });
    const got = await getBoard(BASE);
    expect(got.blocks).toEqual([]);
    expect(got.assignments).toEqual([]);
    expect(got.cycles).toEqual([]);
  });

  it("reads the teacher's own board and day", async () => {
    const fetchMock = stub(board);
    await getOwnBoard(BASE);
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/teaching/schedule");
    stub({ date: "2026-10-05", weekday: 1, classes: [] });
    const day = await getOwnDay(BASE, "2026-10-05");
    expect(day.classes).toEqual([]);
  });

  it("reads one date of a cycle", async () => {
    const fetchMock = stub({ date: "2026-10-05", weekday: 1, classes: [{ ...block, blockId: BLOCK, status: "substitute", baseTeacherId: "t0", baseStartsAt: "08:00", baseEndsAt: "08:50", overrideId: OVERRIDE, reason: "Gripe" }] });
    const day = await getDay(BASE, CYCLE, "2026-10-05");
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/schedule/day?cycleId=${CYCLE}&date=2026-10-05`);
    expect(day.classes[0]?.status).toBe("substitute");
  });

  it("an unknown class status degrades to regular", async () => {
    stub({ date: "2026-10-05", weekday: 1, classes: [{ ...block, blockId: BLOCK, status: "teleported", overrideId: null, reason: null }] });
    const day = await getDay(BASE, CYCLE, "2026-10-05");
    expect(day.classes[0]?.status).toBe("regular");
  });

  it("creates a class with HH:MM times and the reason only when given", async () => {
    const fetchMock = stub({ block }, 201);
    const got = await createBlock(BASE, { assignmentId: "a", weekday: 1, startsAt: "08:00", endsAt: "08:50" });
    const [url, init] = call(fetchMock);
    expect(url).toBe("http://gw/api/v1/schedule/blocks");
    expect(init.method).toBe("POST");
    expect(JSON.parse(String(init.body))).toEqual({ assignmentId: "a", weekday: 1, startsAt: "08:00", endsAt: "08:50" });
    expect(got.id).toBe(BLOCK);
    await createBlock(BASE, { assignmentId: "a", weekday: 2, startsAt: "09:00", endsAt: "09:50", reason: "Ajuste" });
    expect(JSON.parse(String(call(fetchMock, 1)[1].body)).reason).toBe("Ajuste");
  });

  it("moves a class with PUT and removes it with the reason in the query", async () => {
    const fetchMock = stub({ block });
    await updateBlock(BASE, BLOCK, { assignmentId: "a", weekday: 3, startsAt: "10:00", endsAt: "10:50" });
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/schedule/blocks/${BLOCK}`);
    expect(call(fetchMock)[1].method).toBe("PUT");
    stub(null, 204);
    const del = stub(null, 204);
    await deleteBlock(BASE, BLOCK, "Cambio de plan");
    expect(call(del)[0]).toBe(`http://gw/api/v1/schedule/blocks/${BLOCK}?reason=Cambio+de+plan`);
    expect(call(del)[1].method).toBe("DELETE");
  });

  it("records and undoes a change for one date", async () => {
    const fetchMock = stub({ override: { id: OVERRIDE } }, 201);
    await createOverride(BASE, { blockId: BLOCK, date: "2026-10-05", kind: "substitute", teacherId: "t2", reason: "Gripe" });
    expect(call(fetchMock)[0]).toBe("http://gw/api/v1/schedule/overrides");
    expect(JSON.parse(String(call(fetchMock)[1].body))).toEqual({ blockId: BLOCK, date: "2026-10-05", kind: "substitute", teacherId: "t2", reason: "Gripe" });
    const del = stub(null, 204);
    await deleteOverride(BASE, OVERRIDE);
    expect(call(del)[0]).toBe(`http://gw/api/v1/schedule/overrides/${OVERRIDE}`);
    expect(call(del)[1].method).toBe("DELETE");
  });

  it("asks who is free in a slot, leaving the class itself out", async () => {
    const fetchMock = stub({ teachers: [{ teacherId: "t2", firstName: "Luis", lastName: "Pérez", match: "subject" }, { teacherId: "t3", firstName: "Eva", lastName: "Gil", match: "nope" }] });
    const got = await listAvailableTeachers(BASE, {
      cycleId: CYCLE, groupId: GROUP, subjectId: SUBJECT, date: "2026-10-05", startsAt: "08:00", endsAt: "08:50", excludeBlockId: BLOCK,
    });
    expect(call(fetchMock)[0]).toBe(
      `http://gw/api/v1/schedule/available-teachers?cycleId=${CYCLE}&groupId=${GROUP}&subjectId=${SUBJECT}&date=2026-10-05&startsAt=08%3A00&endsAt=08%3A50&excludeBlockId=${BLOCK}`,
    );
    expect(got.map((t) => t.match)).toEqual(["subject", "other"]);
  });

  it("sets a level's grid", async () => {
    const fetchMock = stub({ settings: { levelId: LEVEL, gridStart: "07:30", gridEnd: "14:00" } });
    const got = await putSettings(BASE, LEVEL, { gridStart: "07:30", gridEnd: "14:00" });
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/schedule/settings/${LEVEL}`);
    expect(call(fetchMock)[1].method).toBe("PUT");
    expect(got.gridStart).toBe("07:30");
  });

  it("reads the history of a cycle", async () => {
    const fetchMock = stub({ events: [{ id: "e", at: "2026-10-01T15:00:00Z", actorId: "u", actorName: "Dirección", action: "override_created", blockId: BLOCK, overrideId: OVERRIDE, date: "2026-10-05", reason: "Gripe", summary: "Suplencia" }] });
    const got = await listEvents(BASE, CYCLE);
    expect(call(fetchMock)[0]).toBe(`http://gw/api/v1/schedule/events?cycleId=${CYCLE}`);
    expect(got[0]?.actorName).toBe("Dirección");
  });

  it("a refusal carries the server's sentence and its X-Error-Code", async () => {
    stub("Ana Ruiz ya da Matemáticas a 3.º A el lunes de 08:00 a 08:50", 409, { "X-Error-Code": "SCHEDULE_TEACHER_BUSY" });
    const err = await createBlock(BASE, { assignmentId: "a", weekday: 1, startsAt: "08:00", endsAt: "08:50" }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ScheduleError);
    expect((err as ScheduleError).message).toBe("Ana Ruiz ya da Matemáticas a 3.º A el lunes de 08:00 a 08:50");
    expect((err as ScheduleError).code).toBe("SCHEDULE_TEACHER_BUSY");
    expect((err as ScheduleError).status).toBe(409);
  });

  it("refuses an id that is not an id before any request", async () => {
    const fetchMock = stub({ block });
    await expect(updateBlock(BASE, "../x", { assignmentId: "a", weekday: 1, startsAt: "08:00", endsAt: "08:50" })).rejects.toThrow();
    await expect(getBoard(BASE, "nope")).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
