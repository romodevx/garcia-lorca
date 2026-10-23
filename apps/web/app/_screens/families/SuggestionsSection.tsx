"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { FamilySuggestion } from "../../families/api";
import type { FamiliesAccess } from "../../families/model";
import type { FamilyPupil } from "./familyPupil";
import { SuggestionCard } from "./SuggestionCard";
import type { FamilySuggestions } from "./useFamilySuggestions";

const READ_ONLY = "Sólo la dirección puede armar familias";

/**
 * Suggestions come first: confirming them is most of the work, and the
 * "muy probable" ones (a contact in common) go in one click for all.
 */
export function SuggestionsSection({ state, pupils, access, onAdjust, onBuilt, onDismissed }: {
  state: FamilySuggestions;
  pupils: ReadonlyMap<string, FamilyPupil>;
  access: FamiliesAccess;
  onAdjust: (s: FamilySuggestion) => void;
  /** `count` families were built (one, or the batch). */
  onBuilt: (message: string) => void;
  onDismissed: () => void;
}): ReactElement | null {
  if (state.suggestions.length === 0) return null;
  const high = state.suggestions.filter((s) => s.confidence === "high").length;
  const canBuild = access.canWriteGuardians;

  const acceptAll = async (): Promise<void> => {
    const built = await state.acceptAllHigh();
    if (built > 0) onBuilt(built === 1 ? "1 familia creada" : `${built} familias creadas`);
  };

  return (
    <section aria-labelledby="families-suggestions-title" style={{ display: "grid", gap: 12 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        <h2 id="families-suggestions-title" style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", flex: 1, minWidth: 0 }}>Sugerencias por revisar</h2>
        {canBuild && high > 0 && <Btn icon="check" onClick={() => void acceptAll()}>{`Aceptar las muy probables (${high})`}</Btn>}
      </div>
      {!canBuild && <p style={{ fontSize: 14, color: "var(--text-muted)", margin: 0 }}>{READ_ONLY}</p>}
      <div style={{ display: "grid", gap: 12, gridTemplateColumns: "repeat(auto-fill, minmax(min(100%, 300px), 1fr))" }}>
        {state.suggestions.map((s) => (
          <SuggestionCard
            key={s.signature}
            suggestion={s}
            pupils={pupils}
            busy={state.busy.has(s.signature)}
            error={state.errors[s.signature] || undefined}
            actions={{
              ...(canBuild
                ? {
                    onAccept: () => void state.accept(s).then((ok) => { if (ok) onBuilt(`Familia ${s.proposedName} creada`); }),
                    onAdjust: () => onAdjust(s),
                  }
                : {}),
              ...(access.canUpdateStudents ? { onDismiss: () => void state.dismiss(s).then((ok) => { if (ok) onDismissed(); }) } : {}),
            }}
          />
        ))}
      </div>
    </section>
  );
}
