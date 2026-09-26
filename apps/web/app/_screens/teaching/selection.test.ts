import { describe, expect, it } from "vitest";

import { parseSelection, resolveSelection, selectionToSearch, subjectLabel, type TeachingOption } from "./selection";

const mat2B: TeachingOption = { id: "a1", groupId: "g2b", groupName: "2° B", subjectId: "s-mat", subjectName: "Matemáticas" };
const mat3A: TeachingOption = { id: "a2", groupId: "g3a", groupName: "3° A", subjectId: "s-mat", subjectName: "Matemáticas" };
const esp2B: TeachingOption = { id: "a3", groupId: "g2b", groupName: "2° B", subjectId: "s-esp", subjectName: "Español" };

describe("the selection in the query string", () => {
  it("reads asignatura, grupo and clase as ids, ignoring everything else", () => {
    expect(parseSelection("?asignatura=s-mat&clase=a1&x=1")).toEqual({ subject: "s-mat", assignment: "a1" });
    expect(parseSelection("?grupo=g2b")).toEqual({ group: "g2b" });
    expect(parseSelection("")).toEqual({});
  });

  it("writes only what is chosen, and nothing at all for the first step", () => {
    expect(selectionToSearch({ subject: "s-mat", assignment: "a1" })).toBe("?asignatura=s-mat&clase=a1");
    expect(selectionToSearch({ subject: "s-esp" })).toBe("?asignatura=s-esp");
    expect(selectionToSearch({ group: "g2b" })).toBe("?grupo=g2b");
    expect(selectionToSearch({})).toBe("");
  });
});

describe("resolveSelection — by subject then class", () => {
  it("no options is the empty state", () => {
    expect(resolveSelection([], {}, true).step).toBe("empty");
  });

  it("two subjects and nothing chosen asks for the subject, alphabetically by name", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], {}, true);
    expect(r.step).toBe("subject");
    expect(r.subjects).toEqual([
      { id: "s-esp", name: "Español" },
      { id: "s-mat", name: "Matemáticas" },
    ]);
  });

  it("a chosen subject with two classes asks for the group, only among that subject's", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], { subject: "s-mat" }, true);
    expect(r.step).toBe("group");
    expect(r.groups.map((g) => g.id)).toEqual(["a1", "a2"]);
  });

  it("a subject with a single class goes straight to the list", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], { subject: "s-esp" }, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(esp2B);
    expect(r.groupSkipped).toBe(true);
  });

  it("a single subject is skipped without being in the URL", () => {
    const r = resolveSelection([mat2B, mat3A], {}, true);
    expect(r.subjectSkipped).toBe(true);
    expect(r.subject).toEqual({ id: "s-mat", name: "Matemáticas" });
    expect(r.step).toBe("group");
  });

  it("one assignment in total skips both steps", () => {
    const r = resolveSelection([mat2B], {}, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(mat2B);
  });

  it("a subject or class the teacher no longer has is treated as not chosen", () => {
    expect(resolveSelection([mat2B, esp2B, mat3A], { subject: "s-historia" }, true).step).toBe("subject");
    expect(resolveSelection([mat2B, esp2B, mat3A], { subject: "s-mat", assignment: "aX" }, true).step).toBe("group");
    // A class of ANOTHER subject is not a class of this one.
    expect(resolveSelection([mat2B, esp2B, mat3A], { subject: "s-mat", assignment: "a3" }, true).step).toBe("group");
  });

  it("both chosen is the list for that class", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], { subject: "s-mat", assignment: "a2" }, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(mat3A);
  });

  it("two subjects with the SAME name are two choices, each with its own classes", () => {
    const matSec: TeachingOption = { id: "a9", groupId: "g1s", groupName: "1° Sec", subjectId: "s-mat-sec", subjectName: "Matemáticas" };
    const r = resolveSelection([mat2B, matSec], {}, true);
    expect(r.step).toBe("subject");
    expect(r.subjects.map((s) => s.id).sort()).toEqual(["s-mat", "s-mat-sec"]);

    const chosen = resolveSelection([mat2B, matSec], { subject: "s-mat-sec" }, true);
    expect(chosen.step).toBe("ready");
    expect(chosen.selected).toBe(matSec);
  });

  it("two classes of one subject in groups of the same name are told apart by their id", () => {
    const twin: TeachingOption = { ...mat2B, id: "a1-twin", groupId: "g2b-other" };
    const r = resolveSelection([mat2B, twin], { subject: "s-mat", assignment: "a1-twin" }, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(twin);
  });

  it("a subject with no name is shown as such and can still be chosen", () => {
    const nameless: TeachingOption = { id: "a7", groupId: "g4c", groupName: "4° C", subjectId: "s-x", subjectName: "" };
    const r = resolveSelection([mat2B, nameless], {}, true);
    expect(r.subjects).toContainEqual({ id: "s-x", name: "Asignatura sin nombre" });

    const chosen = resolveSelection([mat2B, nameless], { subject: "s-x" }, true);
    expect(chosen.step).toBe("ready");
    expect(chosen.selected).toBe(nameless);
  });
});

describe("subjectLabel", () => {
  it("is the name, or a stand-in for a blank one", () => {
    expect(subjectLabel(mat2B)).toBe("Matemáticas");
    expect(subjectLabel({ ...mat2B, subjectName: "  " })).toBe("Asignatura sin nombre");
    expect(subjectLabel({ ...mat2B, subjectName: undefined })).toBe("Asignatura sin nombre");
  });
});

describe("resolveSelection — group only (attendance)", () => {
  const g1: TeachingOption = { id: "g1", groupId: "g1", groupName: "1° A" };
  const g2: TeachingOption = { id: "g2", groupId: "g2", groupName: "2° B" };

  it("asks for the group, ignoring any subject", () => {
    const r = resolveSelection([g1, g2], { subject: "s-mat" }, false);
    expect(r.step).toBe("group");
    expect(r.subjectSkipped).toBe(true);
  });

  it("a single group goes straight to the list", () => {
    expect(resolveSelection([g1], {}, false).selected).toBe(g1);
  });

  it("the chosen group is the list", () => {
    expect(resolveSelection([g1, g2], { group: "g2" }, false).selected).toBe(g2);
  });
});

describe("resolveSelection — by subject then GROUP (attendance per class)", () => {
  const matA: TeachingOption = { id: "a1", groupId: "gA", groupName: "1° A", subjectId: "s-mat", subjectName: "Matemáticas" };
  const matB: TeachingOption = { id: "a2", groupId: "gB", groupName: "1° B", subjectId: "s-mat", subjectName: "Matemáticas" };
  const diaA: TeachingOption = { id: "dia:gA", groupId: "gA", groupName: "1° A", subjectId: "dia", subjectName: "Lista del día" };

  it("the group step is keyed by the group id, within the chosen subject", () => {
    const r = resolveSelection([matA, matB, diaA], { subject: "s-mat", group: "gA" }, true, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(matA);
  });

  it("the same group under another subject is a different choice", () => {
    const r = resolveSelection([matA, matB, diaA], { subject: "dia", group: "gA" }, true, true);
    expect(r.selected).toBe(diaA);
    expect(r.groupSkipped).toBe(true);
  });

  it("an assignment id in the URL means nothing in this mode", () => {
    expect(resolveSelection([matA, matB, diaA], { subject: "s-mat", assignment: "a1" }, true, true).step).toBe("group");
  });
});
