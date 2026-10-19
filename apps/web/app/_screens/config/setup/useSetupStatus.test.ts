import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { API_URL } from "../../../api-url";
import * as configApi from "../../../config/api";
import type { GradeLevel, Group, SchoolProfile, Term } from "../../../config/api";
import type { TuitionPlan } from "../../../finance/api";
import { useSetupStatus, type RefreshKind } from "./useSetupStatus";

vi.mock("../../../config/api", () => ({
  listGradeLevels: vi.fn(),
  listTerms: vi.fn(),
  listGroups: vi.fn(),
  listSubjects: vi.fn(),
}));
const config = vi.mocked(configApi);

const grade = (id: string, levelId: string): GradeLevel => ({ id, levelId, name: id, ordinal: 1 });
const term = (id: string, cycleId: string, levelId: string): Term => ({ id, cycleId, levelId, name: id, ordinal: 1, startsOn: "", endsOn: "" });
const group = (id: string, cycleId: string, gradeLevelId: string): Group => ({ id, cycleId, gradeLevelId, name: id, capacity: 30 });

/*
 * The server, in miniature. Every mock answers according to its arguments, so a
 * filtered call returns LESS than an unfiltered one: a status built from the
 * wrong read cannot pass by accident.
 */
const db = {
  grades: [grade("p1", "prim"), grade("s1", "sec")],
  terms: [term("t1", "c1", "prim"), term("t2", "c1", "sec"), term("t3", "c2", "prim")],
  groups: [group("g1", "c1", "p1"), group("g2", "c1", "s1"), group("g3", "c2", "p1")],
};

beforeEach(() => {
  vi.clearAllMocks();
  config.listGradeLevels.mockImplementation(async (_url, levelId) => db.grades.filter((g) => levelId === undefined || g.levelId === levelId));
  config.listTerms.mockImplementation(async (_url, cycleId, levelId) =>
    db.terms.filter((t) => t.cycleId === cycleId && (levelId === undefined || t.levelId === levelId)),
  );
  config.listSubjects.mockResolvedValue([{ id: "m1", gradeLevelId: "p1", name: "MATEMÁTICAS" }]);
  config.listGroups.mockImplementation(async (_url, cycleId, gradeLevelId) =>
    db.groups.filter((g) => g.cycleId === cycleId && (gradeLevelId === undefined || g.gradeLevelId === gradeLevelId)),
  );
});

function deferred<T>(): { promise: Promise<T>; resolve: (v: T) => void } {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((r) => (resolve = r));
  return { promise, resolve };
}

const ids = (read: { kind: string; value?: { id: string }[] }): string[] | string =>
  read.kind === "ready" && read.value !== undefined ? read.value.map((x) => x.id) : read.kind;

describe("useSetupStatus: the three unfiltered reads (§4.1)", () => {
  it("reads every grade, and every term and group of the cycle, without a filter", async () => {
    const { result } = renderHook(() => useSetupStatus("c1"));

    await waitFor(() => expect(result.current.groups.kind).toBe("ready"));
    expect(config.listGradeLevels).toHaveBeenCalledWith(API_URL);
    expect(config.listTerms).toHaveBeenCalledWith(API_URL, "c1");
    expect(config.listGroups).toHaveBeenCalledWith(API_URL, "c1");
    expect(ids(result.current.grades)).toEqual(["p1", "s1"]);
    expect(ids(result.current.terms)).toEqual(["t1", "t2"]);
    expect(ids(result.current.groups)).toEqual(["g1", "g2"]);
  });

  it("is loading until each read answers", () => {
    const { result } = renderHook(() => useSetupStatus("c1"));
    expect(result.current.grades.kind).toBe("loading");
    expect(result.current.terms.kind).toBe("loading");
    expect(result.current.groups.kind).toBe("loading");
  });

  it("says a read failed", async () => {
    config.listTerms.mockRejectedValue(new Error("boom"));
    const { result } = renderHook(() => useSetupStatus("c1"));
    await waitFor(() => expect(result.current.terms.kind).toBe("failed"));
    expect(result.current.grades.kind).toBe("ready");
  });

  it("reads nothing per cycle without a cycle", async () => {
    const { result } = renderHook(() => useSetupStatus(undefined));
    await waitFor(() => expect(result.current.grades.kind).toBe("ready"));
    expect(config.listTerms).not.toHaveBeenCalled();
    expect(config.listGroups).not.toHaveBeenCalled();
  });

  it("re-reads the cycle's terms and groups on a cycle change, not the grades", async () => {
    const { result, rerender } = renderHook(({ cycleId }) => useSetupStatus(cycleId), { initialProps: { cycleId: "c1" } });
    await waitFor(() => expect(result.current.groups.kind).toBe("ready"));

    rerender({ cycleId: "c2" });

    expect(result.current.terms.kind).toBe("loading");
    await waitFor(() => expect(ids(result.current.terms)).toEqual(["t3"]));
    expect(ids(result.current.groups)).toEqual(["g3"]);
    expect(config.listGradeLevels).toHaveBeenCalledTimes(1);
  });
});

