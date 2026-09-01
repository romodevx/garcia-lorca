"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { Icon, type IconName } from "../../_design/Icon";
import type { ColorKey } from "../../_design/colors";
import {
  createStudent,
  enroll,
  listStudents,
  type AdmissionStatus,
  type CreateStudentInput,
  type Student,
} from "../../students/api";
import { listCycles, listGradeLevels, listGroups, listLevels, type Cycle, type GradeLevel, type Group, type Level } from "../../config/api";
import { enrollIntoGrade, StudentsError, type Placement } from "../../students/api";

import { API_URL } from "../../api-url";
import { csvName, downloadCsv, toCsv, type CsvColumn } from "../../_lib/csv";
import { ContactsPanel } from "./ContactsPanel";


// ── admission status → pill (design language) ──
interface StatusMeta {
  label: string;
  dot: string;
  bg: string;
  fg: string;
  tab: "active" | "pending" | "inactive";
}
const STATUS: Record<AdmissionStatus, StatusMeta> = {
  enrolled: { label: "Inscrito", dot: "var(--forest)", bg: "var(--ok-bg)", fg: "var(--ok-fg)", tab: "active" },
  accepted: { label: "Aceptado", dot: "var(--forest)", bg: "var(--ok-bg)", fg: "var(--ok-fg)", tab: "active" },
  pending: { label: "Trámite", dot: "var(--yellow)", bg: "var(--warn-bg)", fg: "var(--warn-fg)", tab: "pending" },
  rejected: { label: "Rechazado", dot: "var(--red)", bg: "var(--bad-bg)", fg: "var(--bad-fg)", tab: "inactive" },
  withdrawn: { label: "Baja", dot: "var(--red)", bg: "var(--bad-bg)", fg: "var(--bad-fg)", tab: "inactive" },
};
const FALLBACK_STATUS: StatusMeta = { label: "—", dot: "var(--text-faint)", bg: "var(--surface-3)", fg: "var(--text-muted)", tab: "pending" };
const statusOf = (s: AdmissionStatus): StatusMeta => STATUS[s] ?? FALLBACK_STATUS;

const AV: ColorKey[] = ["lime", "forest", "orange", "purple", "yellow"];

function StatusPill({ status }: { status: AdmissionStatus }): ReactElement {
  const s = statusOf(status);
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 7, padding: "5px 12px 5px 10px", borderRadius: 99, background: s.bg, color: s.fg, fontWeight: 700, fontSize: 13, fontFamily: "var(--font-display)", whiteSpace: "nowrap" }}>
      <span style={{ width: 9, height: 9, borderRadius: 99, background: s.dot, boxShadow: `0 0 0 3px color-mix(in oklab, ${s.dot} 22%, transparent)` }} />
      {s.label}
    </span>
  );
}

const qb: CSSProperties = { width: 36, height: 36, borderRadius: "50%", display: "grid", placeItems: "center", background: "var(--surface-3)", border: "1px solid var(--border)", color: "var(--text-muted)", cursor: "pointer" };

const pager = (disabled: boolean): CSSProperties => ({
  display: "inline-flex", alignItems: "center", gap: 6, height: 40, padding: "0 16px", borderRadius: 99,
  fontSize: 14, fontWeight: 600, fontFamily: "var(--font-display)", border: "1px solid var(--border)",
  background: "var(--surface)", color: "var(--text)", cursor: disabled ? "not-allowed" : "pointer", opacity: disabled ? 0.45 : 1,
});

/**
 * Marks a failure that happened AFTER the pupil was created, while placing them.
 *
 * The distinction is the whole reason it exists: the drawer has to tell somebody
 * that their pupil is safe and only the placement failed, or they re-type the
 * form and meet "ya existe ese CURP".
 */
export class PlacementFailed extends Error {
  override readonly cause: unknown;

  constructor(cause: unknown) {
    super("placement failed");
    this.name = "PlacementFailed";
    this.cause = cause;
  }
}

/** The shape the quick-add drawer validates before it asks the server. */
export interface QuickAddDraft {
  curp: string;
  firstName: string;
  lastName: string;
  guardianName: string;
  /** The config grade the pupil joins. The GROUP is chosen by capacity, not here. */
  gradeLevelId?: string;
}

/** A CURP is exactly eighteen characters. The server checks its shape too. */
const CURP_LENGTH = 18;

