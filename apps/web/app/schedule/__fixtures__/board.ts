import type { ScheduleBoard } from "../api";

export const PRIMARIA = "10000000-0000-4000-8000-000000000001";
export const SECUNDARIA = "10000000-0000-4000-8000-000000000002";
export const G3 = "20000000-0000-4000-8000-000000000003";
export const G1S = "20000000-0000-4000-8000-000000000011";
export const G3A = "30000000-0000-4000-8000-00000000003a";
export const G3B = "30000000-0000-4000-8000-00000000003b";
export const G1SA = "30000000-0000-4000-8000-0000000001aa";
export const MATE = "40000000-0000-4000-8000-000000000001";
export const ESP = "40000000-0000-4000-8000-000000000002";
export const ANA = "50000000-0000-4000-8000-000000000001";
export const LUIS = "50000000-0000-4000-8000-000000000002";

/** A small school: Primaria 3.º A/B and Secundaria 1.º A. */
export function sampleBoard(over: Partial<ScheduleBoard> = {}): ScheduleBoard {
  return {
    cycle: { id: "c0000000-0000-4000-8000-000000000001", name: "2026-2027", startsOn: "2026-08-24", endsOn: "2027-07-09", active: true },
    cycles: [{ id: "c0000000-0000-4000-8000-000000000001", name: "2026-2027", startsOn: "2026-08-24", endsOn: "2027-07-09", active: true }],
    started: false,
    settings: [{ levelId: PRIMARIA, gridStart: "07:00", gridEnd: "14:00" }],
    levels: [{ id: PRIMARIA, name: "Primaria" }, { id: SECUNDARIA, name: "Secundaria" }],
    grades: [{ id: G3, levelId: PRIMARIA, name: "3.º", ordinal: 3 }, { id: G1S, levelId: SECUNDARIA, name: "1.º", ordinal: 1 }],
    groups: [{ id: G3A, gradeLevelId: G3, name: "A" }, { id: G3B, gradeLevelId: G3, name: "B" }, { id: G1SA, gradeLevelId: G1S, name: "A" }],
    subjects: [{ id: MATE, gradeLevelId: G3, name: "Matemáticas" }, { id: ESP, gradeLevelId: G3, name: "Español" }],
    teachers: [{ id: ANA, firstName: "Ana", lastName: "Ruiz" }, { id: LUIS, firstName: "Luis", lastName: "Pérez" }],
    assignments: [
      { id: "60000000-0000-4000-8000-000000000001", groupId: G3A, subjectId: MATE, teacherId: ANA },
      { id: "60000000-0000-4000-8000-000000000002", groupId: G3A, subjectId: ESP, teacherId: LUIS },
      { id: "60000000-0000-4000-8000-000000000003", groupId: G3B, subjectId: MATE, teacherId: ANA },
    ],
    blocks: [
      { id: "70000000-0000-4000-8000-000000000001", cycleId: "c", assignmentId: "60000000-0000-4000-8000-000000000001", groupId: G3A, subjectId: MATE, teacherId: ANA, weekday: 1, startsAt: "08:00", endsAt: "08:50" },
      { id: "70000000-0000-4000-8000-000000000002", cycleId: "c", assignmentId: "60000000-0000-4000-8000-000000000002", groupId: G3A, subjectId: ESP, teacherId: LUIS, weekday: 1, startsAt: "08:50", endsAt: "09:40" },
      { id: "70000000-0000-4000-8000-000000000003", cycleId: "c", assignmentId: "60000000-0000-4000-8000-000000000003", groupId: G3B, subjectId: MATE, teacherId: ANA, weekday: 2, startsAt: "08:00", endsAt: "08:50" },
    ],
    ...over,
  };
}
