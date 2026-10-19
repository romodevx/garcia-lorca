import { describe, expect, it } from "vitest";

import { byFullName, fullName } from "./personName";

/**
 * ISSUE-CC-122: the teacher pickers sorted by last name but showed "first
 * last", so the list read as unsorted ("Luis Pérez" before "Ana Ruiz").
 */
describe("byFullName", () => {
  it("orders people the way their names are shown", () => {
    const people = [
      { id: "1", firstName: "Luis", lastName: "Pérez" },
      { id: "2", firstName: "Ángela", lastName: "Soto" },
      { id: "3", firstName: "Ana", lastName: "Ruiz" },
    ];

    expect(byFullName(people).map(fullName)).toEqual(["Ana Ruiz", "Ángela Soto", "Luis Pérez"]);
  });

  it("does not reorder the list it was given", () => {
    const people = [{ firstName: "Luis", lastName: "Pérez" }, { firstName: "Ana", lastName: "Ruiz" }];
    byFullName(people);
    expect(people[0]?.firstName).toBe("Luis");
  });
});
