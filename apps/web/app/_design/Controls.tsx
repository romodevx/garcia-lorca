"use client";

import type { CSSProperties, MouseEvent, ReactElement, ReactNode } from "react";
import { Icon, type IconName } from "./Icon";
import type { Theme } from "../theme/theme-script";

export function ThemeToggle({ theme, onToggle }: { theme: Theme; onToggle: () => void }): ReactElement {
  const dark = theme === "dark";
  return (
    <button
      onClick={onToggle}
      aria-label="Cambiar tema"
      title={dark ? "Modo claro" : "Modo oscuro"}
      style={{
        width: 46,
        height: 46,
        borderRadius: "50%",
        display: "grid",
        placeItems: "center",
        background: "var(--surface-3)",
        color: "var(--text)",
        border: "1px solid var(--border)",
        transition: "all .2s var(--ease)"
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.background = "var(--surface-2)";
        e.currentTarget.style.color = "var(--forest)";
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.background = "var(--surface-3)";
        e.currentTarget.style.color = "var(--text)";
      }}
    >
      <Icon name={dark ? "sun" : "moon"} size={22} />
    </button>
  );
}

export type BtnKind = "primary" | "forest" | "ghost" | "outline";

export interface BtnProps {
  children: ReactNode;
  kind?: BtnKind;
  icon?: IconName;
  onClick?: (e: MouseEvent<HTMLButtonElement>) => void;
  full?: boolean;
  danger?: boolean;
  disabled?: boolean;
  type?: "button" | "submit";
  style?: CSSProperties;
}

export function Btn({
  children,
  kind = "primary",
  icon,
  onClick,
  full,
  danger,
  disabled,
  type = "button",
  style = {}
}: BtnProps): ReactElement {
  const base: CSSProperties = {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    gap: 9,
    height: 50,
    padding: "0 22px",
    borderRadius: "var(--r-pill)",
    fontFamily: "var(--font-display)",
    fontWeight: 600,
    fontSize: 16,
    width: full ? "100%" : "auto",
    transition: "all .18s var(--ease)",
    whiteSpace: "nowrap",
    cursor: disabled ? "not-allowed" : "pointer",
    opacity: disabled ? 0.6 : 1,
    ...style
  };
  const kinds: Record<BtnKind, CSSProperties> = {
    primary: {
      background: "var(--primary)",
      color: "var(--primary-ink)",
      boxShadow: "0 4px 14px color-mix(in oklab, var(--lime) 40%, transparent)"
    },
    forest: { background: "var(--forest)", color: "#fff" },
    ghost: { background: "var(--surface-3)", color: "var(--text)", border: "1px solid var(--border)" },
    outline: { background: "transparent", color: "var(--text)", border: "1.5px solid var(--border-strong)" }
  };
  const k: CSSProperties = danger ? { background: "var(--red)", color: "#fff" } : kinds[kind];
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      style={{ ...base, ...k }}
      onMouseEnter={(e) => {
        if (disabled) return;
        e.currentTarget.style.transform = "translateY(-1px)";
        e.currentTarget.style.filter = "brightness(1.04)";
      }}
      onMouseLeave={(e) => {
        if (disabled) return;
        e.currentTarget.style.transform = "none";
        e.currentTarget.style.filter = "none";
      }}
    >
      {icon && <Icon name={icon} size={20} />}
      {children}
    </button>
  );
}

export function IconBtn({
  name,
  onClick,
  badge,
  label,
  size = 46
}: {
  name: IconName;
  onClick?: (e: MouseEvent<HTMLButtonElement>) => void;
  badge?: number;
  label: string;
  size?: number;
}): ReactElement {
  return (
    <button
      onClick={onClick}
      aria-label={label}
      title={label}
      style={{
        position: "relative",
        width: size,
        height: size,
        borderRadius: "50%",
        display: "grid",
        placeItems: "center",
        background: "var(--surface-3)",
        color: "var(--text)",
        border: "1px solid var(--border)",
        transition: "all .2s var(--ease)"
      }}
      onMouseEnter={(e) => {
        e.currentTarget.style.color = "var(--forest)";
        e.currentTarget.style.background = "var(--surface-2)";
      }}
      onMouseLeave={(e) => {
        e.currentTarget.style.color = "var(--text)";
        e.currentTarget.style.background = "var(--surface-3)";
      }}
    >
      <Icon name={name} size={22} />
      {badge !== undefined && badge > 0 && (
        <span
          style={{
            position: "absolute",
            top: 6,
            right: 6,
            minWidth: 18,
            height: 18,
            padding: "0 4px",
            borderRadius: 99,
            background: "var(--red)",
            color: "#fff",
            fontSize: 11,
            fontWeight: 700,
            display: "grid",
            placeItems: "center",
            boxShadow: "0 0 0 2px var(--surface)"
          }}
        >
          {badge}
        </span>
      )}
    </button>
  );
}
