"use client";

import { useTranslations } from "next-intl";
import { useId, type ReactElement } from "react";

import { Icon } from "../_design/Icon";
import type { ModuleDef } from "../_design/data";

export interface HomeCardProps {
  module: ModuleDef;
  onOpen: (id: string, intent?: string) => void;
}

/**
 * A module card of the refreshed home (PLAN-redesign-home §3.2). The main
 * button stretches over the whole card (globals.css, .home-card__main::after),
 * so a click anywhere opens the module; the description is outside it and
 * describes it. Hover and focus live in CSS, never in state.
 */
export function HomeCard({ module: m, onOpen }: HomeCardProps): ReactElement {
  const t = useTranslations("home");
  const descId = useId();
  return (
    <li className="home-card" data-group={m.group} data-module={m.id}>
      <div className="home-card__head">
        <span className="home-card__icon">
          <Icon name={m.icon} size={22} />
        </span>
        <button type="button" className="home-card__main" aria-describedby={descId} onClick={() => onOpen(m.id)}>
          {m.label}
        </button>
      </div>
      <div className="home-card__body">
        <p id={descId} className="home-card__desc">
          {t(`desc.${m.id}`)}
        </p>
      </div>
    </li>
  );
}
