"use client";

import { useState, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { SlideOver } from "../../_design/SlideOver";
import { ConfigError, type Cycle, type CreateCycleInput } from "../../config/api";
import { DateField, TextField, useRequired } from "../../_design/form";
import { schoolISODate } from "../../_lib/schoolDay";
import { proposeCycle } from "./cycleProposal";
import { EmptyHint, ErrorAlert, Section, SelectChip } from "./parts";
import { useSetupPresentation } from "./SetupPresentation";
import { ActivateCycleDialog } from "./ActivateCycleDialog";
import { CycleRenameForm } from "./CycleRenameForm";

export interface CycleBarProps {
  cycles: readonly Cycle[];
  selectedCycleId: string | undefined;
  onSelect: (id: string) => void;
  onCreate: (input: CreateCycleInput) => Promise<void>;
  /** The school's today (aaaa-mm-dd), for the proposed new cycle; defaults to now. */
  today?: string;
  /** Puts the selected draft year into service, standing down the previous one. */
  onActivate: (id: string) => Promise<void>;
  /** Renames a year (config:write); without it the pencil is not offered. */
  onRename?: (id: string, name: string) => Promise<void>;
}

/** The refusal of a year whose dates share a day with another (QA CC-101). */
const CYCLE_OVERLAP = "CYCLE_OVERLAP";

/**
 * What went wrong, in words that point at a fix.
 *
 * The whole message used to be "Intenta de nuevo", which for the commonest
 * failure — a cycle name the school already used — is advice that can never
 * work. A person following it clicks the same button forever.
 */
export function cycleFailureMessage(err: unknown): string {
  if (err instanceof ConfigError) {
    // The server names the year it collides with and its dates; nothing here can say it better.
    if (err.code === CYCLE_OVERLAP && err.detail !== "") return err.detail;
    if (err.isConflict) return "Ya existe un ciclo con ese nombre. Usa otro.";
    if (err.isInvalid) return "Revisa las fechas: el fin del ciclo debe ser posterior al inicio.";
    if (err.status === 401) return "Tu sesión expiró. Vuelve a entrar.";
    if (err.status === 403) return "Tu cuenta no tiene permiso para crear ciclos.";
    if (err.status === 404) return "El módulo de configuración está apagado.";
  }
  return "No pudimos crear el ciclo. Revisa tu conexión e intenta de nuevo.";
}

const STATUS_LABEL: Record<string, string> = {
  draft: "Borrador",
  active: "Activo",
  closed: "Cerrado",
};

/**
 * Pick the school year to configure, or start a new one.
 *
 * The form used to sit open underneath the list, so the screen showed a choice
 * and a creation form at once and the school called it confusing. Creating is
 * now a drawer — the same one a new pupil or a payment opens — and this panel
 * does one thing: which year are we working on.
 */
/** Fuera del componente: useRequired los toma como dependencia. */
const CYCLE_REQUIRED = ["name", "startsOn", "endsOn"] as const;

export function CycleBar({ cycles, selectedCycleId, onSelect, onCreate, onActivate, onRename, today }: CycleBarProps): ReactElement {
  const [creating, setCreating] = useState(false);
  const form = useRequired(CYCLE_REQUIRED);
  const [name, setName] = useState("");
  const [startsOn, setStartsOn] = useState("");
  const [endsOn, setEndsOn] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [activating, setActivating] = useState(false);
  const [confirmingActivation, setConfirmingActivation] = useState(false);
  const [renaming, setRenaming] = useState(false);

  const selected = cycles.find((c) => c.id === selectedCycleId);
  // The year activating `selected` would pause, if any (QA CC-102).
  const live = cycles.find((c) => c.status === "active" && c.id !== selectedCycleId);
  // Inside a setup step "Nuevo ciclo" is the step's lime only while there is no
  // cycle yet; after that "Siguiente" is, and one step never shows two (§1.3).
  const { inSetup } = useSetupPresentation();
  const newCycleKind = inSetup && cycles.length > 0 ? "outline" : "primary";

  const activate = async (): Promise<void> => {
    if (selected === undefined) {
      return;
    }
    setActivating(true);
    setError(undefined);
    try {
      await onActivate(selected.id);
    } catch (err) {
      setError(cycleFailureMessage(err));
    } finally {
      setActivating(false);
    }
  };

  // From the confirmation dialog: a refusal is thrown back so the dialog shows it.
  const confirmActivation = async (id: string): Promise<void> => {
    setActivating(true);
    setError(undefined);
    try {
      await onActivate(id);
    } finally {
      setActivating(false);
    }
  };

  // Pausing the live year of the whole school is not a one-click thing; with no
  // live year there is nothing to pause, and asking would only be a click more.
  const requestActivate = (): void => {
    setError(undefined);
    if (live === undefined) {
      void activate();
      return;
    }
    setConfirmingActivation(true);
  };

  // Opens on the proposed school year, so the happy path is one click (ISSUE-T14-023).
  const startCreating = (): void => {
    const proposal = proposeCycle(cycles, today ?? schoolISODate());
    setName(proposal.name);
    setStartsOn(proposal.startsOn);
    setEndsOn(proposal.endsOn);
    setError(undefined);
    setCreating(true);
  };

  const submit = async (): Promise<void> => {
    // Cada campo que falta se marca en sí mismo. La frase que había aquí decía
    // "escribe el nombre del ciclo y sus dos fechas" sin decir CUÁL faltaba.
    if (!form.check({ name, startsOn, endsOn })) return;
    setPending(true);
    setError(undefined);
    try {
      await onCreate({ name: name.trim(), startsOn, endsOn });
      setName("");
      setStartsOn("");
      setEndsOn("");
      setCreating(false);
    } catch (err) {
      setError(cycleFailureMessage(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <Section title="Ciclo escolar" subtitle="El año que estás configurando.">
      {cycles.length > 0 ? (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: 16 }}>
          {cycles.map((c) => (
            <SelectChip key={c.id} selected={c.id === selectedCycleId} onClick={() => onSelect(c.id)}>
              {c.name} · {STATUS_LABEL[c.status] ?? c.status}
            </SelectChip>
          ))}
        </div>
      ) : (
        <EmptyHint>Todavía no hay ciclos. Crea el primero para empezar.</EmptyHint>
      )}

      {renaming && selected !== undefined && onRename !== undefined && (
        <CycleRenameForm key={selected.id} name={selected.name} onRename={(name) => onRename(selected.id, name)} onDone={() => setRenaming(false)} />
      )}

      <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
        <Btn icon="plus" kind={newCycleKind} onClick={startCreating}>Nuevo ciclo</Btn>
        {selected !== undefined && selected.status !== "active" && (
          <Btn kind="ghost" icon="check" onClick={requestActivate} disabled={activating}>
            {activating ? "Activando…" : `Activar ciclo ${selected.name}`}
          </Btn>
        )}
        {selected !== undefined && selected.status === "active" && (
          <p style={{ alignSelf: "center", fontSize: 13.5, color: "var(--text-muted)", margin: 0 }}>
            {selected.name} es el ciclo en curso.
          </p>
        )}
        {selected !== undefined && onRename !== undefined && !renaming && (
          <Btn kind="ghost" icon="edit" aria-label={`Renombrar ciclo ${selected.name}`} onClick={() => setRenaming(true)}>Renombrar</Btn>
        )}
      </div>
      {confirmingActivation && selected !== undefined && live !== undefined && (
        <ActivateCycleDialog
          cycleName={selected.name}
          pausedName={live.name}
          onConfirm={() => confirmActivation(selected.id)}
          failureMessage={cycleFailureMessage}
          onClose={() => setConfirmingActivation(false)}
        />
      )}
      {error !== undefined && !creating && <ErrorAlert message={error} />}

      {creating && (
        <SlideOver
          title="Nuevo ciclo escolar"
          subtitle="El año que la escuela va a cursar"
          icon="calendar"
          submitLabel="Crear ciclo"
          pending={pending}
          error={error}
          onClose={() => { setCreating(false); setError(undefined); }}
          onSubmit={() => void submit()}
        >
          <TextField
            label="Nombre del ciclo"
            value={name}
            onChange={setName}
            uppercase
            required
            help="Cómo se llama el ciclo escolar. Es lo que verás en cada filtro del sistema."
            error={form.errors.name}
            onClearError={() => form.clear("name")}
          />
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 14 }}>
            <DateField
              label="Inicia"
              value={startsOn}
              onChange={setStartsOn}
              required
              error={form.errors.startsOn}
              onClearError={() => form.clear("startsOn")}
            />
            <DateField
              label="Termina"
              value={endsOn}
              onChange={setEndsOn}
              required
              error={form.errors.endsOn}
              onClearError={() => form.clear("endsOn")}
            />
          </div>
          <div style={{ display: "flex", gap: 11, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
            <span style={{ color: "var(--forest)", flexShrink: 0, marginTop: 1 }}>i</span>
            <p style={{ fontSize: 13.5, color: "var(--text-muted)", lineHeight: 1.5, margin: 0 }}>
              Nace como <strong>borrador</strong>. Nada cambia para la escuela hasta que lo actives,
              y activarlo deja en pausa el ciclo anterior.
            </p>
          </div>
        </SlideOver>
      )}
    </Section>
  );
}
