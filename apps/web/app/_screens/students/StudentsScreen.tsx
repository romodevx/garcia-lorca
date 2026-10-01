"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type ReactElement } from "react";
import { Avatar } from "../../_design/Brand";
import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { SelectField, TextArea, TextField, useFocusFirstInvalid } from "../../_design/form";
import { Icon, type IconName } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import type { ColorKey } from "../../_design/colors";
import {
  createStudent,
  enroll,
  listEnrollments,
  listStudents,
  transfer,
  type AdmissionStatus,
  type CreateStudentInput,
  type Enrollment,
  type Student,
} from "../../students/api";
import { listCycles, listGradeLevels, listLabelledGroups, listLevels, type Cycle, type GradeLevel, type LabelledGroup, type Level } from "../../config/api";
import { addNote, enrollIntoGrade, reissueCard, revokeCard, StudentsError, type Placement } from "../../students/api";
import { VerifyCardDrawer } from "./VerifyCardDrawer";

import { API_URL } from "../../api-url";
import { csvName, downloadCsv, toCsv, type CsvColumn } from "../../_lib/csv";
import { ContactsPanel } from "./ContactsPanel";
import { formatStudentName } from "../../_design/students/studentName";
import { Checkmark, StudentFilters, StudentSelectionBand } from "../../_design/students/StudentFilters";
import { useStudentFilter } from "../../_design/students/useStudentFilter";
import { EMPTY_FILTER, filterFromParams, filterToParams, type FilterableStudent, type StudentFilterState } from "../../_design/students/studentFilter";
import { placeStudents } from "../../_design/students/placement";
import { fold } from "@garcia-lorca/ui/command-core";

/** The floating "Nuevo alumno": its height, its distance from the bottom, and
 * the breathing room the page leaves above it. */
const FAB_HEIGHT = 60;
const FAB_BOTTOM = 32;
const FAB_GAP = 16;


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

// A pupil whose level was deleted (ADR 0021) keeps no level, grade or group:
// the office has to find them to reassign them, so the list names the gap.
const NO_LEVEL = "Sin nivel";
const NO_GROUP = "Sin grupo";
const levelOf = (s: Student): string => (s.level.trim() === "" ? NO_LEVEL : s.level);
const groupOf = (s: Student): string => (s.gradeGroup.trim() === "" ? NO_GROUP : s.gradeGroup);

const AV: ColorKey[] = ["lime", "forest", "orange", "purple", "yellow"];

type StatusTab = "all" | "active" | "pending" | "inactive";
const STATUS_TABS: readonly StatusTab[] = ["all", "active", "pending", "inactive"];
/** The status tab's key in the address bar, beside the shared filter's. */
const TAB_KEY = "estado";

/** A row of the list: the pupil, where they sit this cycle, and their classroom's name. */
interface Row extends FilterableStudent {
  student: Student;
  /** The level the chips file the pupil under, by name ("Sin nivel" when none). */
  levelName: string;
  /** "1° PREESCOLAR · B" when the cycle's group is known; else the file's letter. */
  room: string;
}

/** Whether the cycle's groups and enrolments were read: grades and groups hang on it. */
type PlacementState = "loading" | "ok" | "failed";

/** The list's address as last written, so the edit page's own Back returns to it. */
let lastListPath = "";

/** Set by a deliberate visit to Estudiantes (the palette, a menu): open clean. */
let freshVisit = false;

/**
 * The next list opens with no filters, whatever the address bar still says:
 * the shell marks every deliberate navigation (true only for Estudiantes, so
 * a stale mark never survives the next one). Browser Back and the edit page's
 * Back do not pass here, so they keep the view.
 */
export function markStudentsVisit(fresh: boolean): void {
  freshVisit = fresh;
}

/** Where "Volver" from a pupil's page goes: the list as it was left, filters included. */
export function studentsListPath(): string {
  return lastListPath === "" ? "/alumnos" : lastListPath;
}

/** The view the address bar names: Back from a pupil's page lands on it again. */
function readView(): { filter: StudentFilterState; tab: StatusTab } {
  // The flag is spent after mount, not here: StrictMode runs this twice.
  if (typeof window === "undefined" || freshVisit) return { filter: EMPTY_FILTER, tab: "all" };
  const params = new URLSearchParams(window.location.search);
  const tab = STATUS_TABS.find((t) => t === params.get(TAB_KEY)) ?? "all";
  return { filter: filterFromParams(params), tab };
}

/** Writes the view into the address bar without a new history entry per click. */
function writeView(filter: StudentFilterState, tab: StatusTab): void {
  const base = new URLSearchParams(window.location.search);
  if (tab === "all") base.delete(TAB_KEY);
  else base.set(TAB_KEY, tab);
  const query = filterToParams(filter, base).toString();
  const next = `${window.location.pathname}${query === "" ? "" : `?${query}`}`;
  lastListPath = next;
  if (next !== `${window.location.pathname}${window.location.search}`) {
    window.history.replaceState(window.history.state, "", next);
  }
}

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
  /** The first note failed too: a retry cannot write it (the CURP is taken). */
  readonly noteFailed: boolean;

  constructor(cause: unknown, noteFailed = false) {
    super("placement failed");
    this.name = "PlacementFailed";
    this.cause = cause;
    this.noteFailed = noteFailed;
  }
}

const NOTE_LOST = "La nota no se guardó: agrégala en su ficha.";

/** The shape the quick-add drawer validates before it asks the server. */
export interface QuickAddDraft {
  curp: string;
  firstName: string;
  /** The paternal surname. The maternal one is optional and never "missing". */
  lastName: string;
  guardianName: string;
  /** The config grade the pupil joins. The GROUP is chosen by capacity, not here. */
  gradeLevelId?: string;
}

/** A CURP is exactly eighteen characters. The server checks its shape too. */
const CURP_LENGTH = 18;

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
  return Object.values(missingFields(draft))[0];
}

