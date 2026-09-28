"use client";

import { useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { Icon } from "../../_design/Icon";
import { SlideOver } from "../../_design/SlideOver";
import type { AttendanceMode } from "../../attendance/sheet";
import type { CreateLevelInput, EnrollmentGranularity, Level, LevelGrading } from "../../config/api";
import { TextField } from "../../_design/form";
import { AttendanceModeControl } from "./AttendanceModeControl";
import { GradingControl } from "./GradingControl";
import { EmptyHint, RadioGroup, Section, SelectChip } from "./parts";

export interface LevelsPanelProps {
  levels: readonly Level[];
  selectedLevelId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateLevelInput) => Promise<void>;
  /** Saves how the level takes attendance (per day or per class). */
  onSetAttendanceMode: (levelId: string, mode: AttendanceMode) => Promise<void>;
  /** Saves whether and how the level grades (scale, decimals, passing mark). */
  onSetGrading: (levelId: string, grading: LevelGrading) => Promise<void>;
  /** Opens the deletion of a level. Absent = the principal may not (levels:delete). */
  onDelete?: (levelId: string) => void;
}

const GRANULARITY_OPTIONS = [
  { value: "cycle", label: "Por ciclo" },
  { value: "term", label: "Por periodo" },
] as const;

const GRANULARITY_LABEL: Record<string, string> = { cycle: "por ciclo", term: "por periodo" };

/**
 * The school's levels: maternal, kínder, primaria…
 *
 * Choosing one and creating one are different acts, and the screen used to do
 * both at once — a list of chips with a form permanently open underneath it.
 * Now the panel is the list, and creating opens the same drawer a new pupil
 * does.
 */
export function LevelsPanel({ levels, selectedLevelId, onSelect, onCreate, onSetAttendanceMode, onSetGrading, onDelete }: LevelsPanelProps): ReactElement {
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [sepCode, setSepCode] = useState("");
  const [granularity, setGranularity] = useState("cycle");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const selectedLevel = levels.find((l) => l.id === selectedLevelId);

  /**
   * `andNew` deja el cajón abierto y listo para el siguiente.
   *
   * Una escuela captura maternal, kínder y primaria de corrido, no uno cada
   * quince días, y cerrar entre uno y otro cobra el viaje completo. El nivel se
   * queda en cajón —y no baja a renglón como grados o materias— porque elegir
   * "por periodo" cambia cómo se inscribe el alumno todo el año, y esa decisión
   * necesita el párrafo que la explica.
   */
  const submit = async (andNew: boolean): Promise<void> => {
    // Sólo el nombre es obligatorio. La clave SEP es opcional, como dice su
    // ayuda y como siempre lo permitió el servidor: exigirla dejaba fuera a
    // preescolar y a cualquier nivel que la escuela nombre a su manera.
    if (name.trim() === "") {
      setError("Un nivel necesita nombre.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      await onCreate({
        name: name.trim(),
        sepCode: sepCode.trim(),
        enrollmentGranularity: granularity as EnrollmentGranularity,
      });
      setName("");
      setSepCode("");
      setGranularity("cycle");
      if (andNew) {
        // El cajón sigue montado, así que su primer campo sigue ahí; el foco
        // vuelve a él para que el siguiente nivel se escriba sin tocar el ratón.
        document.querySelector<HTMLInputElement>('[role="dialog"] input')?.focus();
      } else {
        setCreating(false);
      }
    } catch {
      setError("No se pudo crear el nivel. Revisa que no exista ya con ese nombre.");
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Niveles educativos" subtitle="Elige uno para ver sus grados, o agrega otro.">
      {levels.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {levels.map((l) => (
            <SelectChip key={l.id} selected={l.id === selectedLevelId} onClick={() => onSelect(l.id)}>
              {l.name} · {GRANULARITY_LABEL[l.enrollmentGranularity] ?? l.enrollmentGranularity}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Aún no hay niveles. Agrega el primero.</EmptyHint>
      )}

      {selectedLevel !== undefined && (
        <>
          <AttendanceModeControl key={selectedLevel.id} level={selectedLevel} onChange={onSetAttendanceMode} />
          <GradingControl key={`grading-${selectedLevel.id}`} level={selectedLevel} onChange={onSetGrading} />
          {onDelete !== undefined && <DeleteLevelButton level={selectedLevel} onDelete={onDelete} />}
        </>
      )}

      <Btn icon="plus" onClick={() => { setError(undefined); setCreating(true); }}>Nuevo nivel</Btn>

      {creating && (
        <SlideOver
          title="Nuevo nivel educativo"
          subtitle="Maternal, kínder, primaria…"
          icon="students"
          submitLabel="Crear nivel"
          pending={pending}
          error={error}
          onClose={() => { setCreating(false); setError(undefined); }}
          onSubmit={() => void submit(false)}
          onSubmitAndNew={() => void submit(true)}
        >
          <TextField label="Nombre del nivel" value={name} onChange={setName} uppercase required help="El nivel educativo: preescolar, primaria, secundaria." />
          <TextField label="Clave SEP" value={sepCode} onChange={setSepCode} uppercase help="La clave corta con la que la SEP identifica este nivel. Puede quedar vacía y ponerse después." />
          <RadioGroup legend="Inscripción" options={GRANULARITY_OPTIONS} value={granularity} onChange={setGranularity} />
          <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>
              <strong>Por ciclo</strong> es lo normal: el alumno se inscribe una vez al año.
              <strong> Por periodo</strong> es para niveles que se cursan por cuatrimestre o semestre.
            </p>
          </div>
        </SlideOver>
      )}
    </Section>
  );
}

/**
 * A secondary action in the danger colour — never the screen's primary. It only
 * opens the three-step page; nothing is deleted from here.
 */
function DeleteLevelButton({ level, onDelete }: { level: Level; onDelete: (levelId: string) => void }): ReactElement {
  return (
    <div style={{ margin: "4px 0 16px" }}>
      <button
        type="button"
        onClick={() => onDelete(level.id)}
        aria-label={`Eliminar nivel ${level.name}`}
        style={{
          display: "inline-flex", alignItems: "center", gap: 8, minHeight: 44, padding: "0 16px", borderRadius: "var(--r-pill)",
          border: "1.5px solid var(--red)", background: "transparent", color: "var(--bad-fg)", fontWeight: 600, fontSize: 14.5,
        }}
      >
        <Icon name="warning" size={18} />
        Eliminar nivel
      </button>
    </div>
  );
}
