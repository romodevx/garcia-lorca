"use client";

import { useFlags } from "@garcia-lorca/ui/flags";
import { useTranslations } from "next-intl";
import type { CSSProperties, ReactElement, ReactNode } from "react";

import type { ModuleDef } from "../_design/data";
import { groupHomeModules } from "./areas";
import { HomeCard } from "./HomeCard";
import { homeShortcuts } from "./shortcuts";
import { usePeek } from "./usePeek";

/** Up to this many modules, areas would be one-card rows: one plain list instead (B12). */
const SINGLE_LIST_MAX = 4;

/** Sections enter one after another (60 ms apart), never card by card (review D15). */
type AreaVars = CSSProperties & { "--area-i": number };

function Area({ id, title, index, compact, children }: { id: string; title: string; index: number; compact?: boolean; children: ReactNode }): ReactElement {
  const headingId = `home-area-${id}`;
  const vars: AreaVars = { "--area-i": index };
  return (
    <section aria-labelledby={headingId} className={compact === true ? "home-area home-area--compact" : "home-area"} style={vars}>
      <h2 id={headingId} className="home-area__title">
        {title}
      </h2>
      <ul className="home-grid">{children}</ul>
    </section>
  );
}

export interface HomeModulesProps {
  modules: readonly ModuleDef[];
  /** The caller's permissions: a shortcut's `requires` is checked against them (E4). */
  permissions: readonly string[];
  onOpen: (id: string, intent?: string) => void;
}

/** The visible modules, by area (§3.1); with four or fewer, one "Tus módulos" list. */
export function HomeModules({ modules, permissions, onOpen }: HomeModulesProps): ReactElement {
  const t = useTranslations("home");
  const flags = useFlags();
  const peek = usePeek();
  const card = (m: ModuleDef): ReactElement => (
    <HomeCard
      key={m.id}
      module={m}
      shortcuts={homeShortcuts(m.id, permissions, flags)}
      peekOpen={peek.openId === m.id}
      onTogglePeek={() => peek.toggle(m.id)}
      onClosePeek={peek.close}
      onOpen={onOpen}
    />
  );

  if (modules.length <= SINGLE_LIST_MAX) {
    return (
      <Area id="yours" title={t("heading.yours")} index={0}>
        {modules.map(card)}
      </Area>
    );
  }
  return (
    <>
      {groupHomeModules(modules).map(({ area, modules: inArea }, i) => (
        <Area key={area.id} id={area.id} title={t(`group.${area.id}`)} index={i} compact={area.id === "system"}>
          {inArea.map(card)}
        </Area>
      ))}
    </>
  );
}

/** Placeholder rows per area while `me()` resolves (B8), so nothing reflows when it does. */
const SKELETON_AREAS = [3, 4, 3] as const;

export function HomeModulesSkeleton(): ReactElement {
  return (
    <div role="status" aria-label="Cargando módulos" className="home-skeleton">
      {SKELETON_AREAS.map((cards, i) => (
        <div key={i} aria-hidden className="home-skeleton__area">
          <span className="home-skeleton__title" />
          <div className="home-grid">
            {Array.from({ length: cards }, (_, j) => (
              <span key={j} className="home-skeleton__card" />
            ))}
          </div>
        </div>
      ))}
    </div>
  );
}
