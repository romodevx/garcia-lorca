import { describe, expect, it } from "vitest";

import { parseSelection, resolveSelection, selectionToSearch, type TeachingOption } from "./selection";

const mat2B: TeachingOption = { id: "a1", groupId: "g2b", groupName: "2° B", subjectName: "Matemáticas" };
const mat3A: TeachingOption = { id: "a2", groupId: "g3a", groupName: "3° A", subjectName: "Matemáticas" };
const esp2B: TeachingOption = { id: "a3", groupId: "g2b", groupName: "2° B", subjectName: "Español" };

describe("the selection in the query string", () => {
  it("reads asignatura and grupo, ignoring everything else", () => {
    expect(parseSelection("?asignatura=Matem%C3%A1ticas&grupo=g2b&x=1")).toEqual({ subject: "Matemáticas", group: "g2b" });
    expect(parseSelection("")).toEqual({});
  });

  it("writes only what is chosen, and nothing at all for the first step", () => {
    expect(selectionToSearch({ subject: "Matemáticas", group: "g2b" })).toBe("?asignatura=Matem%C3%A1ticas&grupo=g2b");
    expect(selectionToSearch({ subject: "Español" })).toBe("?asignatura=Espa%C3%B1ol");
    expect(selectionToSearch({})).toBe("");
  });
});

describe("resolveSelection — by subject then group", () => {
  it("no options is the empty state", () => {
    expect(resolveSelection([], {}, true).step).toBe("empty");
  });

  it("two subjects and nothing chosen asks for the subject, alphabetically", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], {}, true);
    expect(r.step).toBe("subject");
    expect(r.subjects).toEqual(["Español", "Matemáticas"]);
  });

  it("a chosen subject with two groups asks for the group, only among that subject's", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], { subject: "Matemáticas" }, true);
    expect(r.step).toBe("group");
    expect(r.groups.map((g) => g.id)).toEqual(["a1", "a2"]);
  });

  it("a subject with a single group goes straight to the list", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], { subject: "Español" }, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(esp2B);
    expect(r.groupSkipped).toBe(true);
  });

  it("a single subject is skipped without being in the URL", () => {
    const r = resolveSelection([mat2B, mat3A], {}, true);
    expect(r.subjectSkipped).toBe(true);
    expect(r.subject).toBe("Matemáticas");
    expect(r.step).toBe("group");
  });

  it("one assignment in total skips both steps", () => {
    const r = resolveSelection([mat2B], {}, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(mat2B);
  });

  it("a subject or group the teacher no longer has is treated as not chosen", () => {
    expect(resolveSelection([mat2B, esp2B, mat3A], { subject: "Historia" }, true).step).toBe("subject");
    expect(resolveSelection([mat2B, esp2B, mat3A], { subject: "Matemáticas", group: "gX" }, true).step).toBe("group");
  });

  it("both chosen is the list for that pair", () => {
    const r = resolveSelection([mat2B, esp2B, mat3A], { subject: "Matemáticas", group: "g3a" }, true);
    expect(r.step).toBe("ready");
    expect(r.selected).toBe(mat3A);
  });
});

describe("resolveSelection — group only (attendance)", () => {
  const g1: TeachingOption = { id: "g1", groupId: "g1", groupName: "1° A" };
  const g2: TeachingOption = { id: "g2", groupId: "g2", groupName: "2° B" };

  it("asks for the group, ignoring any subject", () => {
    const r = resolveSelection([g1, g2], { subject: "Matemáticas" }, false);
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