/**
 * Lo mismo, pero por campo, para poder marcarlos TODOS a la vez.
 *
 * Devolver un solo mensaje era mejor que el botón gris y mudo que reemplazó,
 * pero seguía obligando a guardar cinco veces para descubrir los cinco datos
 * que faltaban. El texto de cada mensaje se conserva tal cual: un CURP de 17
 * caracteres no está "vacío", y decir cuántos lleva es justo lo que esta
 * pantalla ya sabía hacer bien.
 *
 * El orden de las llaves es el orden de lectura de la forma, así que el primero
 * que devuelve `missingFieldMessage` sigue siendo el de más arriba.
 */
export function missingFields(draft: QuickAddDraft): Record<string, string> {
  const found: Record<string, string> = {};
  const curp = draft.curp.trim();
  if (curp === "") {
    found.curp = "Escribe el CURP del alumno.";
  } else if (curp.length !== CURP_LENGTH) {
    found.curp = `El CURP debe tener ${CURP_LENGTH} caracteres; llevas ${curp.length}.`;
  }
  // Surname before first name: the order the form asks for them (T3).
  if (draft.lastName.trim().length < 2) found.lastName = "Escribe el apellido paterno del alumno.";
  if (draft.firstName.trim().length < 2) found.firstName = "Escribe el nombre del alumno.";
  if (draft.guardianName.trim().length < 3) found.guardianName = "Escribe el nombre del tutor.";
  if (draft.gradeLevelId !== undefined && draft.gradeLevelId === "") {
    found.gradeLevelId = "Elige el nivel y el grado al que entra.";
  }
  return found;
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
  withNote,
}: {
  onClose: () => void;
  onSave: (input: CreateStudentInput, gradeLevelId: string, note: string) => Promise<void>;
  levels: readonly Level[];
  /** students:notes_write: offer an optional first note (task 6). */
  withNote: boolean;
}): ReactElement {
  const [note, setNote] = useState("");
  const [curp, setCurp] = useState("");
  const [firstName, setFirst] = useState("");
  const [lastName, setLast] = useState("");
  const [maternalLastName, setMaternal] = useState("");
  const [levelId, setLevelId] = useState("");
  const [gradeLevelId, setGradeLevelId] = useState("");
  const [grades, setGrades] = useState<GradeLevel[]>([]);
  const [guardianName, setGuardian] = useState("");
  // Los faltantes vienen de missingFields, que conserva el mensaje propio de
  // cada campo ("llevas 17 caracteres"), así que no se usa useRequired: ese
  // hook da un mensaje genérico y aquí ya había uno mejor.
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const señalarPrimerError = useFocusFirstInvalid();
  const clearField = (key: string): void =>
    setFieldErrors((prev) => {
      if (prev[key] === undefined) return prev;
      const rest = { ...prev };
      delete rest[key];
      return rest;
    });
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
    // Se valida al PRESIONAR, no apagando el botón. Un control que se niega y no
    // dice por qué es justo lo que esto reemplazó — y ahora se marcan todos los
    // faltantes a la vez, en sus propios campos.
    const missing = missingFields({ curp, firstName, lastName, guardianName, gradeLevelId });
    setFieldErrors(missing);
    if (Object.keys(missing).length > 0) {
      // Marcarlos no basta: el último campo del cajón queda DEBAJO del pie
      // pegajoso, así que su rojo no se ve y el foco, si se queda en el botón,
      // tampoco lleva allí.
      señalarPrimerError();
      return;
    }
    const chosenLevel = levels.find((l) => l.id === levelId);
    if (chosenLevel === undefined) {
      setError("Elige el nivel y el grado al que entra.");
      return;
    }
    setSaving(true);
    setError(undefined);
    try {
      // The level travels as the config level's ID: whichever levels the school
      // defined take pupils (ADR 0014), and the server supplies the name.
      await onSave(
        {
          curp: curp.trim().toUpperCase(),
          firstName: firstName.trim(),
          lastName: lastName.trim(),
          maternalLastName: maternalLastName.trim(),
          levelId: chosenLevel.id,
          // Left empty on purpose: the server writes it when capacity picks the
          // group, so the record cannot claim a group the pupil is not in.
          gradeGroup: "",
          guardianName: guardianName.trim(),
        },
        gradeLevelId,
        withNote ? note.trim() : "",
      );
    } catch (err) {
      setError(
        err instanceof PlacementFailed
          ? [placementFailureMessage(err.cause), err.noteFailed ? NOTE_LOST : ""].filter((m) => m !== "").join(" ")
          : createFailureMessage(err),
      );
      setSaving(false);
    }
  };

  return (
    <DrawerShell
      title="Nuevo alumno"
      subtitle="Alta rápida · la admisión inicia en trámite"
      icon="students"
      onClose={onClose}
      footer={
        <>
          <Btn kind="ghost" onClick={onClose}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          {/* "Crear", no "Guardar": esto hace un alumno que no existía. Y se
              apaga SÓLO mientras la petición va en camino — una forma
              incompleta recibe sus campos en rojo, no un control muerto. */}
          <Btn icon="check" onClick={() => void submit()} disabled={saving} style={{ opacity: saving ? 0.45 : 1, height: 52, fontSize: 16 }}>
            {saving ? "Creando…" : "Crear alumno"}
          </Btn>
        </>
      }
    >
      <>
          <TextField
            label="CURP"
            value={curp}
            onChange={setCurp}
            uppercase
            required
            help="Los 18 caracteres de la CURP del alumno, tal como vienen en el acta. Es lo que impide que el mismo alumno se registre dos veces."
            error={fieldErrors.curp}
            onClearError={() => clearField("curp")}
          />
          {/* Surnames first, as the school reads its lists (T3). The two
              surnames are one logical entity, so they may sit side by side. */}
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: 14 }}>
            <TextField
              label="Apellido paterno"
              value={lastName}
              onChange={setLast}
              uppercase
              required
              help="El primer apellido del alumno, el del padre."
              error={fieldErrors.lastName}
              onClearError={() => clearField("lastName")}
            />
            <TextField
              label="Apellido materno"
              value={maternalLastName}
              onChange={setMaternal}
              uppercase
              help="El segundo apellido, el de la madre. Déjalo vacío si el acta no lo trae."
            />
          </div>
          <TextField
            label="Nombre(s)"
            value={firstName}
            onChange={setFirst}
            uppercase
            required
            help="Sólo los nombres de pila, sin apellidos."
            error={fieldErrors.firstName}
            onClearError={() => clearField("firstName")}
          />
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(240px, 1fr))", gap: 14 }}>
            <SelectField
              label="Nivel"
              value={levelId}
              onChange={setLevelId}
              required
              help="El nivel educativo donde va a cursar: preescolar, primaria o secundaria."
              options={levels.map((l) => ({ value: l.id, label: l.name }))}
            />
            <SelectField
              label="Grado"
              value={gradeLevelId}
              onChange={setGradeLevelId}
              required
              disabled={grades.length === 0}
              help="El grado dentro del nivel. Se enciende en cuanto eliges el nivel."
              options={grades.map((g) => ({ value: g.id, label: g.name }))}
              error={fieldErrors.gradeLevelId}
              onClearError={() => clearField("gradeLevelId")}
            />
          </div>
          <TextField
            label="Tutor"
            value={guardianName}
            onChange={setGuardian}
            uppercase
            required
            help="Quién responde por el alumno ante el colegio: nombre completo de la madre, el padre o el tutor legal."
            error={fieldErrors.guardianName}
            onClearError={() => clearField("guardianName")}
          />
          {withNote && (
            <TextArea
              label="Nota"
              value={note}
              onChange={setNote}
              rows={2}
              help="Opcional. Algo que el colegio deba recordar de este alumno; queda en su ficha, con tu nombre."
            />
          )}
          <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
            <span style={{ color: "var(--forest)", flexShrink: 0, marginTop: 1 }}><Icon name="check" size={19} stroke={2.6} /></span>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>El grupo <strong>se asigna solo</strong>, según el cupo de cada salón: se llena A y el siguiente alumno pasa a B. Los documentos se cargan después.</p>
          </div>
          {error !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{error}</p>}
      </>
    </DrawerShell>
  );
}