/**
 * The four levels the SEP recognises, folded the way the services fold them.
 *
 * A school may name its levels anything — the catalogue is theirs — but a pupil's
 * record carries a level the whole system understands, so a name outside this
 * list has nowhere to go. Checking here means the person is told which levels
 * work instead of meeting a 400 that blames their CURP.
 *
 * "maternal" was missing until M16, which meant this school could not enrol its
 * youngest pupils at all — the guard below was reporting a real gap as if it
 * were the user's mistake.
 */
const SEP_LEVELS = ["maternal", "kinder", "primaria", "secundaria", "preparatoria"];

function foldLevel(name: string): string {
  return name.trim().toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "");
}

/** Whether a school's level name corresponds to a SEP level. */
export function isSepLevel(name: string): boolean {
  return SEP_LEVELS.includes(foldLevel(name));
}

/**
 * What the form is still waiting for, or undefined when it is ready.
 *
 * This exists because the button used to be greyed out and silent: four separate
 * requirements, any one of which could be unmet, and no way to find out which.
 * The one nobody guesses is the tutor — a two-letter name failed the check with
 * no sign at all.
 *
 * One field at a time, in reading order, so the answer matches where the eye
 * already is.
 */
export function missingFieldMessage(draft: QuickAddDraft): string | undefined {
  const curp = draft.curp.trim();
  if (curp === "") return "Escribe el CURP del alumno.";
  if (curp.length !== CURP_LENGTH) {
    return `El CURP debe tener ${CURP_LENGTH} caracteres; llevas ${curp.length}.`;
  }
  if (draft.firstName.trim().length < 2) return "Escribe el nombre del alumno.";
  if (draft.lastName.trim().length < 2) return "Escribe los apellidos del alumno.";
  if (draft.guardianName.trim().length < 3) return "Escribe el nombre del tutor.";
  if (draft.gradeLevelId !== undefined && draft.gradeLevelId === "") {
    return "Elige el nivel y el grado al que entra.";
  }
  return undefined;
}

/**
 * What the server said, in words a person can act on.
 *
 * A duplicate and a malformed CURP need different advice, and a server that fell
 * over needs advice about neither — blaming the CURP for it, which is what this
 * used to do, sends somebody re-typing a perfectly good one.
 */
export function createFailureMessage(err: unknown): string {
  if (err instanceof StudentsError) {
    if (err.isConflict) return "Ya existe un alumno con ese CURP.";
    if (err.isInvalid) return "Revisa el CURP: son 18 caracteres con el formato oficial.";
  }
  return "No se pudo crear el alumno. Intenta de nuevo en un momento.";
}

/**
 * What went wrong PLACING a pupil who, by then, already exists.
 *
 * Saying so is the whole point of a separate message: without it somebody reads
 * "no quedan lugares", re-types the whole form and gets "ya existe ese CURP",
 * which is true and useless.
 */
export function placementFailureMessage(err: unknown): string {
  const tail = " El alumno sí se creó: inscríbelo desde su ficha cuando haya lugar.";
  if (err instanceof StudentsError) {
    if (err.isConflict) return "Ese grado ya no tiene lugares. Abre otro grupo en Configuración." + tail;
    if (err.isInvalid) return (err.detail !== "" ? err.detail : "No se pudo inscribir en ese grado.") + tail;
  }
  return "No se pudo inscribir al alumno en ese grado." + tail;
}

