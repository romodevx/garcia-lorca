import { describe, expect, it } from "vitest";

import type { LabelledGroup, Level } from "../../config/api";
import type { Student } from "../../students/api";
import { groupSections, recipientNames, summarizeRecipients } from "./audience";

const level = (id: string, name: string): Level => ({ id, name, sepCode: id, enrollmentGranularity: "cycle" });
const LEVELS = [level("pre", "Preescolar"), level("pri", "Primaria"), level("sec", "Secundaria")];

function group(id: string, gradeName: string, name: string, levelId: string, ordinal: number): LabelledGroup {
  return { id, cycleId: "c1", gradeLevelId: `${levelId}-${ordinal}`, name, capacity: 30, label: `${gradeName} ${levelId} · ${name}`, gradeName, levelId, ordinal };
}

const sectionTitles = (groups: readonly LabelledGroup[]): (string | undefined)[] =>
  groupSections(groups, LEVELS).map((s) => s.title);

describe("groupSections", () => {
  it("files the groups under their level, in the catalogue's level order", () => {
    const groups = [group("s1", "1°", "A", "sec", 1), group("p1", "1°", "A", "pri", 1)];
    expect(sectionTitles(groups)).toEqual(["Primaria", "Secundaria"]);
  });

  it("orders each level's groups by grade, then by letter", () => {
    const groups = [group("b2", "2°", "B", "pri", 2), group("a2", "2°", "A", "pri", 2), group("a1", "1°", "A", "pri", 1)];
    expect(groupSections(groups, LEVELS)[0]?.targets.map((t) => t.id)).toEqual(["a1", "a2", "b2"]);
  });

  it("shows the short name and keeps today's full label as the accessible one", () => {
    const [target] = groupSections([group("a1", "1°", "A", "pri", 1)], LEVELS)[0]?.targets ?? [];
    expect(target).toEqual({ id: "a1", label: "1° pri · A", text: "1° A" });
  });

  it("puts the groups whose level is unknown last, under «Otros»", () => {
    const stray: LabelledGroup = { id: "x", cycleId: "c1", gradeLevelId: "gone", name: "X", capacity: 30, label: "X" };
    const sections = groupSections([stray, group("a1", "1°", "A", "pri", 1)], LEVELS);
    expect(sections.map((s) => s.title)).toEqual(["Primaria", "Otros"]);
    expect(sections[1]?.targets).toEqual([{ id: "x", label: "X", text: "X" }]);
  });

  it("is one untitled list when no group's level is known", () => {
    const bare: LabelledGroup[] = [
      { id: "b", cycleId: "c1", gradeLevelId: "g", name: "B", capacity: 30, label: "1° B" },
      { id: "a", cycleId: "c1", gradeLevelId: "g", name: "A", capacity: 30, label: "1° A" },
    ];
    const sections = groupSections(bare, LEVELS);
    expect(sections).toHaveLength(1);
    expect(sections[0]?.title).toBeUndefined();
    expect(sections[0]?.targets.map((t) => t.text)).toEqual(["1° A", "1° B"]);
  });

  it("has no sections without groups", () => {
    expect(groupSections([], LEVELS)).toEqual([]);
  });
});

const pupil = (id: string, firstName: string, lastName: string): Student => ({
  id, curp: id, firstName, lastName, level: "Primaria", levelId: "pri", gradeGroup: "A", guardianName: "", admissionStatus: "enrolled",
});

describe("recipientNames", () => {
  const catalogue = {
    groups: [group("a1", "1°", "A", "pri", 1), group("b1", "1°", "B", "pri", 1)],
    levels: LEVELS,
    students: [pupil("s1", "Valentina", "Cruz")],
  };

  it("names the picked groups by their full label, in the order they were picked", () => {
    expect(recipientNames("group", ["b1", "a1"], catalogue)).toEqual(["1° pri · B", "1° pri · A"]);
  });

  it("names levels and pupils the way their lists do", () => {
    expect(recipientNames("level", ["sec"], catalogue)).toEqual(["Secundaria"]);
    expect(recipientNames("student", ["s1"], catalogue)).toEqual(["Cruz Valentina"]);
  });

  it("skips an id it cannot name", () => {
    expect(recipientNames("group", ["gone"], catalogue)).toEqual([]);
  });
});

describe("summarizeRecipients", () => {
  it("shows up to three names and counts the rest", () => {
    expect(summarizeRecipients(["A", "B", "C"])).toEqual({ shown: ["A", "B", "C"], more: 0 });
    expect(summarizeRecipients(["A", "B", "C", "D", "E"])).toEqual({ shown: ["A", "B", "C"], more: 2 });
  });
});