// ── detail sheet ──
/**
 * Why a placement did not go through, in words the office can act on.
 *
 * "No se pudo inscribir" alone sent people back to press the same button: the
 * commonest cause was a pupil the quick-add had ALREADY placed (409), and the
 * next was a group of another level, which the server explains itself.
 */
export function placementRefusal(err: unknown): string {
  if (!(err instanceof StudentsError)) return PLACEMENT_FALLBACK;
  const said = SERVER_REFUSALS[err.detail];
  if (said !== undefined) return said;
  // What the server wrote in Spanish (the level check, a withdrawn file, a
  // full group) is already addressed to the office; English never is.
  if (err.detail !== "" && /[áéíóúñ]|\b(el|la|ese|no|ya)\b/i.test(err.detail)) {
    return err.detail.charAt(0).toUpperCase() + err.detail.slice(1) + ".";
  }
  return PLACEMENT_FALLBACK;
}

const PLACEMENT_FALLBACK = "No se pudo guardar el cambio de grupo. Intenta de nuevo.";

/** The server's English refusals, in words the office can act on. */
const SERVER_REFUSALS: Record<string, string> = {
  "student is already enrolled this cycle": "Ya está inscrito en un grupo este ciclo. Usa «Cambiar de grupo».",
  "cannot transfer across cycles": "Ese grupo es de otro ciclo.",
  "enrollment is not active": "Su inscripción ya no está activa.",
  "group not found": "Ese grupo ya no existe. Cierra la ficha y vuelve a abrirla.",
  "already in that group": "Ya está en ese grupo.",
};

/**
 * The pupil's sheet.
 *
 * A pupil already placed this cycle (the quick-add places them) is shown the
 * group they are in and offered a TRANSFER, with a reason, within their level —
 * never a second enrolment, which the server refuses. A pupil with no group is
 * offered an enrolment. Either way only groups of the pupil's own level appear:
 * four groups can all be called "1° A", one per level.
 */
