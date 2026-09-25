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
    <header
      className="topbar"
      style={{
        position: "sticky",
        top: 0,
        zIndex: 30,
        height: 78,
        display: "flex",
        alignItems: "center",
        gap: 20,
        padding: "0 28px",
        background: "var(--topbar-bg)",
        backdropFilter: "blur(18px) saturate(160%)",
        WebkitBackdropFilter: "blur(18px) saturate(160%)",
        borderBottom: "1px solid var(--border)"
      }}
    >
      <button onClick={onHome} className="topbar-logo" style={{ display: "flex", alignItems: "center" }} aria-label={t("nav.dashboard")}>
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
          style={{
            display: "flex",
            alignItems: "center",
            gap: 12,
            width: "100%",
            height: 50,
            padding: "0 14px 0 18px",
            borderRadius: "var(--r-pill)",
            border: "1px solid var(--border)",
            background: "var(--surface)",
            color: "var(--text-faint)",
            fontSize: 15.5,
            cursor: "text",
            textAlign: "left"
          }}
        >
          <Icon name="search" size={20} />
          <span className="topbar-search-text" style={{ flex: 1 }}>{tp("ui.placeholder")}</span>
          <kbd
            className="topbar-search-kbd"
            style={{
              fontFamily: "var(--font-ui)",
              fontSize: 12,
              fontWeight: 600,
              color: "var(--text-muted)",
              background: "var(--surface-3)",
              border: "1px solid var(--border)",
              borderRadius: 7,
              padding: "3px 7px"
            }}
          >
            ⌘K
          </kbd>
        </button>
      </div>
      <div className="topbar-spacer" style={{ flex: 1 }} />
      <div className="topbar-actions" style={{ display: "flex", alignItems: "center", gap: 12 }}>
        {/* No count until something feeds it: a fixed number announced
            notices an empty school did not have (QA 2026-09-25, ISSUE-001). */}
        <IconBtn name="bell" label={t("action.notifications")} />
        {/* Wrapped so the stylesheet can drop it on a phone: it is the one
            control here that a person changes once and then never again. */}
        <span className="topbar-lang">
          <LanguageToggle />
        </span>
        <ThemeToggle theme={theme} onToggle={onToggle} />
        <button
          onClick={onLogout}
          aria-label="Cerrar sesión"
          title="Cerrar sesión"
          className="topbar-user"
          style={{
            display: "flex",
            alignItems: "center",
            gap: 10,
            padding: "5px 6px 5px 14px",
            borderRadius: "var(--r-pill)",
            background: "var(--surface-3)",
            border: "1px solid var(--border)"
          }}
        >
          <div className="topbar-user-text" style={{ textAlign: "right", lineHeight: 1.15 }}>
            <div
              style={{
                fontWeight: 700,
                fontSize: 14,
                color: "var(--text)",
                fontFamily: "var(--font-display)"
              }}
            >
              {user.name}
            </div>
            <div style={{ fontSize: 12, color: "var(--text-muted)" }}>{user.role}</div>
          </div>
          <Avatar name={user.name} color={user.color} size={40} />
        </button>
      </div>
    </header>
  );
}
