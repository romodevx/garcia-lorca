"use client";

import { useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { Icon, type IconName } from "../../_design/Icon";
import type { ColorKey } from "../../_design/colors";
import { listGroups, listSubjects, listTeachers, listTeachingAssignments, listTerms, type Group, type Subject, type Teacher, type TeachingAssignment, type Term } from "../../config/api";
import { captureGrade, getRoster, type RosterRow } from "../../grades/api";
import { listStudents, type Student } from "../../students/api";

const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const AV: ColorKey[] = ["lime", "forest", "orange", "purple", "yellow"];
const distinct = (xs: string[]): string[] => Array.from(new Set(xs));
const sel: CSSProperties = { height: 48, padding: "0 16px", borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15, fontFamily: "var(--font-ui)", outline: "none", cursor: "pointer", minWidth: 240 };

export interface DirectorCaptureProps {
  onBack: () => void;
}

/**
 * Director-scoped grade-capture surface (module_grades, `grades:read/write`). The
 * full school-wide screen: it reads ANY group's assignments (config), resolves
 * student names by fetching the students list, and captures via `/grades/*`. The
 * teacher self-scoped sibling lives in TeacherCapture. This is the original
 * GradesScreen implementation, unchanged — only extracted so GradesScreen can
 * route by role. The gateway is the security boundary; this is UX only.
 */
export function DirectorCapture({ onBack }: DirectorCaptureProps): ReactElement {
  const [assignments, setAssignments] = useState<TeachingAssignment[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [subjects, setSubjects] = useState<Subject[]>([]);
  const [teachers, setTeachers] = useState<Teacher[]>([]);
  const [terms, setTerms] = useState<Term[]>([]);
  const [assignmentId, setAssignmentId] = useState("");
  const [termId, setTermId] = useState("");
  const [roster, setRoster] = useState<RosterRow[]>([]);
  const [draft, setDraft] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(true);
  const [loadingRoster, setLoadingRoster] = useState(false);
  const [savingId, setSavingId] = useState<string | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const say = (m: string): void => { setToast(m); setTimeout(() => setToast(null), 2400); };

  useEffect(() => {
    (async (): Promise<void> => {
      try {
        const [asgs, ss, ts] = await Promise.all([listTeachingAssignments(API_URL), listStudents(API_URL), listTeachers(API_URL)]);
        const gs = await Promise.all(distinct(asgs.map((a) => a.cycleId)).map((id) => listGroups(API_URL, id)));
        const flatGroups = gs.flat();
        const subjectLists = await Promise.all(distinct(flatGroups.map((g) => g.gradeLevelId)).map((id) => listSubjects(API_URL, id)));
        setAssignments(asgs); setStudents(ss); setTeachers(ts); setGroups(flatGroups); setSubjects(subjectLists.flat());
      } catch { /* leave empty */ } finally { setLoading(false); }
    })();
  }, []);

  const groupName = useCallback((id: string): string => groups.find((g) => g.id === id)?.name ?? id, [groups]);
  const subjectName = useCallback((id: string): string => subjects.find((s) => s.id === id)?.name ?? id, [subjects]);
  const teacherName = useCallback((id: string): string => { const t = teachers.find((x) => x.id === id); return t !== undefined ? `${t.firstName} ${t.lastName}` : id; }, [teachers]);
  const studentName = useCallback((id: string): string => { const s = students.find((x) => x.id === id); return s !== undefined ? `${s.firstName} ${s.lastName}` : id; }, [students]);
  const assignmentLabel = useCallback((a: TeachingAssignment): string => `${groupName(a.groupId)} · ${subjectName(a.subjectId)} · ${teacherName(a.teacherId)}`, [groupName, subjectName, teacherName]);

  // Assignment → its cycle's terms.
  useEffect(() => {
    if (assignmentId === "") { setTerms([]); setTermId(""); return; }
    const a = assignments.find((x) => x.id === assignmentId);
    if (a === undefined) return;
    listTerms(API_URL, a.cycleId).then((ts) => setTerms(ts)).catch(() => {});
    setTermId("");
    setRoster([]);
  }, [assignmentId, assignments]);

  // Assignment + term → roster. The `active` guard drops a stale response so a
  // slow roster for a previous assignment/term can never overwrite the current
  // one (a term switch mid-flight would otherwise paint the wrong grades).
  useEffect(() => {
    if (assignmentId === "" || termId === "") { setRoster([]); return; }
    let active = true;
    setLoadingRoster(true);
    getRoster(API_URL, { assignmentId, termId }).then((r) => {
      if (!active) return;
      setRoster(r);
      setDraft(Object.fromEntries(r.map((x) => [x.enrollmentId, x.hasValue ? String(x.value) : ""])));
    }).catch(() => {}).finally(() => { if (active) setLoadingRoster(false); });
    return () => { active = false; };
  }, [assignmentId, termId]);

  const save = async (enrollmentId: string): Promise<void> => {
    const raw = draft[enrollmentId] ?? "";
    const value = parseFloat(raw);
    if (!Number.isFinite(value) || value < 0 || value > 10) { say("Calificación inválida (0–10)"); return; }
    setSavingId(enrollmentId);
    try {
      await captureGrade(API_URL, { assignmentId, enrollmentId, termId, value });
      setRoster((rs) => rs.map((r) => (r.enrollmentId === enrollmentId ? { ...r, value, hasValue: true } : r)));
      say("Calificación guardada");
    } catch { say("No se pudo guardar"); } finally { setSavingId(null); }
  };

  const captured = roster.filter((r) => r.hasValue).length;
  const avg = useMemo(() => { const vs = roster.filter((r) => r.hasValue).map((r) => r.value); return vs.length > 0 ? (vs.reduce((a, b) => a + b, 0) / vs.length).toFixed(1) : "—"; }, [roster]);

  const stats: { icon: IconName; c: string; label: string; val: string }[] = [
    { icon: "students", c: "forest", label: "Alumnos", val: String(roster.length) },
    { icon: "check", c: "lime", label: "Capturadas", val: String(captured) },
    { icon: "clock", c: "yellow", label: "Pendientes", val: String(roster.length - captured) },
    { icon: "star", c: "purple", label: "Promedio", val: avg },
  ];

  return (
    <div style={{ maxWidth: 1160, margin: "0 auto", padding: "24px 28px 96px" }}>
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}><Icon name="back" size={18} stroke={2.4} />Volver al inicio</button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--yellow)", color: "#3A2E00", boxShadow: "0 6px 16px color-mix(in oklab, var(--yellow) 40%, transparent)" }}><Icon name="grades" size={30} /></div>
        <div><h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Calificaciones</h1><p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Captura por asignación y periodo · un clic por alumno</p></div>
      </div>

      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 12, marginBottom: 20 }}>
        <select value={assignmentId} onChange={(e) => setAssignmentId(e.target.value)} aria-label="Asignación" style={sel} disabled={loading}>
          <option value="">Selecciona una asignación…</option>
          {assignments.map((a) => <option key={a.id} value={a.id}>{assignmentLabel(a)}</option>)}
        </select>
        <select value={termId} onChange={(e) => setTermId(e.target.value)} aria-label="Periodo" style={sel} disabled={assignmentId === ""}>
          <option value="">Selecciona un periodo…</option>
          {terms.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </select>
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
          {termId === "" ? (
            <div style={{ padding: "56px 20px", textAlign: "center", color: "var(--text-muted)", fontSize: 15 }}>Elige una asignación y un periodo para capturar.</div>
          ) : loadingRoster ? (
            <div style={{ padding: 56, textAlign: "center", color: "var(--text-muted)" }}>Cargando…</div>
          ) : roster.length === 0 ? (
            <div style={{ padding: "56px 20px", textAlign: "center", color: "var(--text-muted)", fontSize: 15 }}>No hay alumnos en esta asignación.</div>
          ) : (
            roster.map((r, i) => {
              const name = studentName(r.studentId);
              return (
                <div key={r.enrollmentId} style={{ display: "grid", gridTemplateColumns: "1.6fr auto auto", alignItems: "center", gap: 16, padding: "12px 24px", borderRadius: "var(--r-md)" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: 13, minWidth: 0 }}>
                    <Avatar name={name} color={AV[i % AV.length]} size={40} />
                    <span style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)" }}>{name}</span>
                  </div>
                  <input type="number" min={0} max={10} step={0.1} value={draft[r.enrollmentId] ?? ""} aria-label={`Calificación de ${name}`}
                    onChange={(e) => setDraft((d) => ({ ...d, [r.enrollmentId]: e.target.value }))}
                    onKeyDown={(e) => { if (e.key === "Enter") void save(r.enrollmentId); }}
                    style={{ width: 90, height: 46, padding: "0 14px", borderRadius: "var(--r-md)", border: `1.5px solid ${r.hasValue ? "var(--lime)" : "var(--border)"}`, background: "var(--surface)", color: "var(--text)", fontSize: 16, fontFamily: "var(--font-display)", fontWeight: 700, textAlign: "center", outline: "none" }} />
                  <Btn kind="ghost" icon="check" onClick={() => void save(r.enrollmentId)} style={{ height: 44, fontSize: 14, opacity: savingId === r.enrollmentId ? 0.5 : 1 }}>Guardar</Btn>
                </div>
              );
            })
          )}
        </div>
      </div>

      {toast !== null && <div style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--forest)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)", animation: "pop .25s var(--ease) both" }}><Icon name="check" size={20} stroke={2.6} />{toast}</div>}
    </div>
  );
}
