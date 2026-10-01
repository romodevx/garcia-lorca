import { describe, expect, it } from "vitest";

import {
  EMPTY_FILTER,
  NONE,
  applyStudentFilter,
  chooseGrade,
  chooseGroup,
  chooseLevel,
  filterFromParams,
  filterToParams,
  isFilterActive,
  studentFacets,
  type FilterCatalog,
  type FilterableStudent,
} from "./studentFilter";

const catalog: FilterCatalog = {
  levels: [
    { id: "pre", name: "Preescolar" },
    { id: "pri", name: "Primaria" },
    { id: "sec", name: "Secundaria" },
  ],
  grades: [
    // Out of order on purpose: the chips follow the ordinal, not the payload.
    { id: "pre-2", levelId: "pre", name: "2°", ordinal: 2 },
    { id: "pre-1", levelId: "pre", name: "1°", ordinal: 1 },
    { id: "pri-1", levelId: "pri", name: "1°", ordinal: 1 },
  ],
  groups: [
    { id: "pre-1-b", gradeLevelId: "pre-1", name: "B" },
    { id: "pre-1-a", gradeLevelId: "pre-1", name: "A" },
    { id: "pre-2-a", gradeLevelId: "pre-2", name: "A" },
    { id: "pri-1-a", gradeLevelId: "pri-1", name: "A" },
  ],
};

const kid = (
  id: string,
  lastName: string,
  firstName: string,
  levelId: string,
  gradeLevelId: string,
  groupId: string,
  searchText: string[] = [],
): FilterableStudent => ({ id, lastName, firstName, levelId, gradeLevelId, groupId, searchText });

const roll: FilterableStudent[] = [
  kid("1", "Pérez", "Ana Sofía", "pre", "pre-1", "pre-1-b", ["CURPANA", "Laura Núñez"]),
  kid("2", "Álvarez", "Luis", "pre", "pre-1", "pre-1-a"),
  kid("3", "Zúñiga", "Mateo", "pre", "pre-2", "pre-2-a"),
  kid("4", "Cruz", "Valentina", "pri", "pri-1", "pri-1-a"),
  kid("5", "Beltrán", "Diego", "pre", "", ""),
  kid("6", "Ortega", "Emilia", "", "", ""),
];

const ids = (list: readonly FilterableStudent[]): string[] => list.map((s) => s.id);

describe("applyStudentFilter", () => {
  it("returns everybody, surname first, when nothing is chosen", () => {
    expect(ids(applyStudentFilter(roll, EMPTY_FILTER))).toEqual(["2", "5", "4", "6", "1", "3"]);
  });

  it("narrows level → grade → group: 1°B preescolar", () => {
    const f = chooseGroup(chooseGrade(chooseLevel(EMPTY_FILTER, "pre"), "pre-1"), "pre-1-b");
    expect(ids(applyStudentFilter(roll, f))).toEqual(["1"]);
  });

  it("a level alone keeps every grade of it, placed or not", () => {
    expect(ids(applyStudentFilter(roll, chooseLevel(EMPTY_FILTER, "pre")))).toEqual(["2", "5", "1", "3"]);
  });

  it("finds pupils with no level, and pupils of a level with no group", () => {
    expect(ids(applyStudentFilter(roll, chooseLevel(EMPTY_FILTER, NONE)))).toEqual(["6"]);
    expect(ids(applyStudentFilter(roll, chooseGrade(chooseLevel(EMPTY_FILTER, "pre"), NONE)))).toEqual(["5"]);
  });

  it.each([
    ["surname, no accent", "perez", ["1"]],
    ["first name, upper case", "ANA", ["1"]],
    ["accented query over plain text", "zúñiga", ["3"]],
    ["spoken order", "ana sofia perez", ["1"]],
    ["list order", "perez ana", ["1"]],
    ["an extra field (the tutor)", "laura nunez", ["1"]],
    ["nobody", "xyz", []],
  ])("searches by %s", (_, q, want) => {
    expect(ids(applyStudentFilter(roll, { ...EMPTY_FILTER, q }))).toEqual(want);
  });

  it("combines search with the chips", () => {
    const f = { ...chooseLevel(EMPTY_FILTER, "pri"), q: "a" };
    expect(ids(applyStudentFilter(roll, f))).toEqual(["4"]);
  });
});

describe("the chained choosers", () => {
  it("choosing a level forgets the grade and group of another", () => {
    const deep = chooseGroup(chooseGrade(chooseLevel(EMPTY_FILTER, "pre"), "pre-1"), "pre-1-b");
    expect(chooseLevel(deep, "pri")).toEqual({ ...EMPTY_FILTER, levelId: "pri" });
  });

  it("choosing a grade forgets the group", () => {
    const deep = chooseGroup(chooseGrade(chooseLevel(EMPTY_FILTER, "pre"), "pre-1"), "pre-1-b");
    expect(chooseGrade(deep, "pre-2")).toEqual({ ...EMPTY_FILTER, levelId: "pre", gradeId: "pre-2" });
  });

  it("choosing the chosen chip again turns it off (and what hangs from it)", () => {
    const deep = chooseGroup(chooseGrade(chooseLevel(EMPTY_FILTER, "pre"), "pre-1"), "pre-1-b");
    expect(chooseGroup(deep, "pre-1-b").groupId).toBe("");
    expect(chooseGrade(deep, "pre-1")).toEqual({ ...EMPTY_FILTER, levelId: "pre" });
    expect(chooseLevel(deep, "pre")).toEqual(EMPTY_FILTER);
  });

  it("keeps the search while the chips change", () => {
    expect(chooseLevel({ ...EMPTY_FILTER, q: "ana" }, "pre").q).toBe("ana");
  });
});

