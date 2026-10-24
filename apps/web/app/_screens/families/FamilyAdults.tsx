"use client";

import { useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import type { FamilyMember, FamilyPerson } from "../../families/api";
import { accessReachLine, familyIsFull } from "../../families/model";
import { useContactInvite } from "../students/contactInvite";
import { AddAdultForm } from "./AddAdultForm";
import { AdultRow, personKey } from "./AdultRow";

const SECTION_TITLE = { fontSize: 13.5, fontWeight: 700, color: "var(--text-muted)", textTransform: "uppercase", letterSpacing: ".04em" } as const;

/**
 * The family's adults (guardians:read), each person once however many
 * children carry them. Access goes through the contact invite the pupil's
 * file already uses; the server keeps the cap of two and says so on a 409.
 */
export function FamilyAdults({ familyId, members, studentIds, people, failed, accountCount, canWrite, onChanged, onAskRevoke, say, onDirtyChange }: {
  familyId: string;
  /** The family's children, as the drawer read them: who an account will see (M2). */
  members: readonly FamilyMember[];
  /** Every child the family lists; more than `members` means the read came back short (M2 fails closed). */
  studentIds: readonly string[];
  people: readonly FamilyPerson[];
  failed: boolean;
  accountCount: number | undefined;
  canWrite: boolean;
  /** Something changed on the server: re-read the family and its adults. */
  onChanged: () => void;
  onAskRevoke: (person: FamilyPerson) => void;
  say: (message: string) => void;
  onDirtyChange: (dirty: boolean) => void;
}): ReactElement {
  const invite = useContactInvite(onChanged);
  const [adding, setAdding] = useState(false);
  const stopAdding = (): void => {
    setAdding(false);
    onDirtyChange(false);
  };
  const full = familyIsFull(accountCount);
  const accessReach = accessReachLine({ studentIds, members });

  return (
    <section aria-label="Adultos" style={{ display: "grid", gap: 10 }}>
      <h3 style={SECTION_TITLE}>Adultos</h3>
      {failed && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>No pudimos leer a los adultos de la familia.</p>}
      {!failed && people.length === 0 && <p style={{ fontSize: 14, color: "var(--text-faint)", margin: 0 }}>Todavía no hay adultos registrados.</p>}
      <ul style={{ display: "grid", listStyle: "none", margin: 0, padding: 0 }}>
        {people.map((p) => (
          <AdultRow key={personKey(p)} person={p} invite={invite} full={full} canGrant={canWrite} accessReach={accessReach} onAskRevoke={canWrite ? () => onAskRevoke(p) : undefined} />
        ))}
      </ul>
      {canWrite && !adding && <div><Btn kind="ghost" icon="plus" onClick={() => setAdding(true)}>Agregar adulto</Btn></div>}
      {canWrite && adding && (
        <AddAdultForm
          familyId={familyId}
          onCancel={stopAdding}
          onDirtyChange={onDirtyChange}
          onAdded={(name) => {
            stopAdding();
            say(`${name} quedó como contacto de cada hijo`);
            onChanged();
          }}
        />
      )}
    </section>
  );
}
