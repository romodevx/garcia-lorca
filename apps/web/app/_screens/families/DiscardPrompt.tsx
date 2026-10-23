"use client";

import { useCallback, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";

/**
 * "¿Descartar…?" asked where the person is working, not in a second dialog
 * (one dialog at a time). "Seguir editando" takes the focus: the safe answer
 * is the one Enter gives.
 */
export function DiscardPrompt({ question, onDiscard, onKeep }: { question: string; onDiscard: () => void; onKeep: () => void }): ReactElement {
  // Stable, so it focuses once when the prompt appears, not on every render.
  const focusOnMount = useCallback((node: HTMLButtonElement | null) => node?.focus(), []);
  return (
    <div role="group" aria-label={question} style={{ display: "grid", gap: 8, padding: "12px 14px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)" }}>
      <p style={{ margin: 0, fontSize: 14.5, fontWeight: 600 }}>{question}</p>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        <Btn kind="ghost" danger onClick={onDiscard} style={{ height: 44, fontSize: 14.5 }}>Descartar</Btn>
        <Btn ref={focusOnMount} onClick={onKeep} style={{ height: 44, fontSize: 14.5 }}>Seguir editando</Btn>
      </div>
    </div>
  );
}
