"use client";

import { useState, type ReactElement } from "react";

import { DrawerShell } from "../../_design/DrawerShell";
import { CorrectPaymentDrawer } from "../../_design/payments/CorrectPaymentDrawer";
import type { Deposit } from "../../finance/api";
import { DateField } from "../../_design/form";
import { localISODate } from "../../_lib/localDate";
import type { CutAccess } from "./CashCutClosing";
import { CashUpPanel } from "./CashUpPanel";

export interface CashUpDrawerProps {
  /** The office's day (aaaa-mm-dd); injected by tests. Defaults to today, locally. */
  today?: string;
  /** May correct a registered payment (payments:edit, T9). UX only: the gateway decides. */
  canCorrect?: boolean;
  /** T10: expenses and closing the day. Absent: module_expenses is off. */
  cutAccess?: CutAccess;
  onClose: () => void;
}

/**
 * The day's cut, in a drawer: Finanzas and the counter (Registro de pagos)
 * open the same one. Each opening starts on today — opening the cut is
 * closing the day — and any past day can be chosen, since a payment captured
 * late is dated the day the family paid and lands on that day's cut (task 7).
 */
export function CashUpDrawer({ today, canCorrect = false, cutAccess, onClose }: CashUpDrawerProps): ReactElement {
  const cutToday = today ?? localISODate();
  const [cutDay, setCutDay] = useState(cutToday);
  // The receipt being corrected (T9): one dialog at a time, so the cut steps
  // aside and comes back re-read (a fresh panel reads the day again).
  const [correcting, setCorrecting] = useState<Deposit | undefined>(undefined);
  const [reads, setReads] = useState(0);
  if (correcting !== undefined) {
    return (
      <CorrectPaymentDrawer
        depositId={correcting.id}
        studentId={correcting.studentId}
        today={cutToday}
        onClose={() => { setCorrecting(undefined); setReads((n) => n + 1); }}
      />
    );
  }
  return (
    <DrawerShell
      title="Corte de caja"
      subtitle={cutDay === cutToday ? "Lo recibido hoy, por forma de pago" : "Lo recibido ese día, por forma de pago"}
      icon="finance"
      onClose={onClose}
    >
      <DateField label="Día del corte" value={cutDay} max={cutToday} onChange={(v) => v !== "" && setCutDay(v)} />
      {/* Today is asked as "" so the server keeps its own day, as before. */}
      <CashUpPanel key={reads} day={cutDay === cutToday ? "" : cutDay} onCorrect={canCorrect ? setCorrecting : undefined} access={cutAccess} />
    </DrawerShell>
  );
}
