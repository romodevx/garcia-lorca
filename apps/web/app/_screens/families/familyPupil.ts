import type { AdmissionStatus } from "../../students/api";

/**
 * A pupil as the families surface shows it: the name as the school reads it
 * and the classroom with its grade ("3° PRIMARIA · B"). Built once by the
 * students list from what it already read, so families cost no extra reads.
 */
export interface FamilyPupil {
  id: string;
  firstName: string;
  lastName: string;
  maternalLastName?: string;
  /** "Paterno Materno Nombre(s)". */
  name: string;
  room: string;
  admissionStatus: AdmissionStatus;
}

/** The pupils by id, for the cards and rows that only hold ids. */
export function pupilsById(pupils: readonly FamilyPupil[]): ReadonlyMap<string, FamilyPupil> {
  return new Map(pupils.map((p) => [p.id, p]));
}
