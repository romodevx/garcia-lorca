"use client";

import { useState, type ReactElement } from "react";

import { SlideOver } from "../../_design/SlideOver";
import { TextField } from "../../_design/form";
import { API_URL } from "../../api-url";
import { createFamily, familyRefusal, type Family } from "../../families/api";
import { proposedFamilyName } from "../../families/model";
import type { FamilyPupil } from "./familyPupil";
import { PupilChips } from "./PupilChips";
import { PupilSearch } from "./PupilSearch";

export interface FamilyDraft {
  name: string;
  pupils: readonly FamilyPupil[];
}

const NO_PUPIL = "Elige al menos un alumno";

/**
 * Two fields — who and how the family is called — in a drawer, because picking
 * pupils needs a search and a list that an inline row cannot hold. The name
 * follows the first pupil's surnames until the office types its own.
 * "Ajustar" opens it with a suggestion already in it.
 */
export function FamilyCreateDrawer({ candidates, draft, onClose, onCreated }: {
  /** Pupils without a family. */
  candidates: readonly FamilyPupil[];
  draft?: FamilyDraft;
  onClose: () => void;
  onCreated: (family: Family) => void;
}): ReactElement {
  const [chosen, setChosen] = useState<readonly FamilyPupil[]>(draft?.pupils ?? []);
  const [ownName, setOwnName] = useState<string | undefined>(draft?.name);
  const [missing, setMissing] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const name = ownName ?? proposedFamilyName(chosen[0]);
  const offered = candidates.filter((c) => !chosen.some((p) => p.id === c.id));

  const submit = async (): Promise<void> => {
    if (chosen.length === 0) {
      setMissing(true);
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      onCreated(await createFamily(API_URL, { name, studentIds: chosen.map((p) => p.id) }));
    } catch (cause) {
      setError(familyRefusal(cause, "No se pudo crear la familia."));
      setPending(false);
    }
  };

  return (
    // Esc and the veil close it only while nothing was picked: a family half
    // put together is not thrown away by a stray key (Cancel still closes).
    <SlideOver title="Nueva familia" subtitle="Hermanos que comparten papás" icon="students" submitLabel="Crear familia" pending={pending} error={error} onClose={onClose} onSubmit={() => void submit()} dismissible={chosen.length === 0}>
      <div style={{ display: "grid", gap: 16 }}>
        <PupilSearch
          label="Buscar alumno sin familia"
          candidates={offered}
          onPick={(p) => {
            setChosen((prev) => [...prev, p]);
            setMissing(false);
          }}
        />
        <PupilChips pupils={chosen} onRemove={(p) => setChosen((prev) => prev.filter((c) => c.id !== p.id))} />
        {missing && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{NO_PUPIL}</p>}
        <TextField
          label="Nombre de la familia"
          value={name}
          onChange={setOwnName}
          uppercase
          help="Cómo la llama la oficina. Si lo dejas vacío, toma los apellidos del primer alumno."
        />
      </div>
    </SlideOver>
  );
}
