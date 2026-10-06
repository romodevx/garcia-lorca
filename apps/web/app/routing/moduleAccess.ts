import { MODULES, isModuleVisible } from "../_design/data";
import type { View } from "./routes";

/** The panel module each screen belongs to; the public and home screens belong to none. */
const MODULE_OF_SCREEN: Partial<Record<View["screen"], string>> = {
  students: "students",
  studentEdit: "students",
  finance: "finance",
  expenses: "expenses",
  schedule: "schedule",
  activities: "activities",
  scholarships: "scholarships",
  payments: "payments",
  announcements: "announcements",
  config: "settings",
  levelDelete: "settings",
  levelPeriods: "settings",
  studentImport: "settings",
  grades: "grades",
  attendance: "attendance",
  teachers: "teachers",
  teacherLoad: "teachers",
  uniforms: "uniforms",
  admin: "admin",
};

/** The id (in MODULES) of the module a view shows, or undefined for dashboard/login/changePassword. */
export function moduleIdOfView(view: View): string | undefined {
  if (view.screen === "module") return view.moduleId;
  return MODULE_OF_SCREEN[view.screen];
}

/**
 * Whether the principal may land on this view by typing its address: the same
 * rule the panel uses to show the module's tile. Unresolved permissions and
 * unknown module ids redirect nobody. UX only — the gateway's 403/404 is the
 * boundary; this just keeps a teacher from staring at an empty admin shell.
 */
export function canReachView(view: View, permissions: readonly string[] | undefined): boolean {
  if (permissions === undefined) return true;
  const moduleId = moduleIdOfView(view);
  if (moduleId === undefined) return true;
  const module = MODULES.find((m) => m.id === moduleId);
  return module === undefined || isModuleVisible(module, permissions);
}