describe("studentFacets", () => {
  it("offers every configured level with its count, and 'Sin nivel' only when somebody lacks one", () => {
    const f = studentFacets(roll, catalog, EMPTY_FILTER);
    expect(f.levels).toEqual([
      { id: "pre", label: "Preescolar", count: 4 },
      { id: "pri", label: "Primaria", count: 1 },
      { id: "sec", label: "Secundaria", count: 0 },
      { id: NONE, label: "Sin nivel", count: 1 },
    ]);
    expect(f.grades).toEqual([]);
    expect(f.groups).toEqual([]);
  });

  it("shows only the chosen level's grades, in their order, plus the unplaced", () => {
    const f = studentFacets(roll, catalog, chooseLevel(EMPTY_FILTER, "pre"));
    expect(f.grades).toEqual([
      { id: "pre-1", label: "1°", count: 2 },
      { id: "pre-2", label: "2°", count: 1 },
      { id: NONE, label: "Sin grado", count: 1 },
    ]);
    expect(f.groups).toEqual([]);
  });

  it("shows only the chosen grade's groups, by name", () => {
    const f = studentFacets(roll, catalog, chooseGrade(chooseLevel(EMPTY_FILTER, "pre"), "pre-1"));
    expect(f.groups).toEqual([
      { id: "pre-1-a", label: "A", count: 1 },
      { id: "pre-1-b", label: "B", count: 1 },
    ]);
  });

  it("offers no grade row for a level the school gave no grades (not a lone 'Sin grado')", () => {
    const f = studentFacets(roll, { ...catalog, grades: [] }, chooseLevel(EMPTY_FILTER, "pre"));
    expect(f.grades).toEqual([]);
  });

  it("still shows a chosen grade of a level that lost its grades, so it can be turned off", () => {
    const f = studentFacets(roll, { ...catalog, grades: [] }, chooseGrade(chooseLevel(EMPTY_FILTER, "pre"), "pre-1"));
    expect(f.grades).toEqual([{ id: "pre-1", label: "(ya no existe)", count: 0 }]);
  });

  it("keeps the chosen chip visible even when the search empties it, so it can be turned off", () => {
    const f = studentFacets(roll, catalog, { ...chooseLevel(EMPTY_FILTER, NONE), q: "perez" });
    expect(f.levels.at(-1)).toEqual({ id: NONE, label: "Sin nivel", count: 0 });
  });

  it("shows a chosen id the catalog no longer has (an old link), so it can be turned off", () => {
    const f = studentFacets(roll, catalog, chooseLevel(EMPTY_FILTER, "gone"));
    expect(f.levels.at(-1)).toEqual({ id: "gone", label: "(ya no existe)", count: 0 });
  });

  it("counts under the search, so the numbers match what a click will show", () => {
    const f = studentFacets(roll, catalog, { ...EMPTY_FILTER, q: "perez" });
    expect(f.levels.find((l) => l.id === "pre")?.count).toBe(1);
    expect(f.levels.find((l) => l.id === NONE)).toBeUndefined();
  });
});

describe("the URL form", () => {
  it("round-trips a filter, leaving empty keys out", () => {
    const f = { levelId: "pre", gradeId: "pre-1", groupId: "", q: "ana" };
    const params = filterToParams(f);
    expect(params.toString()).toBe("nivel=pre&grado=pre-1&q=ana");
    expect(filterFromParams(params)).toEqual(f);
  });

  it("drops a grade without a level and a group without a grade", () => {
    expect(filterFromParams(new URLSearchParams("grado=pre-1&grupo=pre-1-a"))).toEqual(EMPTY_FILTER);
  });

  it("keeps other keys of the page untouched", () => {
    const params = filterToParams({ ...EMPTY_FILTER, levelId: "pri" }, new URLSearchParams("estado=pending&nivel=pre"));
    expect(params.toString()).toBe("estado=pending&nivel=pri");
  });
});

describe("isFilterActive", () => {
  it("is false only when nothing is chosen and the search is blank", () => {
    expect(isFilterActive(EMPTY_FILTER)).toBe(false);
    expect(isFilterActive({ ...EMPTY_FILTER, q: "  " })).toBe(false);
    expect(isFilterActive(chooseLevel(EMPTY_FILTER, "pre"))).toBe(true);
    expect(isFilterActive({ ...EMPTY_FILTER, q: "a" })).toBe(true);
  });
});
