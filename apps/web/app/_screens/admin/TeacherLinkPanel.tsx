"use client";

import { useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import type { AdminUser } from "../../admin/api";
import type { Teacher } from "../../teachers/api";
import { EmptyHint, ErrorAlert, Section, SelectField } from "./parts";

export interface TeacherLinkPanelProps {
  teachers: readonly Teacher[];
  users: readonly AdminUser[];
  /** Link (userId set) or unlink (userId "") a teacher to an iam user. */
  onLink: (teacherId: string, userId: string) => Promise<void>;
}

/**
 * Teacher↔user link: connects a config teacher record to an iam login account
 * (populating the reserved `teachers.user_id`, the seam that later unlocks
 * per-teacher scoping). Picking a teacher + a user and linking, or unlinking an
 * already-linked teacher. Dumb panel — the gateway validates the user exists via
 * iam before config stores the id (module_config + config:write).
 */
export function TeacherLinkPanel({ teachers, users, onLink }: TeacherLinkPanelProps): ReactElement {
  const [teacherId, setTeacherId] = useState("");
  const [userId, setUserId] = useState("");
  const [pending, setPending] = useState(false);
  const [busyId, setBusyId] = useState<string | undefined>(undefined);
  const [error, setError] = useState<string | undefined>(undefined);

  const teacherOptions = teachers.map((t) => ({ value: t.id, label: `${t.firstName} ${t.lastName} · ${t.email}` }));
  const userOptions = users.map((u) => ({ value: u.id, label: `${u.displayName} · ${u.email}` }));
  const userLabel = (id: string): string => {
    const u = users.find((x) => x.id === id);
    return u !== undefined ? `${u.displayName} · ${u.email}` : id;
  };
  const linked = teachers.filter((t) => t.userId !== "");

  const canLink = teacherId !== "" && userId !== "" && !pending;

  const link = async (): Promise<void> => {
    setPending(true);
    setError(undefined);
    try {
      await onLink(teacherId, userId);
      setTeacherId("");
      setUserId("");
    } catch {
      setError("No se pudo vincular. Intenta de nuevo.");
    } finally {
      setPending(false);
    }
  };

  const unlink = async (id: string): Promise<void> => {
    setBusyId(id);
    setError(undefined);
    try {
      await onLink(id, "");
    } catch {
      setError("No se pudo desvincular. Intenta de nuevo.");
    } finally {
      setBusyId(undefined);
    }
  };

  return (
    <Section title="Vincular profesor con usuario" subtitle="Conecta un profesor del catálogo con una cuenta de acceso.">
      {linked.length > 0 ? (
        <ul style={{ listStyle: "none", padding: 0, margin: "0 0 16px", display: "grid", gap: 8 }}>
          {linked.map((t) => (
            <li
              key={t.id}
              style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 10, padding: "10px 12px", borderRadius: "var(--r-md)", background: "var(--surface-2)" }}
            >
              <span style={{ fontSize: 14 }}>
                <span style={{ fontWeight: 600 }}>
                  {t.firstName} {t.lastName}
                </span>{" "}
                <span style={{ color: "var(--text-muted)" }}>→ {userLabel(t.userId)}</span>
              </span>
              <button
                onClick={() => void unlink(t.id)}
                aria-label={`Desvincular a ${t.firstName} ${t.lastName}`}
                disabled={busyId === t.id}
                style={{ fontSize: 13, fontWeight: 600, color: "var(--bad-fg)", opacity: busyId === t.id ? 0.5 : 1 }}
              >
                Desvincular
              </button>
            </li>
          ))}
        </ul>
      ) : (
        <EmptyHint>Aún no hay profesores vinculados a un usuario.</EmptyHint>
      )}

      <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr auto", gap: 12, alignItems: "end" }}>
        <SelectField label="Profesor" value={teacherId} options={teacherOptions} onChange={setTeacherId} placeholder="Selecciona un profesor…" />
        <SelectField label="Usuario" value={userId} options={userOptions} onChange={setUserId} placeholder="Selecciona un usuario…" />
        <Btn type="button" icon="check" disabled={!canLink} onClick={() => void link()}>
          Vincular
        </Btn>
      </div>
      {error !== undefined && <ErrorAlert message={error} />}
    </Section>
  );
}
