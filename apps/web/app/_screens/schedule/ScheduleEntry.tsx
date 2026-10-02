"use client";

import type { ReactElement } from "react";

import { MODULES, type ModuleDef } from "../../_design/data";
import { ModulePlaceholder } from "../ModulePlaceholder";
import { ScheduleScreen } from "./ScheduleScreen";

export interface ScheduleEntryProps {
  /** module_schedule (and module_config) are on. UX only: the gateway answers 404 otherwise. */
  enabled: boolean;
  permissions: readonly string[] | undefined;
  onBack: () => void;
}

/**
 * Which Horarios a person gets (T16). schedule:read reads the whole school's
 * timetable (schedule:write edits it); a teacher with only teaching:read gets
 * their own week, read-only. Flag off: the friendly stub, never the surface.
 */
export function ScheduleEntry({ enabled, permissions, onBack }: ScheduleEntryProps): ReactElement {
  if (!enabled) return <ModulePlaceholder m={MODULES.find((m) => m.id === "schedule") as ModuleDef} onBack={onBack} />;
  const perms = permissions ?? [];
  const manages = perms.includes("schedule:read");
  return (
    <ScheduleScreen
      onBack={onBack}
      mode={manages ? "manage" : "teacher"}
      canWrite={manages && perms.includes("schedule:write")}
    />
  );
}
