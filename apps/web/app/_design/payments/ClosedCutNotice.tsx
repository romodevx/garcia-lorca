"use client";

import type { ReactElement } from "react";

import { shortDayLabel } from "../../_lib/localDate";

/**
 * Said before money is taken on a day whose cash cut is already closed
 * (ISSUE-026): collecting is allowed, and this is what it does to the cut.
 * Shared by every screen that enters the cut — the counter, Finanzas'
 * "Registrar pago", the uniform till.
 *
 * The `role="status"` region is ALWAYS mounted and only its text changes: a
 * live region announces a change only when it was already in the page.
 * `day` is "" for today.
 */
export function ClosedCutNotice({ closed, day }: { closed: boolean; day: string }): ReactElement {
  const which = day === "" ? "El corte de hoy" : `El corte del ${shortDayLabel(day)}`;
  return (
    <p
      role="status"
      data-testid="closed-cut-status"
      style={
        closed
          ? { margin: 0, padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14, fontWeight: 600 }
          : { margin: 0 }
      }
    >
      {closed ? `${which} ya se cerró. Este cobro lo dejará marcado como modificado después del cierre.` : ""}
    </p>
  );
}
