"use client";

import type { CSSProperties, ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { DateField } from "../../_design/form";
import { Icon, type IconName } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import type { ColorKey } from "../../_design/colors";
import type { AttendanceStatus } from "../../attendance/api";
import { usePassList } from "./usePassList";

const AV: ColorKey[] = ["lime", "forest", "orange", "purple", "yellow"];
const STATUSES: { value: AttendanceStatus; label: string; color: string }[] = [
  { value: "present", label: "Presente", color: "forest" },
  { value: "absent", label: "Ausente", color: "red" },
  { value: "late", label: "Retardo", color: "yellow" },
  { value: "excused", label: "Justificado", color: "purple" },
];
const sel: CSSProperties = { height: 48, padding: "0 16px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15, fontFamily: "var(--font-ui)", outline: "none", cursor: "pointer" };

export interface DirectorAttendanceProps {
  onBack: () => void;
  today?: string;
}

/**
 * School-wide daily pass list (attendance:read / attendance:write) — the
 * director/admin surface. Extracted verbatim from the original AttendanceScreen
 * when the module went role-adaptive; its behavior is unchanged. It reads the
 * admin config/students/attendance clients through `usePassList` (any group in
 * any cycle). A teacher, who cannot touch those clients, uses TeacherAttendance
 * instead — AttendanceScreen routes between the two.
 */
export function DirectorAttendance({ onBack, today }: DirectorAttendanceProps): ReactElement {
  const list = usePassList(today);
  const { cycles, groups, cycleId, setCycleId, groupId, setGroupId, date, setDate, rows, counts, loadingRoster, saving, rosterError, saveError } = list;
  const ready = groupId !== "" && rows.length > 0;

  const stats: { icon: IconName; c: string; label: string; val: string }[] = [
    { icon: "check", c: "forest", label: "Presentes", val: String(counts.present) },
    { icon: "close", c: "red", label: "Ausentes", val: String(counts.absent) },
    { icon: "clock", c: "yellow", label: "Retardos", val: String(counts.late) },
    { icon: "students", c: "lime", label: "En lista", val: String(rows.length) },
  ];

  return (
    <Page>
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}><Icon name="back" size={18} stroke={2.4} />Volver al inicio</button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--lime)", color: "var(--primary-ink)", boxShadow: "0 6px 16px color-mix(in oklab, var(--lime) 40%, transparent)" }}><Icon name="qr" size={30} /></div>
          <div><h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Asistencia</h1><p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Pase de lista · todos presentes, marca solo las excepciones</p></div>
        </div>
        <Btn icon="check" onClick={() => void list.save()} style={{ height: 52, fontSize: 16.5, opacity: ready && !saving ? 1 : 0.45, pointerEvents: ready && !saving ? "auto" : "none" }}>{saving ? "Guardando…" : "Guardar lista"}</Btn>
      </div>

      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 20 }}>
        <select value={cycleId} onChange={(e) => setCycleId(e.target.value)} aria-label="Ciclo" style={sel}>{cycles.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}</select>
        <select value={groupId} onChange={(e) => setGroupId(e.target.value)} aria-label="Grupo" style={sel}><option value="">Selecciona un grupo…</option>{groups.map((g) => <option key={g.id} value={g.id}>{g.name}</option>)}</select>
        <div style={{ minWidth: 0 }}>
          <DateField label="Fecha" value={date} onChange={setDate} />
        </div>
      </div>

      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 22 }}>
        {stats.map((k, i) => (
          <div key={i} style={{ flex: "1 1 180px", minWidth: 0, display: "flex", alignItems: "center", gap: 14, padding: "16px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
            <div style={{ width: 44, height: 44, borderRadius: 13, flexShrink: 0, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${k.c}) 16%, var(--surface))`, color: `var(--${k.c})` }}><Icon name={k.icon} size={22} /></div>
            <div><div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600 }}>{k.label}</div><div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, color: "var(--text)", lineHeight: 1.1 }}>{k.val}</div></div>
          </div>
        ))}
      </div>

      <div className="fade-up" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
        <div style={{ padding: 8 }}>
          {groupId === "" ? (
            <div style={{ padding: "56px 20px", textAlign: "center", color: "var(--text-muted)", fontSize: 15 }}>Selecciona un grupo para pasar lista.</div>
          ) : loadingRoster ? (
            <div style={{ padding: 56, textAlign: "center", color: "var(--text-muted)" }}>Cargando…</div>
          ) : rosterError !== undefined ? (
            <div role="alert" style={{ padding: "56px 20px", textAlign: "center", color: "var(--bad-fg)", fontSize: 15, fontWeight: 600 }}>No se pudo cargar el pase de lista. Reintenta.</div>
          ) : rows.length === 0 ? (
            <div style={{ padding: "56px 20px", textAlign: "center", color: "var(--text-muted)", fontSize: 15 }}>Este grupo no tiene alumnos inscritos.</div>
          ) : (
            rows.map((r, i) => {
              const name = list.studentName(r.studentId);
              return (
                <div key={r.enrollmentId} style={{ display: "grid", gridTemplateColumns: "1.6fr auto", alignItems: "center", gap: 16, padding: "12px 24px", borderRadius: "var(--r-md)" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 13, minWidth: 0 }}>
                    <Avatar name={name} color={AV[i % AV.length]} size={40} />
                    <span style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)" }}>{name}</span>
                  </div>
                  <div role="radiogroup" aria-label={`Asistencia de ${name}`} style={{ display: "flex", gap: 6, flexWrap: "wrap", justifyContent: "flex-end" }}>
                    {STATUSES.map((s) => {
                      const on = r.status === s.value;
                      return <button key={s.value} role="radio" aria-checked={on} aria-label={s.label} onClick={() => list.setStatus(r.enrollmentId, s.value)} style={{ height: 38, padding: "0 14px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", border: `1.5px solid ${on ? `var(--${s.color})` : "var(--border)"}`, background: on ? `color-mix(in oklab, var(--${s.color}) 14%, var(--surface))` : "var(--surface)", color: on ? `var(--${s.color === "yellow" ? "warn-fg" : s.color})` : "var(--text-muted)" }}>{s.label}</button>;
                    })}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {saveError !== undefined ? (
        <div role="alert" style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--red)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)", animation: "pop .25s var(--ease) both" }}><Icon name="warning" size={20} stroke={2.4} />No se pudo guardar la lista. Reintenta.</div>
      ) : list.toast !== undefined ? (
        <div style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--forest)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)", animation: "pop .25s var(--ease) both" }}><Icon name="check" size={20} stroke={2.6} />{list.toast}</div>
      ) : null}
    </Page>
  );
}