describe("useSetupStatus: refresh after each create (§4.2)", () => {
  const cases: [RefreshKind, number, number, number][] = [
    // kind, grade reads, term reads, group reads — after the first round of one each.
    ["grades", 2, 1, 1],
    ["terms", 1, 2, 1],
    ["groups", 1, 1, 2],
    ["all", 2, 2, 2],
  ];

  it.each(cases)("refresh(%s) re-reads only what it names", async (kind, gradeReads, termReads, groupReads) => {
    const { result } = renderHook(() => useSetupStatus("c1"));
    await waitFor(() => expect(result.current.groups.kind).toBe("ready"));

    act(() => result.current.refresh(kind));

    await waitFor(() => expect(config.listGradeLevels).toHaveBeenCalledTimes(gradeReads));
    expect(config.listTerms).toHaveBeenCalledTimes(termReads);
    expect(config.listGroups).toHaveBeenCalledTimes(groupReads);
  });

  it("shows the new grade once refreshed, with no reload", async () => {
    const { result } = renderHook(() => useSetupStatus("c1"));
    await waitFor(() => expect(result.current.grades.kind).toBe("ready"));

    db.grades = [...db.grades, grade("s2", "sec")];
    act(() => result.current.refresh("grades"));

    await waitFor(() => expect(ids(result.current.grades)).toEqual(["p1", "s1", "s2"]));
    db.grades = db.grades.slice(0, 2);
  });

  it("keeps the previous answer on screen while a refresh is in flight", async () => {
    const { result } = renderHook(() => useSetupStatus("c1"));
    await waitFor(() => expect(result.current.grades.kind).toBe("ready"));

    config.listGradeLevels.mockReturnValueOnce(new Promise(() => {}));
    act(() => result.current.refresh("grades"));

    expect(ids(result.current.grades)).toEqual(["p1", "s1"]);
  });
});

describe("useSetupStatus: races", () => {
  it("drops the old cycle's late terms after a cycle switch", async () => {
    const late = deferred<Term[]>();
    config.listTerms.mockImplementation(async (_url, cycleId) => (cycleId === "c1" ? late.promise : db.terms.filter((t) => t.cycleId === cycleId)));
    const { result, rerender } = renderHook(({ cycleId }) => useSetupStatus(cycleId), { initialProps: { cycleId: "c1" } });

    rerender({ cycleId: "c2" });
    await waitFor(() => expect(ids(result.current.terms)).toEqual(["t3"]));
    await act(async () => late.resolve([term("t1", "c1", "prim")]));

    expect(ids(result.current.terms)).toEqual(["t3"]);
  });

  it("keeps the newest refresh when an older one answers last", async () => {
    const { result } = renderHook(() => useSetupStatus("c1"));
    await waitFor(() => expect(result.current.groups.kind).toBe("ready"));

    const older = deferred<Group[]>();
    config.listGroups.mockReturnValueOnce(older.promise);
    act(() => result.current.refresh("groups"));
    act(() => result.current.refresh("groups"));
    await waitFor(() => expect(config.listGroups).toHaveBeenCalledTimes(3));
    await act(async () => older.resolve([]));

    expect(ids(result.current.groups)).toEqual(["g1", "g2"]);
  });
});

const plan = (cycleId: string, levelId: string): TuitionPlan => ({ id: `${cycleId}-${levelId}`, cycleId, levelId }) as TuitionPlan;
const PROFILE: SchoolProfile = { name: "Colegio", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" };

describe("useSetupStatus: what the mounted panels report", () => {
  it("takes the profile from SchoolProfilePanel, and its failure", () => {
    const { result } = renderHook(() => useSetupStatus("c1"));
    expect(result.current.profile.kind).toBe("loading");

    act(() => result.current.onProfileLoaded(PROFILE));
    expect(result.current.profile).toEqual({ kind: "ready", value: PROFILE });

    act(() => result.current.onProfileLoaded("failed"));
    expect(result.current.profile.kind).toBe("failed");
  });

  it("takes the selected cycle's plans and drops another cycle's", () => {
    const { result, rerender } = renderHook(({ cycleId }) => useSetupStatus(cycleId), { initialProps: { cycleId: "c1" } });
    expect(result.current.plans.kind).toBe("loading");

    act(() => result.current.onPlansLoaded("c1", [plan("c1", "prim")]));
    expect(ids(result.current.plans)).toEqual(["c1-prim"]);

    rerender({ cycleId: "c2" });
    expect(result.current.plans.kind).toBe("loading");

    act(() => result.current.onPlansLoaded("c2", [plan("c2", "sec")]));
    act(() => result.current.onPlansLoaded("c1", [plan("c1", "prim")]));
    expect(ids(result.current.plans)).toEqual(["c2-sec"]);

    act(() => result.current.onPlansLoaded("c2", "failed"));
    expect(result.current.plans.kind).toBe("failed");
  });

  it("takes the selected cycle's closed days and drops another cycle's", () => {
    const { result, rerender } = renderHook(({ cycleId }) => useSetupStatus(cycleId), { initialProps: { cycleId: "c1" } });

    act(() => result.current.onCalendarLoaded("c1", 3));
    expect(result.current.closedDays).toEqual({ kind: "ready", value: 3 });

    rerender({ cycleId: "c2" });
    act(() => result.current.onCalendarLoaded("c1", 5));
    expect(result.current.closedDays.kind).toBe("loading");

    act(() => result.current.onCalendarLoaded("c2", "failed"));
    expect(result.current.closedDays.kind).toBe("failed");
  });
});

// ISSUE-CC-107: Materias reads the school's subjects so its step can be done.
describe("useSetupStatus: subjects", () => {
  it("reads every subject of the school, and re-reads on refresh", async () => {
    const { result } = renderHook(() => useSetupStatus("c1"));
    await waitFor(() => expect(ids(result.current.subjects)).toEqual(["m1"]));
    expect(config.listSubjects).toHaveBeenCalledWith(API_URL, "");

    config.listSubjects.mockResolvedValue([
      { id: "m1", gradeLevelId: "p1", name: "MATEMÁTICAS" },
      { id: "m2", gradeLevelId: "s1", name: "ESPAÑOL" },
    ]);
    act(() => result.current.refresh("subjects"));

    await waitFor(() => expect(ids(result.current.subjects)).toEqual(["m1", "m2"]));
  });
});
