"use client";

import { useMemo, type FormEvent, type ReactElement, type ReactNode } from "react";

import { Btn } from "../../_design/Controls";
import { CreatePage } from "../../_design/CreatePage";
import { ErrorSummary, TextArea } from "../../_design/form";
import { ErrorAlert } from "../config/parts";
import { AltaIdentityFields } from "./AltaIdentityFields";
import { AltaPlacementFields, groupsOfGrade } from "./AltaPlacementFields";
import { useAltaCatalog, useGradesOf } from "./useAltaCatalog";
import { FIELD_LABEL, useAltaForm, type AltaForm } from "./useAltaForm";

export interface StudentAltaPageProps {
  /** Cancel: back to the list as it was left. */
  onBack: () => void;
  /** Saved: the confirmation the list shows when it comes back. */
  onCreated: (message: string) => void;
  /** students:notes_write: offer an optional first note (task 6). */
  canWriteNotes?: boolean;
}

const LOCKED_NOTICE = "El alumno ya se creó; corrige sólo grupo, grado o fecha de ingreso.";

/**
 * The alta of one pupil, at /alumnos/nuevo (T9).
 *
 * Eleven fields fully open — the drawer it lived in holds six — so it is a
 * PAGE (ui-forms-and-modals): one column, required fields said in text, every
 * problem listed at the top on save, the primary never disabled, and "Crear y
 * agregar otro" for the start of the school year, when the office types forty.
 */
export function StudentAltaPage({ onBack, onCreated, canWriteNotes = false }: StudentAltaPageProps): ReactElement {
  const catalog = useAltaCatalog();
  const form = useAltaForm({ levels: catalog.levels, groups: catalog.groups, withNote: canWriteNotes, onCreated });
  const grades = useGradesOf(form.draft.levelId);
  const gradeGroups = useMemo(() => groupsOfGrade(catalog.groups, form.draft.gradeLevelId), [catalog.groups, form.draft.gradeLevelId]);

  if (catalog.status === "loading") {
    return <p role="status" style={{ padding: 26, color: "var(--text-muted)" }}>Cargando el alta…</p>;
  }

  const submitOnEnter = (e: FormEvent<HTMLFormElement>): void => {
    e.preventDefault();
    void form.submit("close");
  };

  return (
    // Enter in a single-line field creates the pupil; a textarea keeps its newline.
    <form onSubmit={submitOnEnter} noValidate style={{ display: "contents" }}>
      <CreatePage
        breadcrumb={["Estudiantes"]}
        title="Nuevo alumno"
        subtitle="La admisión inicia en trámite"
        footer={<Footer form={form} onBack={onBack} />}
      >
        <p style={{ fontSize: 13.5, color: "var(--text-muted)", margin: 0 }}>Los campos con * son obligatorios.</p>
        {form.done !== undefined && <p role="status" style={{ margin: 0, padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--ok-bg)", color: "var(--ok-fg)", fontWeight: 600 }}>{form.done}</p>}
        {form.created && <p role="status" style={{ margin: 0, padding: "12px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)", fontWeight: 600 }}>{LOCKED_NOTICE}</p>}
        <ErrorSummary attempt={form.attempt} items={Object.entries(form.errors).map(([key, message]) => ({ field: FIELD_LABEL[key] ?? key, message }))} />
        {catalog.status === "failed" && <CatalogFailed onRetry={catalog.retry} />}
        {catalog.status === "ready" && catalog.levels.length === 0 && (
          <ErrorAlert message="El colegio aún no tiene niveles: créalos en Configuración para poder inscribir alumnos." />
        )}

        <Section title="Alumno">
          <AltaIdentityFields form={form} />
        </Section>
        <Section title="Inscripción" note="grupo automático e ingreso hoy, si no cambias nada">
          <AltaPlacementFields form={form} levels={catalog.levels} grades={grades} groups={gradeGroups} cycleStartsOn={catalog.cycleStartsOn} />
        </Section>
        {canWriteNotes && (
          <Section title="Nota">
            <TextArea
              label="Nota"
              value={form.draft.note}
              onChange={(v) => form.set("note", v)}
              rows={2}
              readOnly={form.created}
              help="Opcional. Algo que el colegio deba recordar de este alumno; queda en su ficha, con tu nombre."
            />
          </Section>
        )}
        {form.failure !== undefined && <ErrorAlert message={form.failure} />}
      </CreatePage>
    </form>
  );
}

function Footer({ form, onBack }: { form: AltaForm; onBack: () => void }): ReactElement {
  return (
    <>
      {/* Cancel as text; the primary is never disabled, only busy. */}
      <Btn kind="ghost" onClick={onBack}>Cancelar</Btn>
      <div style={{ flex: 1 }} />
      <Btn kind="outline" onClick={() => void form.submit("another")}>Crear y agregar otro</Btn>
      <Btn icon="check" type="submit" style={{ height: 52, fontSize: 16 }}>{form.saving ? "Creando…" : "Crear alumno"}</Btn>
    </>
  );
}

function CatalogFailed({ onRetry }: { onRetry: () => void }): ReactElement {
  return (
    <div style={{ display: "grid", gap: 8 }}>
      <ErrorAlert message="No pudimos leer los niveles del colegio. Sin ellos no se puede inscribir al alumno." />
      <div><Btn kind="outline" onClick={onRetry}>Reintentar</Btn></div>
    </div>
  );
}

function Section({ title, note, children }: { title: string; note?: string; children: ReactNode }): ReactElement {
  return (
    <section style={{ display: "grid", gap: 14, padding: "18px 20px", borderRadius: "var(--r-lg)", background: "var(--surface)", border: "1px solid var(--border)" }}>
      <h2 style={{ fontSize: 17, fontWeight: 800, color: "var(--text)" }}>
        {title}
        {note !== undefined && <span style={{ marginLeft: 8, fontSize: 13.5, fontWeight: 500, color: "var(--text-muted)" }}>{note}</span>}
      </h2>
      {children}
    </section>
  );
}
