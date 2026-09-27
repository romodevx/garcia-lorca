"use client";

import type { ReactElement } from "react";

import { progressLabel } from "./format";

const PERCENT = 100;

/** Captured vs expected grades, as a bar and in words. */
export function ProgressBar({ captured, expected }: { captured: number; expected: number }): ReactElement {
  const ratio = expected > 0 ? Math.min(captured / expected, 1) : 0;
  const complete = expected > 0 && captured >= expected;
  return (
    <div style={{ display: "grid", gap: 6 }}>
      <div
        role="progressbar"
        aria-label="Calificaciones capturadas"
        aria-valuemin={0}
        aria-valuemax={expected}
        aria-valuenow={captured}
        style={{ height: 8, borderRadius: "var(--r-pill)", background: "var(--surface-3)", overflow: "hidden" }}
      >
        <div style={{ width: `${ratio * PERCENT}%`, height: "100%", background: complete ? "var(--forest)" : "var(--yellow)" }} />
      </div>
      <span style={{ fontSize: 13.5, color: "var(--text-muted)", fontWeight: 600 }}>{progressLabel(captured, expected)}</span>
    </div>
  );
}
