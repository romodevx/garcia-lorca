"use client";

import { useMemo, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { StatCards, type StatCardDef } from "../../_design/StatCards";
import type { Family, FamilySuggestion } from "../../families/api";
import { membershipOf, withApp, withoutFamily, type FamiliesAccess } from "../../families/model";
import { FamiliesTable } from "./FamiliesTable";
import { FamilyCreateDrawer, type FamilyDraft } from "./FamilyCreateDrawer";
import { pupilsById, type FamilyPupil } from "./familyPupil";
import { SuggestionsSection } from "./SuggestionsSection";
import { useFamilySuggestions } from "./useFamilySuggestions";

export interface FamiliesViewProps {
  access: FamiliesAccess;
  families: readonly Family[];
  familiesFailed: boolean;
  /** Every pupil of the list, named and placed. */
  pupils: readonly FamilyPupil[];
  onOpenFamily: (familyId: string) => void;
  /** A family was built: the list re-reads them. */
  onFamiliesChanged: () => void;
  /** "Alumnos sin familia": the directory, filtered to them. */
  onShowWithoutFamily: () => void;
  say: (message: string) => void;
}

/**
 * Familias, the second view of Alumnos (ADR 0041 D1): the numbers, the
 * suggestions to confirm, and every family. Building one is guardians:write;
 * without it the suggestions are read-only.
 */
export function FamiliesView({ access, families, familiesFailed, pupils, onOpenFamily, onFamiliesChanged, onShowWithoutFamily, say }: FamiliesViewProps): ReactElement {
  const suggestions = useFamilySuggestions();
  // undefined = closed; a draft = "Nueva familia", empty or from a suggestion.
  const [creating, setCreating] = useState<{ draft?: FamilyDraft; signature?: string } | undefined>(undefined);
  const byId = useMemo(() => pupilsById(pupils), [pupils]);
  const membership = useMemo(() => membershipOf(families), [families]);
  const orphans = useMemo(() => withoutFamily(pupils, membership), [pupils, membership]);

  const built = (message: string): void => {
    say(message);
    onFamiliesChanged();
  };
  const adjust = (s: FamilySuggestion): void => {
    const draftPupils = s.studentIds.map((id) => byId.get(id)).filter((p): p is FamilyPupil => p !== undefined);
    setCreating({ draft: { name: s.proposedName, pupils: draftPupils }, signature: s.signature });
  };

  const cards: StatCardDef[] = [
    { icon: "students", color: "forest", label: "Familias", value: String(families.length), sub: "Hermanos agrupados", testId: "families-stat-total" },
    { icon: "help", color: "yellow", label: "Sugerencias por revisar", value: String(suggestions.suggestions.length), sub: "Confirma o descarta", testId: "families-stat-suggestions" },
    ...(access.canReadGuardians
      ? [{ icon: "check" as const, color: "lime" as const, label: "Con app", value: String(withApp(families)), sub: "Familias con cuenta", testId: "families-stat-app" }]
      : []),
    { icon: "user", color: "purple", label: "Alumnos sin familia", value: String(orphans.length), sub: "Ver en el directorio", onClick: onShowWithoutFamily, testId: "families-stat-orphans" },
  ];

  return (
    <div style={{ display: "grid", gap: 26 }}>
      <StatCards cards={cards} />
      <SuggestionsSection state={suggestions} pupils={byId} access={access} onAdjust={adjust} onBuilt={built} onDismissed={() => say("Listo: no se volverá a sugerir")} />
      {suggestions.failed && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>No pudimos leer las sugerencias.</p>}
      <div style={{ display: "grid", gap: 14 }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <h2 style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", flex: 1, minWidth: 0 }}>Todas las familias</h2>
          {access.canWriteGuardians && <Btn icon="plus" onClick={() => setCreating({})}>Nueva familia</Btn>}
        </div>
        {familiesFailed ? (
          <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>No pudimos leer las familias.</p>
        ) : (
          <FamiliesTable families={families} pupils={byId} showAccounts={access.canReadGuardians} onOpen={onOpenFamily} />
        )}
      </div>
      {creating !== undefined && (
        <FamilyCreateDrawer
          candidates={orphans}
          draft={creating.draft}
          onClose={() => setCreating(undefined)}
          onCreated={(family) => {
            if (creating.signature !== undefined) suggestions.drop(creating.signature);
            suggestions.reload();
            setCreating(undefined);
            built(`Familia ${family.name} creada`);
          }}
        />
      )}
    </div>
  );
}
