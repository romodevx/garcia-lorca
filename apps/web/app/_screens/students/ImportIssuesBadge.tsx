"use client";

import type { ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import { issuesLabel } from "../../students/importIssues";

/** ⚠ and the count of a pupil's open import issues; its name says it in words. */
export function ImportIssuesBadge({ count }: { count: number }): ReactElement {
  return (
    <span
      role="img"
      aria-label={issuesLabel(count)}
      title={issuesLabel(count)}
      style={{ display: "inline-flex", alignItems: "center", gap: 4, flexShrink: 0, padding: "2px 8px", borderRadius: "var(--r-pill)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 12.5, fontWeight: 700 }}
    >
      <Icon name="warning" size={13} stroke={2.4} />
      {count}
    </span>
  );
}