function StudentSheet({
  st,
  groups,
  cycleId,
  gradeLevels,
  gradeLevelsFailed,
  onClose,
  onPlaced,
  onEdit,
  canRevokeCard = false,
}: {
  st: Student;
  groups: readonly LabelledGroup[];
  /** The cycle whose groups are shown: the pupil's enrolment in THIS cycle. */
  cycleId: string;
  gradeLevels: readonly GradeLevel[];
  gradeLevelsFailed: boolean;
  onClose: () => void;
  onPlaced: (message: string) => void;
  /** Absent when the caller cannot edit a pupil's file. */
  onEdit?: () => void;
  /** module_student_card and students:update: the card can be revoked here. */
  canRevokeCard?: boolean;
}): ReactElement {
  const rows: [string, string][] = [
    ["Matrícula", st.enrollmentNumber ?? ""],
    ["CURP", st.curp],
    ["Nivel", `${levelOf(st)} · ${groupOf(st)}`],
    ["Tutor", st.guardianName],
    ["Admisión", statusOf(st.admissionStatus).label],
  ];
  // undefined = still reading, null = no group this cycle, "failed" = unknown.
  const [current, setCurrent] = useState<Enrollment | null | undefined | "failed">(undefined);
  const [gid, setGid] = useState("");
  const [reason, setReason] = useState("");
  const [reasonError, setReasonError] = useState<string | undefined>(undefined);
  const [refusal, setRefusal] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let alive = true;
    // An enrolment from last year stays "active" (a cycle's end closes nothing),
    // so only this cycle's counts.
    listEnrollments(API_URL, { studentId: st.id, cycleId })
      .then((all) => { if (alive) setCurrent(all.find((e) => e.status === "active") ?? null); })
      // Not knowing is not "no group": offering "Inscribir" then would bring
      // back the second-enrolment 409 this sheet was fixed for.
      .catch(() => { if (alive) setCurrent("failed"); });
    return () => { alive = false; };
  }, [st.id, cycleId]);

  const levelOfGrade = useMemo(() => new Map(gradeLevels.map((g) => [g.id, g.levelId])), [gradeLevels]);
  // A legacy pupil with no level id has nothing to filter by: every group shows.
  const ownLevel = groups.filter((g) => st.levelId === "" || levelOfGrade.get(g.gradeLevelId) === st.levelId);
  const placed = current !== null && current !== undefined && current !== "failed" ? current : undefined;
  const currentGroup = placed === undefined ? undefined : groups.find((g) => g.id === placed.groupId);
  const choices = placed !== undefined ? ownLevel.filter((g) => g.id !== placed.groupId) : ownLevel;
  const transferring = placed !== undefined;
  const ready = current !== undefined && current !== "failed" && !gradeLevelsFailed;

  // A ref, not the `saving` state: a double-click fires twice before React
  // re-renders, and the second click must already see the first in flight.
  const inFlight = useRef(false);
  const save = async (): Promise<void> => {
    if (inFlight.current) return;
    setRefusal(undefined);
    if (gid === "") {
      setRefusal("Elige un grupo.");
      return;
    }
    if (transferring && reason.trim() === "") {
      setReasonError("Escribe por qué cambia de grupo.");
      return;
    }
    inFlight.current = true;
    setSaving(true);
    try {
      if (placed !== undefined) {
        await transfer(API_URL, { enrollmentId: placed.id, toGroupId: gid, reason: reason.trim() });
        onPlaced("Alumno cambiado de grupo");
      } else {
        await enroll(API_URL, { studentId: st.id, groupId: gid });
        onPlaced("Alumno inscrito en el grupo");
      }
    } catch (err) {
      setRefusal(placementRefusal(err));
    } finally {
      inFlight.current = false;
      setSaving(false);
    }
  };

  return (
    <DrawerShell
      title="Ficha del alumno"
      icon="students"
      size="sm"
      onClose={onClose}
      header={
        <div style={{ padding: "26px", borderBottom: "1px solid var(--border)", display: "flex", alignItems: "center", gap: 16 }}>
          <Avatar name={formatStudentName(st)} color="forest" size={62} />
          <div style={{ flex: 1, minWidth: 0 }}>
            <h2 style={{ fontSize: 20, fontWeight: 700, color: "var(--text)" }}>{formatStudentName(st)}</h2>
            <div style={{ marginTop: 6 }}><StatusPill status={st.admissionStatus} /></div>
          </div>
          <button type="button" onClick={onClose} aria-label="Cerrar" style={{ width: 40, height: 40, borderRadius: "50%", display: "grid", placeItems: "center", background: "var(--surface-3)", border: "1px solid var(--border)", color: "var(--text-muted)", cursor: "pointer" }}><Icon name="close" size={19} /></button>
        </div>
      }
      footer={
        <>
          <Btn kind="ghost" icon="documents" style={{ flex: 1 }}>Documentos</Btn>
          {onEdit !== undefined && <Btn kind="ghost" onClick={onEdit} style={{ flex: 1 }}>Editar ficha</Btn>}
          {ready && (
            <Btn icon="check" onClick={() => void save()} style={{ flex: 1, opacity: saving ? 0.6 : 1 }}>
              {transferring ? "Cambiar de grupo" : "Inscribir"}
            </Btn>
          )}
        </>
      }
    >
      <>
          {rows.map(([k, v]) => (
            <div key={k} style={{ display: "flex", justifyContent: "space-between", gap: 16, padding: "15px 0", borderBottom: "1px solid var(--border)" }}>
              <span style={{ fontSize: 14, color: "var(--text-muted)" }}>{k}</span>
              <span style={{ fontSize: 14.5, fontWeight: 600, color: "var(--text)", textAlign: "right" }}>{v}</span>
            </div>
          ))}
          <ContactsPanel studentId={st.id} />
          {canRevokeCard && <RevokeCard studentId={st.id} />}
          {current === "failed" && (
            <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, padding: "18px 0 0" }}>
              No pudimos leer su inscripción de este ciclo. Cierra la ficha y vuelve a abrirla.
            </p>
          )}
          {current !== "failed" && gradeLevelsFailed && (
            <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, padding: "18px 0 0" }}>
              No pudimos leer los grados de la escuela, así que no sabemos qué grupos son de su nivel. Recarga la página.
            </p>
          )}
          {ready && (
            <div style={{ padding: "18px 0", display: "grid", gap: 10 }}>
              {transferring && (
                <p style={{ fontSize: 14, color: "var(--text)" }}>
                  <span style={{ color: "var(--text-muted)" }}>Grupo actual: </span>
                  <strong>{currentGroup?.label ?? st.gradeGroup}</strong>
                </p>
              )}
              <span style={{ fontSize: 13.5, fontWeight: 600, color: "var(--text-muted)" }}>
                {transferring ? "Cambiar a" : "Inscribir en grupo"}
              </span>
              {choices.length === 0 ? (
                <p style={{ fontSize: 13.5, color: "var(--text-faint)" }}>
                  {transferring ? "No hay otro grupo de su nivel en este ciclo." : "No hay grupos de su nivel en este ciclo: créalos en Configuración."}
                </p>
              ) : (
                <div role="group" aria-label={transferring ? "Cambiar a" : "Inscribir en"} style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
                  {choices.map((g) => {
                    const on = gid === g.id;
                    return <button key={g.id} type="button" aria-pressed={on} onClick={() => { setGid(g.id); setRefusal(undefined); }} style={{ height: 44, padding: "0 15px", borderRadius: 99, fontSize: 13.5, fontWeight: 600, fontFamily: "var(--font-display)", cursor: "pointer", border: `1.5px solid ${on ? "var(--lime)" : "var(--border)"}`, background: on ? "color-mix(in oklab, var(--lime) 12%, var(--surface))" : "var(--surface)", color: on ? "var(--forest)" : "var(--text-muted)" }}>{g.label}</button>;
                  })}
                </div>
              )}
              {transferring && (
                <TextField
                  label="Motivo del cambio"
                  value={reason}
                  onChange={setReason}
                  required
                  help="Por qué cambia de grupo: queda en su historial."
                  error={reasonError}
                  onClearError={() => setReasonError(undefined)}
                />
              )}
              {refusal !== undefined && (
                <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{refusal}</p>
              )}
            </div>
          )}
      </>
    </DrawerShell>
  );
}

