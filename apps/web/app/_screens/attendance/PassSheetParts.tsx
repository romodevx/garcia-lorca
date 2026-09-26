import type { ReactElement } from "react";

import { Icon } from "../../_design/Icon";
import type { AttendanceStatus } from "../../attendance/api";
import { STATUS_META } from "./PassSheetRow";
import type { StatusCounts } from "./sheetModel";

const COUNTERS: readonly { status: AttendanceStatus; label: string }[] = [
  { status: "present", label: "Presentes" },
  { status: "absent", label: "Ausentes" },
  { status: "late", label: "Retardos" },
  { status: "excused", label: "Justificadas" },
];

/** The list's tally, one pill per status (icon + word + number, never colour alone). */
export function Counters({ counts }: { counts: StatusCounts }): ReactElement {
  return (
    <ul aria-label="Resumen de la lista" style={{ display: "flex", flexWrap: "wrap", gap: 8, listStyle: "none", margin: 0, padding: 0 }}>
      {COUNTERS.map(({ status, label }) => (
        <li key={status} style={{ display: "inline-flex", alignItems: "center", gap: 6, padding: "6px 12px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>
          <Icon name={STATUS_META[status].icon} size={15} stroke={2.4} style={{ color: STATUS_META[status].tone }} />
          {label}
          <strong style={{ color: "var(--text)", fontFamily: "var(--font-display)" }}>{counts[status]}</strong>
        </li>
      ))}
    </ul>
  );
}

/** Why the list can only be looked at. */
export function ReadOnlyBanner({ text }: { text: string }): ReactElement {
  return (
    <p style={{ margin: "0 0 12px", padding: "10px 14px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontSize: 14, fontWeight: 600 }}>
      Solo lectura · {text}
    </p>
  );
}

/** After scanning: what the teacher should check before saving. */
export function ScanBanner({ text }: { text: string }): ReactElement {
  return (
    <p role="status" style={{ display: "flex", alignItems: "center", gap: 8, margin: "0 0 12px", padding: "10px 14px", borderRadius: "var(--r-md)", background: "var(--ok-bg)", color: "var(--ok-fg)", fontSize: 14, fontWeight: 700 }}>
      <Icon name="qrScan" size={18} stroke={2.4} />
      {text}
    </p>
  );
}

/** Opens the camera to scan credentials (module_student_card). */
export function ScanButton({ onClick, disabled }: { onClick: () => void; disabled: boolean }): ReactElement {
  return (
    <button type="button" aria-disabled={disabled || undefined} onClick={() => !disabled && onClick()} style={{ minHeight: 44, display: "inline-flex", alignItems: "center", gap: 8, padding: "0 16px", borderRadius: "var(--r-pill)", border: "1.5px solid var(--forest)", background: "var(--surface)", color: "var(--forest)", fontWeight: 700, fontSize: 14, cursor: disabled ? "progress" : "pointer" }}>
      <Icon name="qrScan" size={18} stroke={2.4} />
      Escanear QR
    </button>
  );
}
