"use client";

import type { ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { HelpTip, TextField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import type { AnnouncementScope } from "../../comms/api";
import { formatStudentName, sortStudents } from "../../_design/students/studentName";
import { BirthdayGreeting } from "./BirthdayGreeting";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";
import { useAnnouncementComposer } from "./useAnnouncementComposer";

const SCOPES: { value: AnnouncementScope; label: string }[] = [
  { value: "student", label: "Un alumno" },
  { value: "group", label: "Grupos" },
  { value: "level", label: "Niveles" },
  { value: "cycle", label: "Toda la escuela" },
];

const field = {
  width: "100%", height: 48, padding: "0 16px", borderRadius: "var(--r-md)",
  border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)",
  fontSize: 15.5, outline: "none",
} as const;

export interface AnnouncementsScreenProps {
  onBack: () => void;
}

/**
 * One message, one audience.
 *
 * The school's alternative is sending it per family, which is how a junta gets
 * announced to twenty-eight of thirty. The audience is chosen once — a group, a
 * level, the whole school — and the system works out who that is.
 */
export function AnnouncementsScreen({ onBack }: AnnouncementsScreenProps): ReactElement {
  const {
    levels, groups, students, scope, picked, title, body, sending, sent, error,
    chooseScope, toggle, setTitle, setBody, submit,
  } = useAnnouncementComposer();

  const targets = scope === "level"
    ? levels.map((l) => ({ id: l.id, name: l.name }))
    : scope === "student"
      ? sortStudents(students).map((s) => ({ id: s.id, name: formatStudentName(s) }))
      : groups.map((g) => ({ id: g.id, name: g.label }));

  return (
    <Page variant="reading">
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}>
        <Icon name="back" size={18} stroke={2.4} />Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <ModuleHeaderIcon icon="announcements" color="orange" shrinks />
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Avisos</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>
            Un mensaje para un grupo, un nivel o toda la escuela
          </p>
        </div>
      </div>

      <form onSubmit={(e) => void submit(e)} className="fade-up" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", padding: "22px 24px", display: "flex", flexDirection: "column", gap: 18 }}>
        <div>
          <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>¿A quién?</span>
          <div style={{ display: "flex", gap: 8, marginTop: 8, flexWrap: "wrap" }}>
            {SCOPES.map((s) => {
              const on = scope === s.value;
              return (
                <button
                  key={s.value}
                  type="button"
                  onClick={() => chooseScope(s.value)}
                  style={{ height: 44, padding: "0 18px", borderRadius: 99, fontSize: 14.5, fontWeight: 600, cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
                  {s.label}
                </button>
              );
            })}
          </div>
        </div>

        {scope !== "cycle" && (
          <div>
            <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>
              {scope === "level" ? "Niveles" : scope === "student" ? "Alumno" : "Grupos"}
              {picked.length > 1 && ` · ${picked.length} seleccionados`}
            </span>
            <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 8, maxHeight: 200, overflowY: "auto" }}>
              {targets.map((t) => {
                const on = picked.includes(t.id);
                return (
                  <button
                    key={t.id}
                    type="button"
                    onClick={() => toggle(t.id)}
                    aria-label={t.name}
                    aria-pressed={on}
                    style={{ height: 40, padding: "0 15px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, cursor: "pointer", border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
                    {t.name}
                  </button>
                );
              })}
            </div>
          </div>
        )}

        {/*
          SIN `uppercase`. El título es lo que llega a la notificación del
          celular de los papás, así que es cuerpo del aviso, no dato de
          catálogo — y la propia capa de captura dice que un aviso no se grita.
        */}
        <TextField label="Título" value={title} onChange={setTitle} required help="Lo que las familias verán en la lista de avisos, antes de abrirlo." />

        <label style={{ display: "block" }}>
          <span style={{ display: "inline-flex", alignItems: "baseline", gap: 6 }}>
            <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Mensaje</span>
            <HelpTip
              id="aviso-cuerpo-help"
              label="Mensaje"
              text="El aviso completo. Llega tal cual a la app de los padres, así que se escribe como se quiere que se lea."
            />
          </span>
          <textarea
            aria-label="Mensaje"
            aria-describedby="aviso-cuerpo-help"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={4}
            style={{ ...field, height: "auto", padding: "12px 16px", marginTop: 8, resize: "vertical", fontFamily: "inherit" }} />
        </label>

        {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
        {sent !== null && (
          <p role="status" style={{ color: "var(--ok-fg)", fontSize: 14.5, margin: 0 }}>
            Enviado a {sent.audience} {sent.audience === 1 ? "alumno" : "alumnos"}
            {sent.created === 0 && " · ya se había enviado, no se repitió"}
          </p>
        )}

        <div style={{ display: "flex", alignItems: "center" }}>
          <div style={{ flex: 1 }} />
          <Btn icon="check" type="submit" style={{ opacity: sending ? 0.5 : 1, height: 50 }}>Enviar aviso</Btn>
        </div>
      </form>

      {/* T11: the automatic message, written where the school writes to families. */}
      <BirthdayGreeting />
    </Page>
  );
}
