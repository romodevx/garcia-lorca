"use client";

import { useEffect, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { HelpTip, TextField } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { announce, type AnnouncementScope } from "../../comms/api";
import { listCycles, listLabelledGroups, listLevels, type Cycle, type LabelledGroup, type Level } from "../../config/api";
import { listStudents, type Student } from "../../students/api";
import { API_URL } from "../../api-url";
import { formatStudentName, sortStudents } from "../../_design/students/studentName";
import { BirthdayGreeting } from "./BirthdayGreeting";
import { ModuleHeaderIcon } from "../../_design/ModuleHeaderIcon";

/** One key per attempt: a retry must not buzz four hundred phones twice. */
function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

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
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [levels, setLevels] = useState<Level[]>([]);
  const [groups, setGroups] = useState<LabelledGroup[]>([]);
  const [scope, setScope] = useState<AnnouncementScope>("group");
  // A list, always: one pupil is a list of one, and "1° A y 1° B" is a list of
  // two. Keeping two shapes for the same question is how the send ends up
  // addressing the wrong people.
  const [picked, setPicked] = useState<string[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<{ created: number; audience: number } | null>(null);
  const [error, setError] = useState<string | undefined>(undefined);
  const [attemptKey, setAttemptKey] = useState(newAttemptKey);

  const cycle = cycles.find((c) => c.status === "active") ?? cycles[0];

  useEffect(() => {
    listCycles(API_URL).then(setCycles).catch(() => setError("No pudimos leer los ciclos."));
    listLevels(API_URL).then(setLevels).catch(() => {});
    listStudents(API_URL).then(setStudents).catch(() => {});
  }, []);

  useEffect(() => {
    if (cycle === undefined) {
      return;
    }
    listLabelledGroups(API_URL, cycle.id).then(setGroups).catch(() => {});
  }, [cycle]);

  const send = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (cycle === undefined) {
      setError("No hay un ciclo activo al que dirigir el aviso.");
      return;
    }
    if (title.trim() === "" || body.trim() === "") {
      setError("Un aviso necesita título y mensaje.");
      return;
    }
    if (scope !== "cycle" && picked.length === 0) {
      setError("Elige a quién va dirigido.");
      return;
    }
    setSending(true);
    setError(undefined);
    try {
      const result = await announce(API_URL, {
        scope,
        ...(scope === "cycle" ? {} : { targetIds: picked }),
        cycleId: cycle.id,
        title: title.trim(),
        body: body.trim(),
        idempotencyKey: attemptKey,
      });
      setSent(result);
      setTitle("");
      setBody("");
      setPicked([]);
      // A fresh key for the next message: the old one now means THIS junta, and
      // reusing it would silently deliver nothing.
      setAttemptKey(newAttemptKey());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo enviar el aviso.");
    } finally {
      setSending(false);
    }
  };

  const targets = scope === "level"
    ? levels.map((l) => ({ id: l.id, name: l.name }))
    : scope === "student"
      ? sortStudents(students).map((s) => ({ id: s.id, name: formatStudentName(s) }))
      : groups.map((g) => ({ id: g.id, name: g.label }));

  const toggle = (id: string): void => {
    setPicked((prev) => {
      if (prev.includes(id)) {
        return prev.filter((p) => p !== id);
      }
      // One pupil at a time: a note to a family is not a circular, and picking
      // several would be a group announcement wearing the wrong label.
      return scope === "student" ? [id] : [...prev, id];
    });
  };

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

      <form onSubmit={(e) => void send(e)} className="fade-up" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", padding: "22px 24px", display: "flex", flexDirection: "column", gap: 18 }}>
        <div>
          <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>¿A quién?</span>
          <div style={{ display: "flex", gap: 8, marginTop: 8, flexWrap: "wrap" }}>
            {SCOPES.map((s) => {
              const on = scope === s.value;
              return (
                <button
                  key={s.value}
                  type="button"
                  onClick={() => { setScope(s.value); setPicked([]); }}
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
