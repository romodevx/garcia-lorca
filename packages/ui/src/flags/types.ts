/**
 * Flag keys follow the registry in FLAGS.md: snake_case `module_<name>`.
 * This union is the single typed vocabulary of flags the frontend may check.
 * Keep it in sync with FLAGS.md (the authoritative registry).
 */
export type FlagKey =
  | "module_iam"
  | "module_gateway"
  | "module_students"
  | "module_config"
  | "module_finance"
  | "module_attendance"
  | "module_grades"
  | "module_scholarships"
  | "module_docs"
  | "module_comms"
  | "module_billing"
  | "module_teachers"
  | "module_uniforms"
  | "module_reports"
  | "module_student_card";

/** Map of flag key -> enabled. Absent or false means OFF (flags default OFF). */
export type FlagSet = Partial<Record<FlagKey, boolean>>;
