import { describe, expect, it } from "vitest";

import {
  compareStudentNames,
  compareStudents,
  formatStudentName,
  sortStudents,
  studentInitials,
  studentNameSearchKeys,
  formatDisplayName,
} from "./studentName";

describe("formatStudentName", () => {
  it.each([
    [{ firstName: "Ana Sofía", lastName: "Pérez", maternalLastName: "Núñez" }, "Pérez Núñez Ana Sofía"],
    [{ firstName: "Luis", lastName: "Torres" }, "Torres Luis"],
    [{ firstName: "Luis", lastName: "Torres", maternalLastName: null }, "Torres Luis"],
    [{ firstName: "Luis", lastName: "TORRES GARCÍA" }, "TORRES GARCÍA Luis"],
    [{ firstName: "  Luis ", lastName: " Torres ", maternalLastName: "  " }, "Torres Luis"],
    [{ firstName: "Luis", lastName: "" }, "Luis"],
    [{ firstName: "", lastName: "" }, ""],
  ])("%j → %s", (student, want) => {
    expect(formatStudentName(student)).toBe(want);
  });
});

describe("compareStudents / sortStudents", () => {
  it("orders by paternal, maternal, then first name — Spanish, accent and case blind, ñ after n", () => {
    const pupils = [
      { firstName: "Zoe", lastName: "Ñuñez" },
      { firstName: "Beto", lastName: "álvarez", maternalLastName: "Ruiz" },
      { firstName: "Eva", lastName: "Oropeza" },
      { firstName: "Ana", lastName: "Alvarez", maternalLastName: "Ruiz" },
      { firstName: "Carla", lastName: "Nuñez" },
      { firstName: "Dora", lastName: "Álvarez", maternalLastName: "López" },
    ];
    expect(sortStudents(pupils).map(formatStudentName)).toEqual([
      "Álvarez López Dora",
      "Alvarez Ruiz Ana",
      "álvarez Ruiz Beto",
      "Nuñez Carla",
      "Ñuñez Zoe",
      "Oropeza Eva",
    ]);
  });

  it("does not mutate its input and keeps identical names in order", () => {
    const a = { id: "a", firstName: "Ana", lastName: "Ruiz" };
    const b = { id: "b", firstName: "Ana", lastName: "Ruiz" };
    const input = [b, a];
    expect(sortStudents(input).map((s) => s.id)).toEqual(["b", "a"]);
    expect(input).toEqual([b, a]);
    expect(compareStudents(a, b)).toBe(0);
  });

  it("puts a pupil without a name last", () => {
    const sorted = sortStudents([{ firstName: "", lastName: "" }, { firstName: "Ana", lastName: "Ruiz" }]);
    expect(sorted.map(formatStudentName)).toEqual(["Ruiz Ana", ""]);
  });
});

describe("compareStudentNames", () => {
  it("orders already formatted names the same way", () => {
    const names = ["Zúñiga Zoe", "Ñuñez Ana", "", "Ávila Luis", "Nuñez Eva"];
    expect([...names].sort(compareStudentNames)).toEqual(["Ávila Luis", "Nuñez Eva", "Ñuñez Ana", "Zúñiga Zoe", ""]);
  });
});

describe("short forms", () => {
  it("studentInitials is the monogram: first name, then paternal surname", () => {
    expect(studentInitials({ firstName: "ana", lastName: "Pérez", maternalLastName: "Núñez" })).toBe("AP");
  });
});

describe("formatDisplayName", () => {
  it.each([
    ["MARÍA DE LOS ÁNGELES", "", "María de los Ángeles"],
    ["MARÍA", "DE LA CRUZ", "María de la Cruz"],
    ["", "DE LA CRUZ", "De la Cruz"],
    ["", "O'HIGGINS", "O'Higgins"],
    ["MARÍA JOSÉ", "", "María José"],
    ["", "ÑUÑEZ", "Ñuñez"],
    ["  VALENTINA maría  ", "GÓMEZ-RUIZ", "Valentina María Gómez-Ruiz"],
  ])("formats %j %j as %s", (firstName, surname, expected) => {
    expect(formatDisplayName(firstName, surname)).toBe(expected);
  });
});

describe("studentNameSearchKeys", () => {
  it("offers the name in both orders, so the office finds a pupil however it types", () => {
    expect(studentNameSearchKeys({ firstName: "Ana", lastName: "Pérez", maternalLastName: "López" })).toEqual([
      "Pérez López Ana",
      "Ana Pérez López",
    ]);
  });
});
