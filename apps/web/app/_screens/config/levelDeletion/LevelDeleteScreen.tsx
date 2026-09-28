"use client";

import { useEffect, useRef, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../../_design/Controls";
import { Icon } from "../../../_design/Icon";
import { Page } from "../../../_design/Page";
import { NameStep, PasswordStep, Stepper, SummaryStep } from "./steps";
import { useLevelDeletion, type Step } from "./useLevelDeletion";

export interface LevelDeleteScreenProps {
  levelId: string;
  /** Back to Configuración, nothing deleted. */
  onBack: () => void;
  /** The level is in the trash; the message is the toast Configuración shows. */
  onDeleted: (message: string) => void;
}

const STEP_TITLE: Record<Step, string> = { 1: "Resumen", 2: "Confirma el nombre", 3: "Tu contraseña" };

const alertStyle = { padding: "12px 14px", borderRadius: "var(--r-md)", background: "var(--bad-bg)", color: "var(--bad-fg)", fontWeight: 600, fontSize: 14.5, overflowWrap: "anywhere" } as const;

/**
 * Deleting a level: the most destructive act in Configuración, so it earns its
 * own page and three confirmations — read the summary, type the name, re-enter
 * the password. Three steps is the house rule's signal for a page, not a
 * dialog. The gateway (levels:delete, owner-only) is the boundary; this is UX.
 */
export function LevelDeleteScreen({ levelId, onBack, onDeleted }: LevelDeleteScreenProps): ReactElement {
  const d = useLevelDeletion(levelId, onDeleted);
  const heading = useRef<HTMLHeadingElement | null>(null);
  const mounted = useRef(false);

  // A new step moves focus to its heading, so a screen reader hears where it is.
  useEffect(() => {
    if (!mounted.current) {
      mounted.current = true;
      return;
    }
    heading.current?.focus();
  }, [d.step]);

  const ready = d.load.kind === "ready" ? d.load.preview : undefined;
  const levelName = ready?.levelName ?? "";

  const onSubmit = (e: FormEvent): void => {
    e.preventDefault();
    if (ready === undefined) return;
    if (d.step === 1) d.confirmSummary();
    else if (d.step === 2) d.confirmName();
    else void d.submit();
  };

  return (
    <Page variant="reading">
      <button onClick={onBack} style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}>
        <Icon name="back" size={18} stroke={2.4} />
        Volver a Configuración
      </button>
      <h1 style={{ fontSize: 28, fontWeight: 800, color: "var(--text)", marginBottom: 14, overflowWrap: "anywhere" }}>
        {levelName === "" ? "Eliminar nivel" : `Eliminar nivel ${levelName}`}
      </h1>
      <Stepper step={d.step} />

      <form
        onSubmit={onSubmit}
        noValidate
        style={{ display: "grid", gap: 18, padding: "20px 22px", borderRadius: "var(--r-xl)", background: "var(--surface)", border: "1px solid var(--border)", boxShadow: "var(--shadow-card)", minWidth: 0 }}
      >
        <h2 ref={heading} tabIndex={-1} style={{ fontSize: 19, fontWeight: 700, color: "var(--text)" }}>{STEP_TITLE[d.step]}</h2>

        {d.notice !== undefined && <p role="alert" style={alertStyle}>{d.notice}</p>}
        {d.submitError !== undefined && <p role="alert" style={alertStyle}>{d.submitError}</p>}

        {d.load.kind === "loading" && <p style={{ color: "var(--text-muted)" }}>Cargando resumen…</p>}
        {d.load.kind === "failed" && (
          <div style={{ display: "grid", gap: 10, justifyItems: "start" }}>
            <p role="alert" style={alertStyle}>No se pudo cargar el resumen de este nivel.</p>
            <Btn kind="ghost" onClick={d.retry}>Reintentar</Btn>
          </div>
        )}

        {ready !== undefined && d.step === 1 && <SummaryStep preview={ready} />}
        {ready !== undefined && d.step === 2 && (
          <NameStep levelName={levelName} value={d.name} error={d.nameError} onChange={d.setName} onBlur={() => void d.checkName()} />
        )}
        {ready !== undefined && d.step === 3 && <PasswordStep value={d.password} error={d.passwordError} onChange={d.setPassword} />}

        <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10 }}>
          {d.step === 3 ? (
            <Btn type="submit" danger icon="warning" style={{ whiteSpace: "normal", height: "auto", minHeight: 50, textAlign: "center" }}>
              {`Eliminar nivel ${levelName}`}
            </Btn>
          ) : (
            <Btn type="submit">Continuar</Btn>
          )}
          {d.step > 1 && <Btn kind="ghost" onClick={d.back}>Atrás</Btn>}
          <button type="button" onClick={onBack} style={{ minHeight: 44, padding: "0 12px", color: "var(--text-muted)", fontWeight: 600, fontSize: 15 }}>
            Cancelar
          </button>
        </div>
      </form>
    </Page>
  );
}
