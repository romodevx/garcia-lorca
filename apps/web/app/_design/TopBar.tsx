"use client";

import { useTranslations } from "next-intl";
import type { ReactElement } from "react";
import { Avatar, Logo } from "./Brand";
import { IconBtn, ThemeToggle } from "./Controls";
import { Icon } from "./Icon";
import type { UserProfile } from "./data";
import { LanguageToggle } from "../i18n/LanguageToggle";
import type { Theme } from "../theme/theme-script";

export interface TopBarProps {
  user: UserProfile;
  theme: Theme;
  onToggle: () => void;
  onHome: () => void;
  onLogout: () => void;
  /** Opens the Command Palette (the search box is a trigger, not an input). */
  onOpenPalette: () => void;
}

export function TopBar({
  user,
  theme,
  onToggle,
  onHome,
  onLogout,
  onOpenPalette
}: TopBarProps): ReactElement {
  const t = useTranslations("common");
  const tp = useTranslations("palette");
  return (
    // Every style of the bar lives in globals.css (.topbar*): an inline style
    // would beat the look layer under html[data-look], and the 768px block.
    <header className="topbar">
      <button onClick={onHome} className="topbar-logo" aria-label={t("nav.dashboard")}>
        <Logo height={40} />
      </button>
      {/*
        The search box is a TRIGGER for the Command Palette (Cmd+K), not an
        input: click or ⌘K opens the omnidirectional navigate+execute palette.
      */}
      {/*
        Width lives in globals.css, not here. An inline style beats an author
        rule without !important, so while these three were inline the phone
        media query could not shrink the search box — it silently kept its
        desktop `flex: 1; max-width: 520px` and squeezed the placeholder to
        zero. How wide this is at a given viewport is the stylesheet's call.
      */}
      <div className="topbar-search">
        <button
          type="button"
          onClick={onOpenPalette}
          aria-label={tp("ui.placeholder")}
          className="topbar-search-btn"
        >
          <Icon name="search" size={20} />
          <span className="topbar-search-text">{tp("ui.placeholder")}</span>
          <kbd className="topbar-search-kbd">⌘K</kbd>
        </button>
      </div>
      <div className="topbar-spacer" />
      <div className="topbar-actions">
        {/* No count until something feeds it: a fixed number announced
            notices an empty school did not have (QA 2026-09-25, ISSUE-001). */}
        <IconBtn name="bell" label={t("action.notifications")} />
        {/* Wrapped so the stylesheet can drop it on a phone: it is the one
            control here that a person changes once and then never again. */}
        <span className="topbar-lang">
          <LanguageToggle />
        </span>
        {/* Wrapped for the same reason: the refreshed look drops it below
            768px (PLAN-redesign-home §4.2), so the capsule fits 320px. */}
        <span className="topbar-theme">
          <ThemeToggle theme={theme} onToggle={onToggle} />
        </span>
        <button
          onClick={onLogout}
          aria-label="Cerrar sesión"
          title="Cerrar sesión"
          className="topbar-user"
        >
          <div className="topbar-user-text">
            <div className="topbar-user-name">{user.name}</div>
            <div className="topbar-user-role">{user.role}</div>
          </div>
          <Avatar name={user.name} color={user.color} size={40} />
        </button>
      </div>
    </header>
  );
}