export interface StudentsScreenProps {
  onBack: () => void;
  /** Opens a pupil's edit page. Absent when the caller cannot edit files. */
  onEdit?: (studentId: string) => void;
  /** students:notes_write: the quick-add offers a first note. */
  canWriteNotes?: boolean;
  /** module_student_card: the staff verify a scanned card (task 13). */
  canVerifyCard?: boolean;
  /** module_student_card and students:update: a card can be revoked. */
  canRevokeCard?: boolean;
}

/** What the school reads in Excel, in the order the table shows it. */
const EXPORT_COLUMNS: readonly CsvColumn<Row>[] = [
  { label: "Apellido paterno", value: (r) => r.student.lastName },
  { label: "Apellido materno", value: (r) => r.student.maternalLastName ?? "" },
  { label: "Nombre(s)", value: (r) => r.student.firstName },
  { label: "CURP", value: (r) => r.student.curp },
  { label: "Nivel", value: (r) => r.levelName },
  { label: "Grado y grupo", value: (r) => r.room },
  { label: "Tutor", value: (r) => r.student.guardianName },
  { label: "Estado", value: (r) => statusOf(r.student.admissionStatus).label },
];

export function StudentsScreen({ onBack, onEdit, canWriteNotes = false, canVerifyCard = false, canRevokeCard = false }: StudentsScreenProps): ReactElement {
  const [verifying, setVerifying] = useState(false);
  const [students, setStudents] = useState<Student[]>([]);
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [levels, setLevels] = useState<Level[]>([]);
  const [groups, setGroups] = useState<LabelledGroup[]>([]);
  // Every level's grades, so the sheet can tell which level a group belongs to.
  const [gradeLevels, setGradeLevels] = useState<GradeLevel[]>([]);
  const [gradeLevelsFailed, setGradeLevelsFailed] = useState(false);
  const [gradeLevelsLoaded, setGradeLevelsLoaded] = useState(false);
  // The cycle whose groups the sheet offers (the active one, or the first).
  const [groupsCycleId, setGroupsCycleId] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [initialView] = useState(readView);
  useEffect(() => { freshVisit = false; }, []);
  const [tab, setTab] = useState<StatusTab>(initialView.tab);
  // The cycle's enrolments place each pupil in a grade and a group (T4).
  const [enrollments, setEnrollments] = useState<Enrollment[]>([]);
  const [placement, setPlacement] = useState<PlacementState>("loading");
  const [rollVersion, setRollVersion] = useState(0);
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
    // Three states, not two. An empty roll and a roll we could not read look
    // identical on screen and mean opposite things — "no pupils match" versus
    // "we do not know" — and the office reading the first when the truth is the
    // second goes looking for a problem that is not there.
    listStudents(API_URL)
      .then((rows) => { setStudents(rows); setLoadFailed(false); })
      .catch(() => setLoadFailed(true))
      .finally(() => setLoading(false));
    setRollVersion((v) => v + 1);
  }, []);
  useEffect(() => {
    reload();
    listLevels(API_URL).then(setLevels).catch(() => setLevels([]));
    listGradeLevels(API_URL, "")
      .then((all) => { setGradeLevels(all); setGradeLevelsFailed(false); setGradeLevelsLoaded(true); })
      .catch(() => setGradeLevelsFailed(true));
    listCycles(API_URL).then((cs) => {
      setCycles(cs);
      const active = cs.find((c) => c.status === "active") ?? cs[0];
      if (active !== undefined) setGroupsCycleId(active.id);
      else setPlacement("failed");
    }).catch(() => setPlacement("failed"));
  }, [reload]);

  // The cycle's groups and ONE read of its enrolments, again whenever the roll
  // is re-read (a pupil placed or moved). Unreadable: the chips offer levels
  // only and say why — never a grade row where everybody counts as "Sin grado".
  useEffect(() => {
    if (groupsCycleId === "") return;
    let alive = true;
    void Promise.allSettled([listLabelledGroups(API_URL, groupsCycleId), listEnrollments(API_URL, { cycleId: groupsCycleId })])
      .then(([gs, es]) => {
        if (!alive) return;
        if (gs.status === "fulfilled") setGroups(gs.value);
        if (es.status === "fulfilled") setEnrollments(es.value);
        setPlacement(gs.status === "fulfilled" && es.status === "fulfilled" ? "ok" : "failed");
      });
    return () => { alive = false; };
  }, [groupsCycleId, rollVersion]);

  const counts = useMemo(() => ({
    all: students.length,
    active: students.filter((s) => statusOf(s.admissionStatus).tab === "active").length,
    pending: students.filter((s) => statusOf(s.admissionStatus).tab === "pending").length,
    inactive: students.filter((s) => statusOf(s.admissionStatus).tab === "inactive").length,
  }), [students]);
  const levelsPresent = useMemo(() => Array.from(new Set(students.map(levelOf))), [students]);


  // The school's levels; when they cannot be read, the ones on the roll, so
  // the level chips never vanish over a failed call.
  const catalogLevels = useMemo(() => {
    if (levels.length > 0) return levels;
    const onRoll = new Map<string, string>();
    for (const s of students) if (s.levelId !== "") onRoll.set(s.levelId, s.level);
    return [...onRoll].map(([id, name]) => ({ id, name }));
  }, [levels, students]);
  // Grades and groups need three reads: the school's grades, the cycle's
  // groups and its enrolments. One failed = failed; one pending = loading.
  const placementStatus: PlacementState =
    placement === "failed" || gradeLevelsFailed ? "failed" : placement === "ok" && gradeLevelsLoaded ? "ok" : "loading";
  const placementReady = placementStatus === "ok";
  // Until the cycle is placed there are no grade or group chips to offer.
  const catalog = useMemo(
    () => ({ levels: catalogLevels, grades: placementReady ? gradeLevels : [], groups: placementReady ? groups : [] }),
    [catalogLevels, gradeLevels, groups, placementReady],
  );

  // Every pupil placed once per data change, not per tab click.
  const placedRows = useMemo((): Row[] => {
    // A pupil from before level ids carries only the NAME; match it (ADR 0014).
    const idOfName = new Map(catalogLevels.map((l) => [fold(l.name), l.id]));
    const nameOf = new Map(catalogLevels.map((l) => [l.id, l.name]));
    const files = students.map((s) => ({ id: s.id, levelId: s.levelId !== "" ? s.levelId : (idOfName.get(fold(s.level)) ?? "") }));
    const placed = placeStudents(files, { enrollments: placementReady ? enrollments : [], groups, grades: gradeLevels });
    const labelOf = new Map(groups.map((g) => [g.id, g.label]));
    return students.map((s) => {
      const p = placed.get(s.id) ?? { levelId: s.levelId, gradeLevelId: "", groupId: "" };
      return {
        id: s.id,
        firstName: s.firstName,
        lastName: s.lastName,
        maternalLastName: s.maternalLastName,
        ...p,
        student: s,
        searchText: [s.curp, s.guardianName],
        levelName: nameOf.get(p.levelId) ?? levelOf(s),
        // Placed: this cycle's classroom, or honestly none. Not yet known: the file's letter.
        room: placementReady ? (labelOf.get(p.groupId) ?? NO_GROUP) : groupOf(s),
      };
    });
  }, [students, enrollments, groups, gradeLevels, catalogLevels, placementReady]);
  const rows = useMemo(
    () => placedRows.filter((r) => tab === "all" || statusOf(r.student.admissionStatus).tab === tab),
    [placedRows, tab],
  );
  const filter = useStudentFilter(rows, catalog, initialView.filter);
  const filtered = filter.filtered;

  useEffect(() => { writeView(filter.state, tab); }, [filter.state, tab]);

  // A grade or group from the URL cannot be honoured without the cycle's
  // enrolments: drop it (the level stays) rather than show an empty list.
  const { chooseGrade } = filter;
  const urlGrade = filter.state.gradeId;
  useEffect(() => {
    if (placementStatus === "failed" && urlGrade !== "") chooseGrade(urlGrade);
  }, [placementStatus, urlGrade, chooseGrade]);
  const waitingForPlacement = placementStatus === "loading" && (filter.state.gradeId !== "" || filter.state.groupId !== "");

  // The padrón as it is on screen: the tab, the chips and the search. A file
  // that quietly held every pupil would look right and be wrong.
  const exportRows = (list: readonly Row[]): void => {
    downloadCsv(csvName("alumnos", new Date()), toCsv(EXPORT_COLUMNS, list));
    say(`${list.length} alumnos exportados`);
  };

  // How many pupils sit in each classroom, over whatever filter is on. The
  // school asked for it and it is the number a director counts by hand before
  // deciding whether to open another group.
  const perGroup = useMemo(() => {
    const counted = new Map<string, number>();
    for (const s of filtered) {
      counted.set(s.room, (counted.get(s.room) ?? 0) + 1);
    }
    return [...counted.entries()].sort((a, b) => a[0].localeCompare(b[0], "es"));
  }, [filtered]);

  // Any filter change (or a new page size) sends the view back to the first page.
  const { levelId, gradeId, groupId, q } = filter.state;
  useEffect(() => { setPage(0); }, [tab, levelId, gradeId, groupId, q, pageSize]);

  const total = filtered.length;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));
  const cur = Math.min(page, pageCount - 1);
  const paged = filtered.slice(cur * pageSize, cur * pageSize + pageSize);
  const from = total === 0 ? 0 : cur * pageSize + 1;
  const to = Math.min(total, (cur + 1) * pageSize);

  const cycleName = cycles.find((c) => c.status === "active")?.name ?? cycles[0]?.name ?? "—";
  const clear = (): void => { setTab("all"); filter.clear(); };

  const onSave = async (input: CreateStudentInput, gradeLevelId: string, note: string): Promise<void> => {
    const created = await createStudent(API_URL, input);
    // The note goes right after the pupil exists and BEFORE placement: a full
    // grade keeps the drawer open, and a retry must not write the note twice.
    // A failed note does not undo the pupil; the message says so.
    const noteFailed = note !== "" && !(await addNote(API_URL, created.id, note).then(() => true, () => false));
    // Two calls, and the SECOND one is the one that can fail on a full grade —
    // by then the pupil exists. PlacementFailed carries that distinction to the
    // drawer, so the message can say "el alumno sí se creó" instead of sending
    // somebody back to re-type a CURP that is now taken.
    let placement: Placement;
    try {
      placement = await enrollIntoGrade(API_URL, created.id, gradeLevelId);
    } catch (err) {
      throw new PlacementFailed(err, noteFailed);
    }
    setAdd(false);
    reload();
    say(
      noteFailed
        ? `Alumno creado · ${formatStudentName(input)} quedó en ${placement.groupName}, pero la nota no se guardó: agrégala en su ficha`
        : `Alumno creado · ${formatStudentName(input)} quedó en ${placement.groupName}`,
    );
  };
  const onPlaced = (message: string): void => {
    setSheet(null);
    reload();
    say(message);
  };

  const tabs: [StatusTab, string][] = [["all", "Todos"], ["active", "Inscritos"], ["pending", "Trámite"], ["inactive", "Bajas"]];
  const stats = [
    { icon: "check" as IconName, c: "lime", label: "Inscritos", val: String(counts.active), sub: "Admisión activa" },
    { icon: "clock" as IconName, c: "yellow", label: "En trámite", val: String(counts.pending), sub: "Faltan documentos" },
    { icon: "students" as IconName, c: "forest", label: "Total", val: String(counts.all), sub: "Alumnos registrados" },
    { icon: "graduation" as IconName, c: "purple", label: "Niveles", val: String(levelsPresent.filter((l) => l !== NO_LEVEL).length), sub: "Kínder a Preparatoria" },
  ];

  return (
    <Page>
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
          {canVerifyCard && <Btn kind="ghost" icon="qr" onClick={() => setVerifying(true)}>Verificar credencial</Btn>}
          <Btn kind="ghost" icon="download" onClick={() => exportRows(filtered)}>Exportar</Btn>
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

      <div className="fade-up" style={{ marginBottom: 18 }}>
        <StudentFilters
          filter={filter}
          selectable
          searchHint="Buscar alumno, CURP o tutor…"
          extraActive={tab !== "all"}
          onClear={() => setTab("all")}
          leading={
            <div style={{ display: "flex", gap: 6, padding: 5, maxWidth: "100%", overflowX: "auto", background: "var(--surface-3)", borderRadius: "var(--r-pill)", border: "1px solid var(--border)" }}>
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
          }
        />
        {placementStatus === "failed" && (
          <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: "10px 0 0" }}>
            No pudimos leer los grupos de este ciclo: por ahora sólo se filtra por nivel. Recarga la página en un momento.
          </p>
        )}
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

      {/* What "select all" is for today: taking exactly that set to Excel. */}
      <div style={{ marginBottom: filter.selected.size > 0 ? 14 : 0 }}>
        <StudentSelectionBand filter={filter}>
          <Btn kind="ghost" icon="download" onClick={() => exportRows(filter.selectedItems)}>Exportar selección</Btn>
        </StudentSelectionBand>
      </div>

      <div className="fade-up" style={{ background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", boxShadow: "var(--shadow-card)", overflow: "hidden" }}>
        <div className="students-grid" style={{ display: "grid", gridTemplateColumns: ROW_COLUMNS, gap: 16, padding: "14px 24px 12px", borderBottom: "1px solid var(--border)", fontSize: 12.5, fontWeight: 700, color: "var(--text-faint)", textTransform: "uppercase", letterSpacing: ".04em" }}>
          <div style={{ width: CHECK_CELL }} /><div>Alumno</div><div className="students-col-wide">Nivel</div><div className="students-col-wide">Tutor</div><div className="students-col-wide">Estado</div><div style={{ width: 84 }} />
        </div>
        <div style={{ padding: 8 }}>
          {loading || waitingForPlacement ? (
            <div style={{ padding: "56px", textAlign: "center", color: "var(--text-muted)" }}>Cargando…</div>
          ) : loadFailed ? (
            <div role="alert" style={{ padding: "56px 20px", textAlign: "center" }}>
              <div style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, color: "var(--bad-fg)" }}>No pudimos cargar los alumnos</div>
              <div style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 6 }}>
                No es que no haya — no los pudimos leer. Vuelve a intentar en un momento.
              </div>
              <div style={{ display: "flex", gap: 12, justifyContent: "center", marginTop: 22 }}>
                <Btn kind="ghost" onClick={() => { setLoading(true); reload(); }}>Reintentar</Btn>
              </div>
            </div>
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
            paged.map((row, i) => (
              <StuRow
                key={row.id}
                st={row.student}
                room={row.room}
                levelName={row.levelName}
                i={i}
                selected={filter.selected.has(row.id)}
                onToggle={() => filter.toggle(row.id)}
                onOpen={() => setSheet(row.student)}
                onEdit={onEdit === undefined ? undefined : () => onEdit(row.id)}
              />
            ))
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

      {/* Room for the floating button, so the last row and the pager scroll
          clear of it instead of hiding under it (QA 2026-09-25, ISSUE-005). */}
      <div aria-hidden data-testid="fab-clearance" style={{ height: FAB_BOTTOM + FAB_HEIGHT + FAB_GAP }} />

      <button onClick={() => setAdd(true)} title="Nuevo alumno" style={{ position: "fixed", right: 32, bottom: FAB_BOTTOM, zIndex: 50, height: FAB_HEIGHT, padding: "0 26px", borderRadius: 99, display: "inline-flex", alignItems: "center", gap: 10, background: "var(--primary)", color: "var(--primary-ink)", fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 16.5, cursor: "pointer", boxShadow: "0 10px 30px color-mix(in oklab, var(--lime) 45%, transparent)" }}><Icon name="plus" size={22} stroke={2.6} />Nuevo alumno</button>

      {add && <QuickAdd onClose={() => setAdd(false)} onSave={onSave} levels={levels} withNote={canWriteNotes} />}
      {sheet !== null && <StudentSheet st={sheet} groups={groups} cycleId={groupsCycleId} gradeLevels={gradeLevels} gradeLevelsFailed={gradeLevelsFailed} onClose={() => setSheet(null)} onPlaced={onPlaced} onEdit={onEdit === undefined ? undefined : () => onEdit(sheet.id)} canRevokeCard={canRevokeCard} />}
      {verifying && <VerifyCardDrawer onClose={() => setVerifying(false)} />}
      {toast !== null && (
        <div style={{ position: "fixed", bottom: 28, left: "50%", transform: "translateX(-50%)", zIndex: 80, display: "flex", alignItems: "center", gap: 11, padding: "14px 22px", borderRadius: "var(--r-pill)", background: "var(--forest)", color: "#fff", fontWeight: 600, fontSize: 15, boxShadow: "var(--shadow-pop)", animation: "pop .25s var(--ease) both" }}>
          <Icon name="check" size={20} stroke={2.6} />{toast}
        </div>
      )}
    </Page>
  );
}

