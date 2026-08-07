"use client";

import type { ReactElement, ReactNode } from "react";
import { Icon } from "./Icon";

/**
 * A bottom-centred confirmation toast (`role="status"`), shared by every module
 * surface so a saved action always announces itself the same way.
 */
export function Toast({ children }: { children: ReactNode }): ReactElement {
  return (
    <div
      role="status"
      style={{
        position: "fixed",
        bottom: 28,
        left: "50%",
        transform: "translateX(-50%)",
        zIndex: 60,
        display: "flex",
        alignItems: "center",
        gap: 11,
        padding: "14px 22px",
        borderRadius: "var(--r-pill)",
        background: "var(--forest)",
        color: "#fff",
        fontWeight: 600,
        fontSize: 15,
        boxShadow: "var(--shadow-pop)",
        animation: "pop .25s var(--ease) both",
      }}
    >
      <Icon name="check" size={20} stroke={2.6} />
      {children}
    </div>
  );
}
