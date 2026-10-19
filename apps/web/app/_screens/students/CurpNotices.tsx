"use client";

import type { ReactElement } from "react";

import { curpNotices } from "../../students/curpCheck";
import type { Sex } from "../../students/api";

/**
 * CC-314: what the CURP says against itself and the file, shown under the
 * field as a warning the person can act on — never a block, so the save stays
 * available (the legacy import brings imperfect CURPs the school still has to
 * register). The live region is always mounted so a screen reader announces a
 * warning the moment it appears; it is a bare aria-live region, not a
 * role="status", because the page's own save confirmation is THE status.
 */
export function CurpNotices({ curp, birthDate, sex = "" }: { curp: string; birthDate: string; sex?: Sex }): ReactElement {
  const notices = curpNotices(curp, birthDate, sex);
  return (
    <div aria-live="polite" style={{ display: "grid", gap: 6, marginTop: notices.length > 0 ? -6 : 0 }}>
      {notices.map((notice) => (
        <p key={notice} style={{ margin: 0, padding: "8px 12px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14, fontWeight: 600, lineHeight: 1.45 }}>
          {notice}
        </p>
      ))}
    </div>
  );
}
