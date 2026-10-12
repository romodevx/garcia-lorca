import type { ColorKey } from "../_design/colors";
import type { ModuleDef, ModuleGroup } from "../_design/data";

/**
 * The refreshed home's areas (PLAN-redesign-home §3.1), in the order they are
 * drawn. `order` is daily use, most used first; Sistema closes the page.
 *
 * Each module names its own area (`ModuleDef.group`, required); `order` only
 * ranks inside the area. areas.test.ts keeps the two from disagreeing.
 */
export interface HomeArea {
  readonly id: ModuleGroup;
  readonly color: ColorKey;
  readonly order: readonly string[];
}

export const HOME_AREAS: readonly HomeArea[] = [
  { id: "people", color: "forest", order: ["students", "announcements", "activities", "documents"] },
  { id: "academic", color: "purple", order: ["attendance", "grades", "schedule", "teachers"] },
  { id: "money", color: "orange", order: ["payments", "finance", "scholarships", "uniforms", "expenses", "billing"] },
  { id: "system", color: "neutral", order: ["settings", "admin", "reports"] },
];

export interface GroupedArea {
  readonly area: HomeArea;
  readonly modules: readonly ModuleDef[];
}

/** The visible modules, by area and daily use. An area with none is left out. */
export function groupHomeModules(visible: readonly ModuleDef[]): GroupedArea[] {
  const byId = new Map(visible.map((m) => [m.id, m]));
  return HOME_AREAS.map((area) => ({
    area,
    modules: area.order.flatMap((id) => {
      const m = byId.get(id);
      return m !== undefined && m.group === area.id ? [m] : [];
    }),
  })).filter((g) => g.modules.length > 0);
}
