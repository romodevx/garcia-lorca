/**
 * The web panel's door to the shared name rules (T3): "Paterno Materno
 * Nombre(s)" and alphabetical by surname in Spanish. The rules live in
 * `@garcia-lorca/domain/student-name` so the families' app uses the very same
 * code; screens import from here.
 */
export {
  compareStudentNames,
  compareStudents,
  formatDisplayName,
  formatStudentName,
  sortStudents,
  studentInitials,
  studentNameSearchKeys,
  type StudentNameParts,
} from "@garcia-lorca/domain/student-name";
