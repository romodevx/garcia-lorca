"use client";

import { useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { Group } from "../../config/api";
import type { Enrollment, TransferInput } from "../../students/api";
import { SelectField, TextField, type SelectOption } from "../../_design/form";

export interface EnrollmentRowProps {
  enrollment: Enrollment;
  studentName: string;
  /** Groups of the enrollment's cycle (transfer targets, current group excluded). */
  groups: readonly Group[];
  onTransfer: (input: TransferInput) => Promise<void>;
  onWithdraw: (enrollmentId: string) => Promise<void>;
}

const STATUS_LABEL: Record<Enrollment["status"], string> = {
  active: "Activo",
  transferred: "Trasladado",
  withdrawn: "Baja",
};

/**
 * One roster row: the enrolled student, their status, and — while the enrollment
 * is active — an inline transfer form (a group in the SAME cycle + a required
 * reason) and a withdraw action. Transfer targets exclude the current group, so a
 * no-op move is impossible; the gateway enforces the same-cycle rule.
 */
export function EnrollmentRow({ enrollment, studentName, groups, onTransfer, onWithdraw }: EnrollmentRowProps): ReactElement {
  const [toGroupId, setToGroupId] = useState("");
  const [reason, setReason] = useState("");
  const [pending, setPending] = useState(false);

  const active = enrollment.status === "active";
  const targets: SelectOption[] = groups
    .filter((g) => g.id !== enrollment.groupId)
    .map((g) => ({ value: g.id, label: g.name }));

  const canTransfer = toGroupId !== "" && reason.trim() !== "" && !pending;

  const submitTransfer = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    setPending(true);
    try {
      await onTransfer({ enrollmentId: enrollment.id, toGroupId, reason: reason.trim() });
      setToGroupId("");
      setReason("");
    } finally {
      setPending(false);
    }
  };

  const withdraw = async (): Promise<void> => {
    setPending(true);
    try {
      await onWithdraw(enrollment.id);
    } finally {
      setPending(false);
    }
  };

  return (
    <li style={{ padding: "14px 16px", borderRadius: "var(--r-lg)", background: "var(--surface-2)", display: "grid", gap: 12 }}>
      <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 12 }}>
        <span style={{ fontWeight: 700, fontSize: 15, color: "var(--text)" }}>{studentName}</span>
        <span style={{ fontSize: 12.5, fontWeight: 600, color: "var(--text-muted)", padding: "3px 10px", borderRadius: "var(--r-pill)", background: "var(--surface-3)" }}>
          {STATUS_LABEL[enrollment.status]}
        </span>
      </div>

      {active && (
        <form aria-label="Trasladar alumno" onSubmit={submitTransfer} style={{ display: "grid", gap: 10 }}>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 10 }}>
            <SelectField
              label="Nuevo grupo"
              value={toGroupId}
              options={targets}
              onChange={setToGroupId}
              placeholder="Selecciona un grupo…"
              disabled={targets.length === 0}
            />
            <TextField label="Motivo del traslado" value={reason} onChange={setReason} placeholder="Motivo" />
          </div>
          <div style={{ display: "flex", gap: 10 }}>
            <Btn type="submit" kind="outline" icon="chevron" disabled={!canTransfer}>
              Trasladar
            </Btn>
            <Btn type="button" danger icon="close" disabled={pending} onClick={withdraw}>
              Dar de baja
            </Btn>
          </div>
        </form>
      )}
    </li>
  );
}
