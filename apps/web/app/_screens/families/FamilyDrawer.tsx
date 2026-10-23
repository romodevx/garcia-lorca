"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { ConfirmDialog } from "../../_design/ConfirmDialog";
import { DrawerShell } from "../../_design/DrawerShell";
import { API_URL } from "../../api-url";
import { dissolveFamily, familyRefusal, revokeFamilyAccess, type FamilyPerson } from "../../families/api";
import type { FamiliesAccess } from "../../families/model";
import { DiscardPrompt } from "./DiscardPrompt";
import { FamilyDrawerBody } from "./FamilyDrawerBody";
import type { FamilyPupil } from "./familyPupil";
import { useFamilyDetail } from "./useFamilyDetail";
import type { RemovedChild } from "./useFamilyUndo";
import { useMemberChanges } from "./useMemberChanges";

export interface FamilyDrawerProps {
  familyId: string;
  access: FamiliesAccess;
  pupils: readonly FamilyPupil[];
  /** Pupils without a family: what "Agregar hijo" offers. */
  candidates: readonly FamilyPupil[];
  onClose: () => void;
  /** The family changed on the server: the list re-reads the families. */
  onChanged: () => void;
  /** A child was taken out: the screen offers the Undo (it outlives this drawer). */
  onRemoved: (removed: RemovedChild) => void;
  onOpenStudent: (studentId: string) => void;
  say: (message: string) => void;
}

type Confirming = { kind: "revoke"; person: FamilyPerson } | { kind: "dissolve" };

/**
 * One family: its name, its children, its adults and their access. The two
 * irreversible acts (cutting someone's app, dissolving) ask first — in a
 * dialog that REPLACES the drawer while it asks, never one on top of it.
 */
export function FamilyDrawer(props: FamilyDrawerProps): ReactElement {
  const { familyId, access, onClose, say } = props;
  useFocusBackToTrigger();
  const state = useFamilyDetail(familyId, access.canReadGuardians);
  const [confirming, setConfirming] = useState<Confirming | undefined>(undefined);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const [renamed, setRenamed] = useState<string | undefined>(undefined);
  // A half-written adult: closing the drawer asks first (✕, Esc, the veil).
  const [adultDirty, setAdultDirty] = useState(false);
  const [askingDiscard, setAskingDiscard] = useState(false);
  const close = (): void => {
    if (adultDirty) setAskingDiscard(true);
    else onClose();
  };
  const name = renamed ?? state.detail?.name ?? "";
  const changed = (): void => {
    state.reload();
    props.onChanged();
  };
  const members = useMemberChanges(familyId, { familyName: name, onChanged: changed, onRemoved: props.onRemoved, say });

  const revoke = async (person: FamilyPerson): Promise<void> => {
    setConfirming(undefined);
    if (person.guardianUserId === undefined) return;
    try {
      await revokeFamilyAccess(API_URL, familyId, person.guardianUserId);
      say(`${person.fullName} ya no tiene acceso a la app`);
      changed();
    } catch (cause) {
      setFailure(familyRefusal(cause, "No se pudo quitar el acceso."));
    }
  };
  const dissolve = async (): Promise<void> => {
    setConfirming(undefined);
    try {
      await dissolveFamily(API_URL, familyId);
      props.onChanged();
      say(`Familia ${name} disuelta`);
      onClose();
    } catch (cause) {
      setFailure(familyRefusal(cause, "No se pudo disolver la familia."));
    }
  };

  if (confirming?.kind === "revoke") {
    return (
      <ConfirmDialog title="Quitar el acceso a la app" confirmLabel="Quitar acceso" danger onConfirm={() => void revoke(confirming.person)} onCancel={() => setConfirming(undefined)}>
        {confirming.person.fullName} ya no podrá entrar a la app para ninguno de los hijos de esta familia. Sus datos de contacto se quedan.
      </ConfirmDialog>
    );
  }
  if (confirming?.kind === "dissolve") {
    return (
      <ConfirmDialog title="Disolver la familia" confirmLabel="Disolver" danger onConfirm={() => void dissolve()} onCancel={() => setConfirming(undefined)}>
        Se disuelve la familia {name}: los hijos se quedan sin familia; nadie pierde acceso a la app.
      </ConfirmDialog>
    );
  }

  return (
    <DrawerShell title={name !== "" ? `Familia ${name}` : "Familia"} icon="students" size="lg" onClose={close}>
      {askingDiscard && (
        <div style={{ marginBottom: 16 }}>
          <DiscardPrompt question="¿Descartar a este adulto?" onDiscard={onClose} onKeep={() => setAskingDiscard(false)} />
        </div>
      )}
      <FamilyDrawerBody
        drawer={props}
        state={state}
        name={name}
        members={members}
        failure={failure}
        onRenamed={(next) => { setRenamed(next); props.onChanged(); }}
        onAskRevoke={(person) => setConfirming({ kind: "revoke", person })}
        onAskDissolve={() => setConfirming({ kind: "dissolve" })}
        onAdultDirty={setAdultDirty}
      />
    </DrawerShell>
  );
}

/**
 * Focus goes back to what opened the family (APG dialog contract). DrawerShell
 * does it for itself, but a confirmation REPLACES the drawer, and the drawer
 * that comes back after it only knows the button that no longer exists. So the
 * trigger is remembered here, once, and focused after every cleanup ran.
 */
function useFocusBackToTrigger(): void {
  const trigger = useRef<Element | null>(typeof document === "undefined" ? null : document.activeElement);
  useEffect(() => {
    const el = trigger.current;
    return () => {
      setTimeout(() => {
        if (el instanceof HTMLElement && el.isConnected) el.focus();
      }, 0);
    };
  }, []);
}
