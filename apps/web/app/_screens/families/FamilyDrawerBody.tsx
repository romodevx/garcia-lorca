"use client";

import { useMemo, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { FamilyPerson } from "../../families/api";
import { accountsLabel } from "../../families/model";
import { FamilyAdults } from "./FamilyAdults";
import type { FamilyDrawerProps } from "./FamilyDrawer";
import { FamilyMembers } from "./FamilyMembers";
import { FamilyNameEditor } from "./FamilyNameEditor";
import { pupilsById } from "./familyPupil";
import type { FamilyDetailState } from "./useFamilyDetail";
import type { MemberChanges } from "./useMemberChanges";

const MUTED = { fontSize: 14.5, color: "var(--text-muted)", margin: 0 } as const;

/** What the family drawer shows, once the family was read. */
export function FamilyDrawerBody({ drawer, state, name, members, failure, onRenamed, onAskRevoke, onAskDissolve, onAdultDirty }: {
  drawer: FamilyDrawerProps;
  state: FamilyDetailState;
  name: string;
  members: MemberChanges;
  failure: string | undefined;
  onRenamed: (name: string) => void;
  onAskRevoke: (person: FamilyPerson) => void;
  onAskDissolve: () => void;
  /** The adult form holds unsaved text (the drawer asks before closing). */
  onAdultDirty: (dirty: boolean) => void;
}): ReactElement {
  const { access, familyId } = drawer;
  const byId = useMemo(() => pupilsById(drawer.pupils), [drawer.pupils]);
  const changed = (): void => {
    state.reload();
    drawer.onChanged();
  };

  if (members.gone) return <p style={MUTED}>La familia ya no existe: quitaste a su último hijo. Usa «Deshacer» si fue un error, aunque cierres esta ventana.</p>;
  if (state.status === "loading") return <p style={MUTED}>Cargando…</p>;
  if (state.status === "failed" || state.detail === undefined) return <p role="alert" style={{ ...MUTED, color: "var(--bad-fg)" }}>No pudimos leer esta familia.</p>;
  const { detail } = state;
  const memberIds = new Set(detail.studentIds);
  const candidates = drawer.candidates.filter((c) => !memberIds.has(c.id));

  return (
    <div style={{ display: "grid", gap: 22 }}>
      <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
        {detail.accountCount !== undefined && <p style={{ ...MUTED, flex: 1 }}>{`${accountsLabel(detail.accountCount)} cuentas de la app`}</p>}
        {access.canUpdateStudents && <FamilyNameEditor familyId={familyId} name={name} onRenamed={onRenamed} />}
      </div>
      <FamilyMembers
        members={detail.members}
        pupils={byId}
        onOpenStudent={drawer.onOpenStudent}
        actions={access.canWriteGuardians ? {
          candidates,
          addError: members.error,
          removing: members.removing,
          onRemove: (m) => void members.remove(m.id, byId.get(m.id)?.name ?? `${m.lastName} ${m.firstName}`),
          onAdd: (p) => void members.add(p.id, p.name),
        } : undefined}
      />
      {access.canReadGuardians && (
        <FamilyAdults
          familyId={familyId}
          people={state.people}
          failed={state.peopleFailed}
          accountCount={detail.accountCount}
          canWrite={access.canWriteGuardians}
          onChanged={changed}
          onAskRevoke={onAskRevoke}
          say={drawer.say}
          onDirtyChange={onAdultDirty}
        />
      )}
      {failure !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{failure}</p>}
      {access.canWriteGuardians && (
        <div style={{ borderTop: "1px solid var(--border)", paddingTop: 16 }}>
          <Btn kind="ghost" danger icon="trash" onClick={onAskDissolve}>Disolver familia</Btn>
        </div>
      )}
    </div>
  );
}
