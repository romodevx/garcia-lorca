import { describe, expect, it } from "vitest";

import type { Family, FamilyMember, FamilySuggestion } from "./api";
import {
  accessReachLine,
  accountsLabel,
  confidenceLabel,
  familiesAccess,
  familyIsFull,
  membershipOf,
  proposedFamilyName,
  reasonLabel,
  withApp,
  withoutFamily,
} from "./model";

const fam = (id: string, studentIds: string[], accountCount?: number): Family => ({ id, name: id.toUpperCase(), studentIds, createdAt: "", ...(accountCount === undefined ? {} : { accountCount }) });

describe("families model", () => {
  it("reads the three permissions the families surface cares about", () => {
    expect(familiesAccess(["students:read"])).toEqual({ canReadGuardians: false, canWriteGuardians: false, canUpdateStudents: false });
    expect(familiesAccess(["guardians:read", "guardians:write", "students:update"])).toEqual({ canReadGuardians: true, canWriteGuardians: true, canUpdateStudents: true });
    expect(familiesAccess(undefined).canWriteGuardians).toBe(false);
  });

  it("names each reason and confidence in Spanish", () => {
    expect(reasonLabel("same_surnames")).toBe("Mismos apellidos");
    expect(reasonLabel("shared_contact_email")).toBe("Mismo correo");
    expect(reasonLabel("shared_contact_phone")).toBe("Mismo teléfono");
    expect(reasonLabel("shared_account")).toBe("Ya comparten cuenta");
    expect(confidenceLabel("high")).toBe("Muy probable");
    expect(confidenceLabel("medium")).toBe("Posible");
  });

  it("maps every pupil to its family", () => {
    const map = membershipOf([fam("a", ["s1", "s2"]), fam("b", ["s3"])]);
    expect(map.get("s2")?.id).toBe("a");
    expect(map.get("s3")?.id).toBe("b");
    expect(map.has("s4")).toBe(false);
  });

  it("keeps the pupils without a family, never withdrawn or rejected ones", () => {
    const pupils = [
      { id: "s1", admissionStatus: "enrolled" as const },
      { id: "s2", admissionStatus: "pending" as const },
      { id: "s3", admissionStatus: "withdrawn" as const },
      { id: "s4", admissionStatus: "rejected" as const },
    ];
    expect(withoutFamily(pupils, membershipOf([fam("a", ["s1"])])).map((p) => p.id)).toEqual(["s2"]);
  });

  it("counts the families with the app and says 'N de 2'", () => {
    expect(withApp([fam("a", [], 0), fam("b", [], 1), fam("c", [], 2)])).toBe(2);
    expect(accountsLabel(1)).toBe("1 de 2");
    expect(familyIsFull(2)).toBe(true);
    expect(familyIsFull(1)).toBe(false);
    expect(familyIsFull(undefined)).toBe(false);
  });

  it("proposes the first pupil's two surnames as the family's name", () => {
    expect(proposedFamilyName({ lastName: "GARCÍA", maternalLastName: "LÓPEZ" })).toBe("GARCÍA LÓPEZ");
    expect(proposedFamilyName({ lastName: " PÉREZ ", maternalLastName: "" })).toBe("PÉREZ");
    expect(proposedFamilyName(undefined)).toBe("");
  });

  it("types a suggestion without surprises", () => {
    const s: FamilySuggestion = { signature: "x", proposedName: "P", studentIds: [], reasons: ["shared_account"], confidence: "high" };
    expect(s.reasons.map(reasonLabel)).toEqual(["Ya comparten cuenta"]);
  });
});

describe("who an account will see (M2)", () => {
  const kid = (firstName: string, over: Partial<FamilyMember> = {}): FamilyMember => ({
    id: firstName, firstName, lastName: "GARCÍA", maternalLastName: "LÓPEZ", level: "", gradeGroup: "", admissionStatus: "enrolled", draft: false, ...over,
  });
  const of = (members: FamilyMember[], studentIds = members.map((m) => m.id)) => ({ members, studentIds });

  it("names every child the account reaches, joined in Spanish", () => {
    expect(accessReachLine(of([kid("ANA"), kid("LUIS")]))).toBe("Verá a ANA y LUIS");
    expect(accessReachLine(of([kid("ANA"), kid("LUIS"), kid("EVA")]))).toBe("Verá a ANA, LUIS y EVA");
  });

  it.each([
    ["IVÁN", "e"],
    ["HILDA", "e"],
    ["ISAAC", "e"],
    ["IÑAKI", "e"],
    ["IOANA", "y"],
    ["IUVAL", "y"],
    ["HIELO", "y"],
    ["YARA", "y"],
    ["OMAR", "y"],
  ])("before %s it writes «%s»", (name, conjunction) => {
    expect(accessReachLine(of([kid("ANA"), kid(name)]))).toBe(`Verá a ANA ${conjunction} ${name}`);
  });

  it("leaves out drafts and pupils the school no longer has, as the server does", () => {
    const line = accessReachLine(of([kid("ANA"), kid("LUIS"), kid("TEO", { draft: true }), kid("EVA", { admissionStatus: "withdrawn" }), kid("SOL", { admissionStatus: "rejected" })]));
    expect(line).toBe("Verá a ANA y LUIS");
  });

  it("says nothing for a single child", () => {
    expect(accessReachLine(of([kid("ANA")]))).toBeUndefined();
    expect(accessReachLine(of([kid("ANA"), kid("TEO", { draft: true })]))).toBeUndefined();
    expect(accessReachLine(of([]))).toBeUndefined();
  });

  it("fails closed when the service left children out of the read", () => {
    expect(accessReachLine(of([kid("ANA")], ["ANA", "LUIS", "EVA"]))).toBe("Verá a 3 alumnos de su familia");
  });

  it("fails closed when the family could not be read", () => {
    expect(accessReachLine(undefined)).toBe("Verá a todos los alumnos de su familia");
  });
});
