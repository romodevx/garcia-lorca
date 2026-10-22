"use client";

import type { ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { formatStudentName } from "../../_design/students/studentName";
import type { PaymentVoucher } from "../../finance/vouchers";
import { formatCents } from "../../_lib/money";
import { shortDayLabel } from "../../_lib/localDate";
import { schoolISODate } from "../../_lib/schoolDay";
import type { Student } from "../../students/api";
import type { VoucherQueue as Queue } from "./useVoucherQueue";

export interface VoucherQueueProps {
  queue: Queue;
  /** The roll the screen already read, by id: the queue carries no names. */
  rollById: ReadonlyMap<string, Student>;
  roomOf: (s: Student) => string;
  onOpen: (voucher: PaymentVoucher, student: Student) => void;
}

const TITLE_ID = "voucher-queue-title";

/** "Pagó el 20 oct" from what the family said, or the day it was sent. */
function whenLabel(v: PaymentVoucher): string {
  return v.claimedPaidOn !== undefined
    ? `Pagó el ${shortDayLabel(v.claimedPaidOn)}`
    : `Enviado el ${shortDayLabel(schoolISODate(new Date(v.createdAt)))}`;
}

/**
 * "Comprobantes por revisar": what families sent from the app, oldest first,
 * above the search (ADR 0040). The front desk clears it before anything else.
 * Nothing at all while none is waiting.
 */
export function VoucherQueue({ queue, rollById, roomOf, onOpen }: VoucherQueueProps): ReactElement | null {
  if (queue.status === "failed") {
    return (
      <div role="alert" style={{ display: "flex", alignItems: "center", gap: 12, padding: "14px 18px", marginBottom: 18, borderRadius: "var(--r-lg)", background: "var(--warn-bg)", border: "1px solid var(--border)" }}>
        <p style={{ flex: 1, color: "var(--warn-fg)", fontSize: 14.5, margin: 0 }}>No pudimos leer los comprobantes que enviaron las familias.</p>
        <Btn kind="ghost" onClick={queue.reload} aria-label="Reintentar comprobantes">Reintentar</Btn>
      </div>
    );
  }
  if (queue.status !== "ready" || queue.vouchers.length === 0) {
    return null;
  }
  return (
    <section aria-labelledby={TITLE_ID} className="fade-up" style={{ marginBottom: 22 }}>
      <h2 id={TITLE_ID} style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 17, fontWeight: 800, color: "var(--text)", marginBottom: 10 }}>
        <Icon name="documents" size={19} />
        {`Comprobantes por revisar (${queue.vouchers.length})`}
      </h2>
      <div style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", overflow: "hidden" }}>
        {queue.vouchers.map((v) => {
          const pupil = rollById.get(v.studentId);
          return (
            <VoucherRow key={v.id} voucher={v} name={pupil === undefined ? undefined : formatStudentName(pupil)} room={pupil === undefined ? "" : roomOf(pupil)}
              onOpen={pupil === undefined ? undefined : () => onOpen(v, pupil)} />
          );
        })}
      </div>
    </section>
  );
}

interface VoucherRowProps {
  voucher: PaymentVoucher;
  /** Undefined when the pupil is not on the roll (withdrawn): nothing to open. */
  name: string | undefined;
  room: string;
  onOpen: (() => void) | undefined;
}

const rowStyle = {
  display: "flex", width: "100%", alignItems: "center", gap: 14, padding: "14px 20px", minHeight: 44,
  background: "transparent", border: "none", borderTop: "1px solid var(--border)", textAlign: "left" as const,
};

function VoucherRow({ voucher, name, room, onOpen }: VoucherRowProps): ReactElement {
  const body = (
    <>
      <Avatar name={name ?? "?"} color="yellow" size={42} />
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 15.5, fontWeight: 600, color: "var(--text)" }}>{name ?? "Alumno que ya no está en la lista"}</div>
        <div style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{room}</div>
      </div>
      <div style={{ textAlign: "right" }}>
        <div style={{ fontSize: 12.5, color: "var(--text-faint)" }}>{whenLabel(voucher)}</div>
        <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16, color: "var(--text)" }}>
          {voucher.claimedAmountCents === undefined ? "Sin importe" : formatCents(voucher.claimedAmountCents)}
        </div>
      </div>
    </>
  );
  if (onOpen === undefined) {
    return <div style={rowStyle}>{body}</div>;
  }
  return <button type="button" onClick={onOpen} style={{ ...rowStyle, cursor: "pointer" }}>{body}</button>;
}