/** How long the armed "Confirmar: revocar" waits before it disarms itself. */
const CONFIRM_WINDOW_MS = 5000;

/**
 * The pupil's card, from the office (task 13). Revoking suspends it (the
 * owner's decision: a stolen phone keeps its session and would otherwise get a
 * valid new code) and asks once more on the same button, disarming after a few
 * seconds. "Emitir nueva credencial" gives it back, on purpose, once the
 * office trusts the phone again.
 */
function RevokeCard({ studentId }: { studentId: string }): ReactElement {
  const [confirming, setConfirming] = useState(false);
  const [message, setMessage] = useState<{ tone: "status" | "alert"; text: string } | undefined>(undefined);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    if (!confirming) return;
    const disarm = setTimeout(() => setConfirming(false), CONFIRM_WINDOW_MS);
    return () => clearTimeout(disarm);
  }, [confirming]);

  const run = async (action: () => Promise<void>, done: string, refused: (err: unknown) => string): Promise<void> => {
    if (pending) return;
    setPending(true);
    try {
      await action();
      setMessage({ tone: "status", text: done });
    } catch (err) {
      setMessage({ tone: "alert", text: refused(err) });
    } finally {
      setPending(false);
      setConfirming(false);
    }
  };

  const revoke = (): void => {
    if (!confirming) {
      setConfirming(true);
      return;
    }
    void run(() => revokeCard(API_URL, studentId),
      "La credencial queda suspendida hasta que emitas una nueva.",
      () => "No se pudo revocar la credencial.");
  };

  const reissue = (): void => {
    void run(() => reissueCard(API_URL, studentId),
      "Listo: la familia verá la nueva credencial al abrirla.",
      (err) => (err instanceof StudentsError && err.status === 409
        ? "La credencial sigue vigente: revócala primero si hay que cambiarla."
        : "No se pudo emitir la credencial."));
  };

  return (
    <div style={{ display: "grid", gap: 8, padding: "18px 0 0" }}>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <Btn kind="ghost" icon={confirming ? "warning" : "qr"} danger={confirming} onClick={revoke}>
          {confirming ? "Confirmar: revocar credencial" : "Revocar credencial"}
        </Btn>
        <Btn kind="ghost" onClick={reissue}>Emitir nueva credencial</Btn>
      </div>
      {message !== undefined && (
        <p role={message.tone} style={{ fontSize: 13.5, color: message.tone === "alert" ? "var(--bad-fg)" : "var(--text-muted)" }}>
          {message.text}
        </p>
      )}
    </div>
  );
}

