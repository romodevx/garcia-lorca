"use client";

import { useEffect, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { announce, type AnnouncementScope } from "../../comms/api";
import { listCycles, listGroups, listLevels, type Cycle, type Group, type Level } from "../../config/api";
import { API_URL } from "../../api-url";

/** One key per attempt: a retry must not buzz four hundred phones twice. */
function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

const SCOPES: { value: AnnouncementScope; label: string }[] = [
  { value: "group", label: "Un grupo" },
  { value: "level", label: "Un nivel" },
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
  const [groups, setGroups] = useState<Group[]>([]);
  const [scope, setScope] = useState<AnnouncementScope>("group");
  const [targetId, setTargetId] = useState("");
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
  }, []);

  useEffect(() => {
    if (cycle === undefined) {
      return;
    }
    listGroups(API_URL, cycle.id, "").then(setGroups).catch(() => {});
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
    if (scope !== "cycle" && targetId === "") {
      setError("Elige a quién va dirigido.");
      return;
    }
    setSending(true);
    setError(undefined);
    try {
      const result = await announce(API_URL, {
        scope,
        ...(scope === "cycle" ? {} : { targetId }),
        cycleId: cycle.id,
        title: title.trim(),
        body: body.trim(),
        idempotencyKey: attemptKey,
      });
      setSent(result);
      setTitle("");
      setBody("");
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
    : groups.map((g) => ({ id: g.id, name: g.name }));

  return (
    <div style={{ maxWidth: 820, margin: "0 auto", padding: "24px 28px 96px" }}>
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}>
        <Icon name="back" size={18} stroke={2.4} />Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--orange)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--orange) 38%, transparent)" }}>
          <Icon name="announcements" size={30} />
        </div>
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
                  onClick={() => { setScope(s.value); setTargetId(""); }}
                  style={{ height: 44, padding: "0 18px", borderRadius: 99, fontSize: 14.5, fontWeight: 600, cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>
                  {s.label}
                </button>
              );
            })}
          </div>
        </div>

        {scope !== "cycle" && (
          <label style={{ display: "block" }}>
            <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>{scope === "level" ? "Nivel" : "Grupo"}</span>
            <select
              aria-label={scope === "level" ? "Nivel" : "Grupo"}
              value={targetId}
              onChange={(e) => setTargetId(e.target.value)}
              style={{ ...field, marginTop: 8 }}>
              <option value="">Elige…</option>
              {targets.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
            </select>
          </label>
        )}

        <label style={{ display: "block" }}>
          <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Título</span>
          <input aria-label="Título" value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Junta de padres" style={{ ...field, marginTop: 8 }} />
        </label>

        <label style={{ display: "block" }}>
          <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Mensaje</span>
          <textarea
            aria-label="Mensaje"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            rows={4}
            placeholder="Jueves 8:00 en el auditorio."
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
    </div>
  );
}
