"use client";

import { useTranslations } from "next-intl";
import { useId, useRef, type CSSProperties, type KeyboardEvent, type ReactElement } from "react";

import { Icon } from "../_design/Icon";
import type { ModuleDef } from "../_design/data";
import type { HomeShortcut } from "./shortcuts";
import { PEEK_OWNER_ATTR, moveFocusAmongShortcuts } from "./usePeek";

type Open = (id: string, intent?: string) => void;

/** How many shortcut rows the card reserves room for (globals.css sizes it in rem). */
type CardVars = CSSProperties & { "--peek-rows": number };

export interface HomeCardProps {
  module: ModuleDef;
  /** Shortcuts that go somewhere the card does not (D10); none ⇒ no ⋯, no peek. */
  shortcuts: readonly HomeShortcut[];
  peekOpen: boolean;
  onTogglePeek: () => void;
  onClosePeek: () => void;
  onOpen: Open;
}

/**
 * A module card of the refreshed home (PLAN-redesign-home §3.2). The main
 * button stretches over the whole card (globals.css, .home-card__main::after),
 * so a click anywhere opens the module; the description sits outside it and
 * describes it. The ⋯ discloses the shortcuts: two Tab stops per card, the
 * peek `hidden` while closed, Esc closes it and returns to the ⋯. The hover
 * peek is CSS only and lifts that `hidden` for a mouse, so a closed peek also
 * carries aria-hidden and tabIndex=-1 on its shortcuts: the preview stays mouse
 * decoration, out of Tab and of the accessibility tree, yet clickable (`inert`
 * would block the click too) (review I2).
 */
export function HomeCard({ module: m, shortcuts, peekOpen, onTogglePeek, onClosePeek, onOpen }: HomeCardProps): ReactElement {
  const t = useTranslations("home");
  const tp = useTranslations("palette");
  const descId = useId();
  const peekId = useId();
  const moreRef = useRef<HTMLButtonElement>(null);
  const hasPeek = shortcuts.length > 0;
  const vars: CardVars = { "--peek-rows": shortcuts.length };

  const closeOnEscape = (event: KeyboardEvent<HTMLLIElement>): void => {
    if (event.key !== "Escape" || !peekOpen) return;
    event.stopPropagation();
    onClosePeek();
    moreRef.current?.focus();
  };

  return (
    <li
      className={hasPeek ? "home-card home-card--peek" : "home-card"}
      data-group={m.group}
      data-module={m.id}
      {...(hasPeek ? { [PEEK_OWNER_ATTR]: m.id } : {})}
      style={hasPeek ? vars : undefined}
      onKeyDown={hasPeek ? closeOnEscape : undefined}
    >
      <div className="home-card__head">
        <span className="home-card__icon">
          <Icon name={m.icon} size={22} />
        </span>
        <button type="button" className="home-card__main" aria-describedby={descId} onClick={() => onOpen(m.id)}>
          {m.label}
        </button>
      </div>
      {hasPeek && (
        <button
          ref={moreRef}
          type="button"
          className="home-card__more"
          aria-expanded={peekOpen}
          aria-controls={peekId}
          aria-label={t("more.label", { module: m.label })}
          onClick={onTogglePeek}
        >
          <Icon name="more" size={20} />
        </button>
      )}
      <div className="home-card__body">
        <p id={descId} className="home-card__desc">
          {t(`desc.${m.id}`)}
        </p>
        {hasPeek && (
          <ul
            id={peekId}
            className="home-card__peek"
            hidden={!peekOpen}
            aria-hidden={peekOpen ? undefined : true}
            onKeyDown={moveFocusAmongShortcuts}
          >
            {shortcuts.map((s) => (
              <li key={s.intent}>
                <button
                  type="button"
                  className="home-card__shortcut"
                  tabIndex={peekOpen ? undefined : -1}
                  onClick={() => onOpen(s.moduleId, s.intent)}
                >
                  {tp(s.titleKey)} <span aria-hidden>→</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </li>
  );
}