/** Checkbox, pupil, level, tutor, status, actions. */
const ROW_COLUMNS = "auto 2.4fr 1.1fr 1.6fr 1fr auto";
/** The checkbox cell: a 44 px target. */
const CHECK_CELL = 44;

function StuRow({ st, room, levelName, i, selected, onToggle, onOpen, onEdit }: {
  st: Student;
  room: string;
  levelName: string;
  i: number;
  selected: boolean;
  onToggle: () => void;
  onOpen: () => void;
  onEdit?: () => void;
}): ReactElement {
  const [hov, setHov] = useState(false);
  const s = statusOf(st.admissionStatus);
  return (
    <div onMouseEnter={() => setHov(true)} onMouseLeave={() => setHov(false)} onClick={onOpen} className="students-grid" style={{ display: "grid", gridTemplateColumns: ROW_COLUMNS, alignItems: "center", gap: 16, padding: "13px 24px", borderRadius: "var(--r-md)", position: "relative", cursor: "pointer", background: selected ? "color-mix(in oklab, var(--lime) 10%, var(--surface))" : hov ? "var(--surface-2)" : "transparent", transition: "background .15s var(--ease)" }}>
      <span style={{ position: "absolute", left: 4, top: 10, bottom: 10, width: 4, borderRadius: 99, background: s.dot }} />
      <button
        type="button"
        role="checkbox"
        aria-checked={selected}
        aria-label={`Seleccionar a ${formatStudentName(st)}`}
        onClick={(ev) => { ev.stopPropagation(); onToggle(); }}
        style={{ width: CHECK_CELL, height: CHECK_CELL, display: "grid", placeItems: "center", background: "transparent", border: "none", cursor: "pointer" }}
      >
        <Checkmark on={selected} />
      </button>
      <div style={{ display: "flex", alignItems: "center", gap: 13, minWidth: 0, paddingLeft: 8 }}>
        <Avatar name={formatStudentName(st)} color={AV[i % AV.length]} size={42} />
        <div style={{ minWidth: 0 }}>
          <div className="students-name" style={{ fontWeight: 600, fontSize: 15.5, color: "var(--text)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{formatStudentName(st)}</div>
          <div style={{ fontSize: 12.5, color: "var(--text-faint)", fontFamily: "monospace" }}>{st.curp}</div>
          {/* On a phone the level, tutor and status columns fold away; the classroom stays under the name. */}
          <div className="students-room-inline" style={{ fontSize: 12.5, color: "var(--text-muted)" }}>{room} · {s.label}</div>
        </div>
      </div>
      <div className="students-col-wide" style={{ fontSize: 14, color: "var(--text-muted)", textTransform: "capitalize" }}>{levelName}<div style={{ fontSize: 12.5, color: "var(--text-faint)", textTransform: "none" }}>{room}</div></div>
      <div className="students-col-wide" style={{ minWidth: 0, fontSize: 14, color: "var(--text)", whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis" }}>{st.guardianName}</div>
      <div className="students-col-wide"><StatusPill status={st.admissionStatus} /></div>
      <div style={{ display: "flex", justifyContent: "flex-end", gap: 6, width: 84 }}>
        {onEdit !== undefined && (
          <button onClick={(ev) => { ev.stopPropagation(); onEdit(); }} title="Editar ficha" aria-label={`Editar ficha de ${formatStudentName(st)}`} style={qb}><Icon name="user" size={18} /></button>
        )}
        <button onClick={(ev) => { ev.stopPropagation(); onOpen(); }} title="Ver ficha" aria-label={`Ver ficha de ${formatStudentName(st)}`} style={qb}><Icon name="chevron" size={18} /></button>
      </div>
    </div>
  );
}
