"use client";

import { useTranslations } from "next-intl";
import type { ReactElement } from "react";
import { Avatar, Wordmark } from "./Brand";
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
  search: string;
  onSearch: (value: string) => void;
}

export function TopBar({
  user,
  theme,
  onToggle,
  onHome,
  onLogout,
  search,
  onSearch
}: TopBarProps): ReactElement {
  const t = useTranslations("common");
  return (
    <header
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
      <button onClick={onHome} style={{ display: "flex", alignItems: "center" }} aria-label={t("nav.dashboard")}>
        <Wordmark size={42} />
      </button>
      <div style={{ flex: 1, maxWidth: 520, marginLeft: 8, position: "relative" }}>
        <span
          style={{
            position: "absolute",
            left: 18,
            top: "50%",
            transform: "translateY(-50%)",
            color: "var(--text-faint)",
            pointerEvents: "none"
          }}
        >
          <Icon name="search" size={20} />
        </span>
        <input
          value={search}
          onChange={(e) => onSearch(e.target.value)}
          placeholder={t("search.placeholder")}
          aria-label={t("action.search")}
          style={{
            width: "100%",
            height: 50,
            paddingLeft: 48,
            paddingRight: 18,
            borderRadius: "var(--r-pill)",
            border: "1px solid var(--border)",
            background: "var(--surface)",
            color: "var(--text)",
            fontSize: 15.5,
            outline: "none"
          }}
        />
      </div>
      <div style={{ flex: 1 }} />
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <IconBtn name="bell" badge={3} label={t("action.notifications")} />
        <LanguageToggle />
        <ThemeToggle theme={theme} onToggle={onToggle} />
        <button
          onClick={onLogout}
          aria-label="Cerrar sesión"
          title="Cerrar sesión"
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
          <div style={{ textAlign: "right", lineHeight: 1.15 }}>
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