// ── quick-add slide-over: create a student in one panel, minimal fields ──
function QuickAdd({
  onClose,
  onSave,
  levels,
}: {
  onClose: () => void;
  onSave: (input: CreateStudentInput, gradeLevelId: string) => Promise<void>;
  levels: readonly Level[];
}): ReactElement {
  const [curp, setCurp] = useState("");
  const [firstName, setFirst] = useState("");
  const [lastName, setLast] = useState("");
  const [levelId, setLevelId] = useState("");
  const [gradeLevelId, setGradeLevelId] = useState("");
  const [grades, setGrades] = useState<GradeLevel[]>([]);
  const [guardianName, setGuardian] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);

  // Choosing a level loads its grades. They are whatever the school configured —
  // kínder has three, primaria six — so a school that adds one sees it here
  // without anybody editing this screen.
  useEffect(() => {
    if (levelId === "") {
      setGrades([]);
      return;
    }
    let alive = true;
    setGradeLevelId("");
    listGradeLevels(API_URL, levelId)
      .then((gs) => alive && setGrades(gs))
      .catch(() => alive && setGrades([]));
    return () => {
      alive = false;
    };
  }, [levelId]);

  const submit = async (): Promise<void> => {
    // Validated on PRESS, not by greying the button out. A control that refuses
    // and will not say why is the thing this replaced.
    const missing = missingFieldMessage({ curp, firstName, lastName, guardianName, gradeLevelId });
    if (missing !== undefined) {
      setError(missing);
      return;
    }
    const chosenLevel = levels.find((l) => l.id === levelId);
    if (chosenLevel === undefined) {
      setError("Elige el nivel y el grado al que entra.");
      return;
    }
    if (!isSepLevel(chosenLevel.name)) {
      setError(
        `«${chosenLevel.name}» no es un nivel de la SEP, así que no puede recibir alumnos. ` +
          "Renómbralo en Configuración a Maternal, Kínder, Primaria, Secundaria o Preparatoria.",
      );
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      // The level travels as the config level's NAME. The students service folds
      // accents ("Kínder" → kinder), so there is no mapping table on this side to
      // drift out of step with the one that decides.
      await onSave(
        {
          curp: curp.trim().toUpperCase(),
          firstName: firstName.trim(),
          lastName: lastName.trim(),
          level: chosenLevel.name,
          // Left empty on purpose: the server writes it when capacity picks the
          // group, so the record cannot claim a group the pupil is not in.
          gradeGroup: "",
          guardianName: guardianName.trim(),
        },
        gradeLevelId,
      );
    } catch (err) {
      setError(err instanceof PlacementFailed ? placementFailureMessage(err.cause) : createFailureMessage(err));
      setSaving(false);
    }
  };

  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, zIndex: 70, background: "rgba(6,20,12,.5)", backdropFilter: "blur(4px)", display: "flex", justifyContent: "flex-end" }}>
      <div onClick={(e) => e.stopPropagation()} role="dialog" aria-label="Nuevo alumno" style={{ width: "min(520px, 100%)", height: "100%", background: "var(--surface)", borderLeft: "1px solid var(--border)", boxShadow: "var(--shadow-pop)", display: "flex", flexDirection: "column", animation: "slideIn .28s var(--ease) both" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 14, padding: "22px 26px", borderBottom: "1px solid var(--border)" }}>
          <div style={{ width: 48, height: 48, borderRadius: 14, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff" }}><Icon name="students" size={26} /></div>
          <div style={{ flex: 1 }}>
            <h2 style={{ fontSize: 21, fontWeight: 700, color: "var(--text)" }}>Nuevo alumno</h2>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", marginTop: 2 }}>Alta rápida · la admisión inicia en trámite</p>
          </div>
          <button onClick={onClose} aria-label="Cerrar" style={{ ...qb, width: 40, height: 40 }}><Icon name="close" size={19} /></button>
        </div>

        <div style={{ flex: 1, overflowY: "auto", padding: "22px 26px", display: "flex", flexDirection: "column", gap: 18 }}>
          <Field label="CURP" req>
            <input autoFocus value={curp} onChange={(e) => setCurp(e.target.value)} placeholder="XXXX000000XXXXXX00" style={fi} />
          </Field>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
            <Field label="Nombre" req><input value={firstName} onChange={(e) => setFirst(e.target.value)} placeholder="Ana Sofía" style={fi} /></Field>
            <Field label="Apellidos" req><input value={lastName} onChange={(e) => setLast(e.target.value)} placeholder="Ruiz Márquez" style={fi} /></Field>
          </div>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
            <Field label="Nivel" req>
              <select aria-label="Nivel" value={levelId} onChange={(e) => setLevelId(e.target.value)} style={fi}>
                <option value="">Selecciona…</option>
                {levels.map((l) => (
                  <option key={l.id} value={l.id}>{l.name}</option>
                ))}
              </select>
            </Field>
            <Field label="Grado" req>
              <select aria-label="Grado" value={gradeLevelId} onChange={(e) => setGradeLevelId(e.target.value)} disabled={grades.length === 0} style={fi}>
                <option value="">{levelId === "" ? "Elige un nivel primero" : "Selecciona…"}</option>
                {grades.map((g) => (
                  <option key={g.id} value={g.id}>{g.name}</option>
                ))}
              </select>
            </Field>
          </div>
          <Field label="Tutor" req><input value={guardianName} onChange={(e) => setGuardian(e.target.value)} placeholder="Nombre del tutor" style={fi} /></Field>
          <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
            <span style={{ color: "var(--forest)", flexShrink: 0, marginTop: 1 }}><Icon name="check" size={19} stroke={2.6} /></span>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>El grupo <strong>se asigna solo</strong>, según el cupo de cada salón: se llena A y el siguiente alumno pasa a B. Los documentos se cargan después.</p>
          </div>
          {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
        </div>

        <div style={{ display: "flex", gap: 12, alignItems: "center", padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          {/* "Crear", not "Guardar": this makes a pupil that did not exist. And
              disabled ONLY while the request is in flight — an incomplete form
              gets a sentence, not a dead control. */}
          <Btn icon="check" onClick={() => void submit()} disabled={saving} style={{ opacity: saving ? 0.45 : 1, height: 52, fontSize: 16 }}>
            {saving ? "Creando…" : "Crear alumno"}
          </Btn>
        </div>
      </div>
    </div>
  );
}

function Field({ label, req, children }: { label: string; req?: boolean; children: ReactElement }): ReactElement {
  return (
    <label style={{ display: "block" }}>
      <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)", marginLeft: 3 }}>{label}{req === true && <b style={{ color: "var(--red)" }}> *</b>}</span>
      <div style={{ marginTop: 8 }}>{children}</div>
    </label>
  );
}
const fi: CSSProperties = { width: "100%", height: 50, padding: "0 16px", borderRadius: "var(--r-md)", border: "1.5px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15.5, outline: "none" };

// ── detail sheet ──
function StudentSheet({ st, groups, onClose, onEnroll }: { st: Student; groups: readonly Group[]; onClose: () => void; onEnroll: (groupId: string) => Promise<void> }): ReactElement {
  const rows: [string, string][] = [
    ["CURP", st.curp],
    ["Nivel", `${st.level} · ${st.gradeGroup}`],
    ["Tutor", st.guardianName],
    ["Admisión", statusOf(st.admissionStatus).label],
  ];
  const [gid, setGid] = useState("");
  return (
    <div onClick={onClose} style={{ position: "fixed", inset: 0, zIndex: 70, background: "rgba(6,20,12,.5)", backdropFilter: "blur(4px)", display: "flex", justifyContent: "flex-end" }}>
      <div onClick={(e) => e.stopPropagation()} role="dialog" aria-label="Ficha del alumno" style={{ width: "min(460px,100%)", height: "100%", background: "var(--surface)", borderLeft: "1px solid var(--border)", boxShadow: "var(--shadow-pop)", display: "flex", flexDirection: "column", animation: "slideIn .28s var(--ease) both" }}>
        <div style={{ padding: "26px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", gap: 16 }}>
          <Avatar name={`${st.firstName} ${st.lastName}`} color="forest" size={62} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <h2 style={{ fontSize: 20, fontWeight: 700, color: "var(--text)" }}>{st.firstName} {st.lastName}</h2>
            <div style={{ marginTop: 6 }}><StatusPill status={st.admissionStatus} /></div>
          </div>
          <button onClick={onClose} aria-label="Cerrar" style={{ ...qb, width: 40, height: 40 }}><Icon name="close" size={19} /></button>
        </div>
        <div style={{ flex: 1, overflowY: "auto", padding: "8px 26px" }}>
          {rows.map(([k, v]) => (
            <div key={k} style={{ display: "flex", justifyContent: "space-between", gap: 16, padding: "15px 0", borderBottom: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>{k}</span>
              <span style={{ fontSize: 14.5, fontWeight: 600, color: "var(--text)", textAlign: "right" }}>{v}</span>
            </div>
          ))}
          <ContactsPanel studentId={st.id} />
          {groups.length > 0 && (
            <div style={{ padding: "18px 0" }}>
              <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>Inscribir en grupo</span>
              <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginTop: 10 }}>
                {groups.map((g) => {
                  const on = gid === g.id;
                  return <button key={g.id} onClick={() => setGid(g.id)} style={{ height: 40, padding: "0 15px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>{g.name}</button>;
                })}
              </div>
            </div>
          )}
        </div>
        <div style={{ display: "flex", gap: 10, padding: "18px 26px", borderTop: "1px solid var(--border)", background: "var(--surface-2)" }}>
          <Btn kind="ghost" icon="documents" style={{ flex: 1 }}>Documentos</Btn>
          <Btn icon="check" onClick={() => gid !== "" && void onEnroll(gid)} style={{ flex: 1, opacity: gid === "" ? 0.5 : 1, pointerEvents: gid === "" ? "none" : "auto" }}>Inscribir</Btn>
        </div>
      </div>
    </div>
  );
}

export interface StudentsScreenProps {
  onBack: () => void;
}

/** What the school reads in Excel, in the order the table shows it. */
const EXPORT_COLUMNS: readonly CsvColumn<Student>[] = [
  { label: "Apellidos", value: (s) => s.lastName },
  { label: "Nombre", value: (s) => s.firstName },
  { label: "CURP", value: (s) => s.curp },
  { label: "Nivel", value: (s) => s.level },
  { label: "Grado y grupo", value: (s) => s.gradeGroup },
  { label: "Tutor", value: (s) => s.guardianName },
  { label: "Estado", value: (s) => statusOf(s.admissionStatus).label },
];

export function StudentsScreen({ onBack }: StudentsScreenProps): ReactElement {
  const [students, setStudents] = useState<Student[]>([]);
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [levels, setLevels] = useState<Level[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [loading, setLoading] = useState(true);
  const [tab, setTab] = useState<"all" | "active" | "pending" | "inactive">("all");
  const [level, setLevel] = useState("Todos");
  const [q, setQ] = useState("");
  const [add, setAdd] = useState(false);
  const [sheet, setSheet] = useState<Student | null>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [pageSize, setPageSize] = useState(20);
  const [page, setPage] = useState(0);

  // A single tracked timer: replacing a toast clears the prior countdown so a
  // fast second toast never gets dismissed early by the first one's timeout.
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const say = useCallback((m: string): void => {
    setToast(m);
    if (toastTimer.current !== null) clearTimeout(toastTimer.current);
    toastTimer.current = setTimeout(() => setToast(null), 2600);
  }, []);
  useEffect(() => () => { if (toastTimer.current !== null) clearTimeout(toastTimer.current); }, []);

  const reload = useCallback((): void => {
    listStudents(API_URL).then(setStudents).catch(() => {}).finally(() => setLoading(false));
  }, []);
  useEffect(() => {
    reload();
    listLevels(API_URL).then(setLevels).catch(() => setLevels([]));
    listCycles(API_URL).then((cs) => {
      setCycles(cs);
      const active = cs.find((c) => c.status === "active") ?? cs[0];
      if (active !== undefined) listGroups(API_URL, active.id).then(setGroups).catch(() => {});
    }).catch(() => {});
  }, [reload]);

  const counts = useMemo(() => ({
    all: students.length,
    active: students.filter((s) => statusOf(s.admissionStatus).tab === "active").length,
    pending: students.filter((s) => statusOf(s.admissionStatus).tab === "pending").length,
    inactive: students.filter((s) => statusOf(s.admissionStatus).tab === "inactive").length,
  }), [students]);
  const levelsPresent = useMemo(() => Array.from(new Set(students.map((s) => s.level))), [students]);


  // The padrón as it is on screen: the tab, the level and the search. A file
  // that quietly held every pupil would look right and be wrong.
  const exportView = (): void => {
    downloadCsv(csvName("alumnos", new Date()), toCsv(EXPORT_COLUMNS, filtered));
    say(`${filtered.length} alumnos exportados`);
  };

  const filtered = useMemo(() => {
    const needle = q.trim().toLowerCase();
    return students
      .filter((s) => (tab === "all" || statusOf(s.admissionStatus).tab === tab) && (level === "Todos" || s.level === level) &&
        (needle === "" || `${s.firstName} ${s.lastName}`.toLowerCase().includes(needle) || s.curp.toLowerCase().includes(needle) || s.guardianName.toLowerCase().includes(needle)))
      .sort((a, b) => `${a.lastName} ${a.firstName}`.localeCompare(`${b.lastName} ${b.firstName}`, "es"));
  }, [students, tab, level, q]);

  // How many pupils sit in each classroom, over whatever filter is on. The
  // school asked for it and it is the number a director counts by hand before
  // deciding whether to open another group.
  const perGroup = useMemo(() => {
    const counted = new Map<string, number>();
    for (const s of filtered) {
      counted.set(s.gradeGroup, (counted.get(s.gradeGroup) ?? 0) + 1);
    }
    return [...counted.entries()].sort((a, b) => a[0].localeCompare(b[0], "es"));
  }, [filtered]);

  // Any filter change (or a new page size) sends the view back to the first page.
  useEffect(() => { setPage(0); }, [tab, level, q, pageSize]);

  const total = filtered.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const cur = Math.min(page, pageCount - 1);
  const paged = filtered.slice(cur * pageSize, cur * pageSize + pageSize);
  const from = total === 0 ? 0 : cur * pageSize + 1;
  const to = Math.min(total, (cur + 1) * pageSize);

  const cycleName = cycles.find((c) => c.status === "active")?.name ?? cycles[0]?.name ?? "—";
  const anyFilter = tab !== "all" || level !== "Todos" || q !== "";
  const clear = (): void => { setTab("all"); setLevel("Todos"); setQ(""); };

  const onSave = async (input: CreateStudentInput, gradeLevelId: string): Promise<void> => {
    const created = await createStudent(API_URL, input);
    // Two calls, and the SECOND one is the one that can fail on a full grade —
    // by then the pupil exists. PlacementFailed carries that distinction to the
    // drawer, so the message can say "el alumno sí se creó" instead of sending
    // somebody back to re-type a CURP that is now taken.
    let placement: Placement;
    try {
      placement = await enrollIntoGrade(API_URL, created.id, gradeLevelId);
    } catch (err) {
      throw new PlacementFailed(err);
    }
    setAdd(false);
    reload();
    say(`Alumno creado · ${input.firstName} quedó en ${placement.groupName}`);
  };
  const onEnroll = async (groupId: string): Promise<void> => {
    if (sheet === null) return;
    try {
      await enroll(API_URL, { studentId: sheet.id, groupId });
      setSheet(null);
      reload();
      say("Alumno inscrito en el grupo");
    } catch {
      say("No se pudo inscribir");
    }
  };

  const tabs: [typeof tab, string][] = [["all", "Todos"], ["active", "Inscritos"], ["pending", "Trámite"], ["inactive", "Bajas"]];
  const stats = [
    { icon: "check" as IconName, c: "lime", label: "Inscritos", val: String(counts.active), sub: "Admisión activa" },
    { icon: "clock" as IconName, c: "yellow", label: "En trámite", val: String(counts.pending), sub: "Faltan documentos" },
    { icon: "students" as IconName, c: "forest", label: "Total", val: String(counts.all), sub: "Alumnos registrados" },
    { icon: "graduation" as IconName, c: "purple", label: "Niveles", val: String(levelsPresent.length), sub: "Kínder a Preparatoria" },
  ];

  return (
    <div style={{ maxWidth: 1320, margin: "0 auto", padding: "24px 28px 96px" }}>
      <button onClick={onBack} className="fade-up" style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14, cursor: "pointer" }}>
        <Icon name="back" size={18} stroke={2.4} />Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", justifyContent: "space-between", flexWrap: "wrap", gap: 16, marginBottom: 24 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--forest)", color: "#fff", boxShadow: "0 6px 16px color-mix(in oklab, var(--forest) 38%, transparent)" }}><Icon name="students" size={30} /></div>
          <div>
            <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Estudiantes</h1>
            <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{students.length} alumnos · Ciclo {cycleName}</p>
          </div>
        </div>
        <div style={{ display: "flex", gap: 12 }}>
          <Btn kind="ghost" icon="download" onClick={() => exportView()}>Exportar</Btn>
          <Btn icon="plus" onClick={() => setAdd(true)} style={{ height: 52, fontSize: 16.5 }}>Nuevo alumno</Btn>
        </div>
      </div>

      <div className="fade-up" style={{ display: "flex", flexWrap: "wrap", gap: 16, marginBottom: 26 }}>
        {stats.map((k, i) => (
          <div key={i} style={{ flex: "1 1 200px", minWidth: 0, display: "flex", alignItems: "center", gap: 14, padding: "18px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)" }}>
            <div style={{ width: 46, height: 46, borderRadius: 13, flexShrink: 0, display: "grid", placeItems: "center", background: `color-mix(in oklab, var(--${k.c}) 16%, var(--surface))`, color: `var(--${k.c})` }}><Icon name={k.icon} size={24} /></div>
            <div style={{ minWidth: 0 }}>
              <div style={{ fontSize: 13, color: "var(--text-muted)", fontWeight: 600, whiteSpace: "nowrap" }}>{k.label}</div>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 22, color: "var(--text)", lineHeight: 1.2 }}>{k.val}</div>
              <div style={{ fontSize: 12, color: "var(--text-faint)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{k.sub}</div>
            </div>
          </div>
        ))}
      </div>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap", marginBottom: 12 }}>
        <div style={{ display: "flex", gap: 6, padding: 5, background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)" }}>
          {tabs.map(([id, label]) => {
            const on = tab === id;
            return (
              <button key={id} onClick={() => setTab(id)} style={{ display: "inline-flex", alignItems: "center", gap: 8, height: 40, padding: "0 16px", borderRadius: 99, cursor: "pointer", background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", fontWeight: 600, fontSize: 14, fontFamily: "var(--font-display)", boxShadow: on ? "var(--shadow-card)" : "none" }}>
                {label}
                <span style={{ minWidth: 22, height: 20, padding: "0 6px", borderRadius: 99, background: on ? "var(--surface-3)" : "var(--surface)", color: "var(--text-muted)", fontSize: 12, display: "grid", placeItems: "center" }}>{counts[id]}</span>
              </button>
            );
          })}
        </div>
        <div style={{ position: "relative", flex: 1, minWidth: 230, maxWidth: 360 }}>
          <span style={{ position: "absolute", left: 16, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)" }}><Icon name="search" size={19} /></span>
          <input value={q} onChange={(e) => setQ(e.target.value)} aria-label="Buscar" placeholder="Buscar alumno, CURP o tutor…" style={{ width: "100%", height: 48, paddingLeft: 46, paddingRight: 16, borderRadius: "var(--r-pill)", border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 15, outline: "none" }} />
        </div>
      </div>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 18 }}>
        {["Todos", ...levelsPresent].map((l) => {
          const on = level === l;
          return <button key={l} onClick={() => setLevel(l)} style={{ height: 40, padding: "0 15px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", textTransform: l === "Todos" ? "none" : "capitalize", cursor: "pointer", border: `1.5px solid ${on ? "var(--forest)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--forest) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>{l}</button>;
        })}
        {anyFilter && <button onClick={clear} style={{ height: 38, padding: "0 14px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 6, fontSize: 13, fontWeight: 600, color: "var(--text-muted)", background: "var(--surface-3)", border: "1px solid var(--border)", cursor: "pointer" }}><Icon name="close" size={15} />Limpiar</button>}
      </div>

      {/* How many sit in each classroom, under whatever filter is on. It is the
          number a director counts by hand before deciding to open a group. */}
      {perGroup.length > 0 && (
        <div data-testid="per-group" className="fade-up" style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "wrap", marginBottom: 18 }}>
          <span style={{ fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em" }}>Por salón</span>
          {perGroup.map(([group, n]) => (
            <span key={group} style={{ display: "inline-flex", alignItems: "center", gap: 6, height: 30, padding: "0 12px", borderRadius: 99, background: "var(--surface-3)", border: "1px solid var(--border)", fontSize: 13, color: "var(--text-muted)" }}>
              {group}
              <b style={{ color: "var(--text)", fontFamily: "var(--font-display)" }}>{n}</b>
            </span>
          ))}
        </div>
      )}

      <div className="fade-up" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
        <div style={{ display: "grid", gridTemplateColumns: "2.4fr 1.1fr 1.6fr 1fr auto", gap: 16, padding: "14px 24px 12px", borderBottom: "1px solid var(--border)", fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em" }}>
          <div>Alumno</div><div>Nivel</div><div>Tutor</div><div>Estado</div><div style={{ width: 84 }} />
        </div>
        <div style={{ padding: 8 }}>
          {loading ? (
            <div style={{ padding: "56px", textAlign: "center", color: "var(--text-muted)" }}>Cargando…</div>
          ) : filtered.length === 0 ? (
            <div style={{ padding: "56px 20px", textAlign: "center" }}>
              <div style={{ width: 70, height: 70, borderRadius: 20, margin: "0 auto 16px", display: "grid", placeItems: "center", background: "color-mix(in oklab, var(--forest) 12%, var(--surface))", color: "var(--forest)" }}><Icon name="students" size={36} stroke={1.9} /></div>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, color: "var(--text)" }}>Sin alumnos en esta vista</div>
              <div style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 6 }}>Ajusta los filtros o da de alta un alumno nuevo.</div>
              <div style={{ display: "flex", gap: 12, justifyContent: "center", marginTop: 22 }}>
                <Btn kind="ghost" onClick={clear}>Limpiar filtros</Btn><Btn icon="plus" onClick={() => setAdd(true)}>Nuevo alumno</Btn>
              </div>
            </div>
          ) : (
            paged.map((st, i) => <StuRow key={st.id} st={st} i={i} onOpen={() => setSheet(st)} />)
          )}
        </div>
        {!loading && total > 0 && (
          <div style={{ display: "flex", alignItems: "center", gap: 14, flexWrap: "wrap", padding: "14px 24px", borderTop: "1px solid var(--border)" }}>
            <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>Mostrando <b style={{ color: "var(--text)" }}>{from}–{to}</b> de {total}</span>
            <div style={{ flex: 1 }} />
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <span style={{ fontSize: 13, color: "var(--text-faint)" }}>Mostrar</span>
              <div style={{ display: "flex", gap: 4, padding: 4, background: "var(--surface-3)", borderRadius: 99, border: "1px solid var(--border)" }}>
                {[20, 50, 100].map((n) => {
                  const on = pageSize === n;
                  return <button key={n} onClick={() => setPageSize(n)} style={{ height: 32, minWidth: 42, padding: "0 12px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", background: on ? "var(--surface)" : "transparent", color: on ? "var(--text)" : "var(--text-muted)", boxShadow: on ? "var(--shadow-card)" : "none" }}>{n}</button>;
                })}
              </div>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <button onClick={() => setPage(Math.max(0, cur - 1))} disabled={cur === 0} aria-label="Anterior" style={pager(cur === 0)}><Icon name="back" size={16} stroke={2.4} />Anterior</button>
              <span style={{ fontSize: 13.5, color: "var(--text-muted)", fontFamily: "var(--font-display)", fontWeight: 600, minWidth: 52, textAlign: "center" }}>{cur + 1} / {pageCount}</span>
              <button onClick={() => setPage(Math.min(pageCount - 1, cur + 1))} disabled={cur >= pageCount - 1} aria-label="Siguiente" style={pager(cur >= pageCount - 1)}>Siguiente<Icon name="chevron" size={16} stroke={2.4} /></button>
            </div>
          </div>
        )}
      </div>

      <button onClick={() => setAdd(true)} title="Nuevo alumno" style={{ position: "fixed", right: 32, bottom: 32, zIndex: 50, height: 60, padding: "0 26px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 10, background: "var(--primary)", color: "var(--primary-ink)", fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16.5, cursor: "pointer", boxShadow: "0 10px 30px color-mix(in oklab, var(--lime) 45%, transparent)" }}><Icon name="plus" size={22} stroke={2.6} />Nuevo alumno</button>

      {add && <QuickAdd onClose={() => setAdd(false)} onSave={onSave} levels={levels} />}
      {sheet !== null && <StudentSheet st={sheet} groups={groups} onClose={() => setSheet(null)} onEnroll={onEnroll} />}
      {toast !== null && (
        <div style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--forest)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)", animation: "pop .25s var(--ease) both" }}>
          <Icon name="check" size={20} stroke={2.6} />{toast}
        </div>
      )}
    </div>
  );
}

function StuRow({ st, i, onOpen }: { st: Student; i: number; onOpen: () => void }): ReactElement {
  const [hov, setHov] = useState(false);
  const s = statusOf(st.admissionStatus);
  return (
    <div onMouseEnter={() => setHov(true)} onMouseLeave={() => setHov(false)} onClick={onOpen} style={{ display: "grid", gridTemplateColumns: "2.4fr 1.1fr 1.6fr 1fr auto", alignItems: "center", gap: 16, padding: "13px 24px", borderRadius: "var(--r-md)", position: "relative", cursor: "pointer", background: hov ? "var(--surface-2)" : "transparent", transition: "background .15s var(--ease)" }}>
      <span style={{ position: "absolute", left: 4, top: 10, bottom: 10, width: 4, borderRadius: 99, background: s.dot }} />
      <div style={{ display: "flex", alignItems: "center", gap: 13, minWidth: 0, paddingLeft: 8 }}>
        <Avatar name={`${st.firstName} ${st.lastName}`} color={AV[i % AV.length]} size={42} />
        <div style={{ minWidth: 0 }}>
          <div style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{st.firstName} {st.lastName}</div>
          <div style={{ fontSize: 12.5, color: "var(--text-faint)", fontFamily: "monospace" }}>{st.curp}</div>
        </div>
      </div>
      <div style={{ fontSize: 14, color: "var(--text-muted)", textTransform: "capitalize" }}>{st.level}<div style={{ fontSize: 12.5, color: "var(--text-faint)" }}>{st.gradeGroup}</div></div>
      <div style={{ minWidth: 0, fontSize: 14, color: "var(--text)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{st.guardianName}</div>
      <div><StatusPill status={st.admissionStatus} /></div>
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 6, width: 84 }}>
        <button onClick={(ev) => { ev.stopPropagation(); onOpen(); }} title="Ver ficha" aria-label={`Ver ficha de ${st.firstName} ${st.lastName}`} style={qb}><Icon name="chevron" size={18} /></button>
      </div>
    </div>
  );
}
