"use client";

import { useState, type FormEvent, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { TextField, useRequired } from "../../_design/form";
import { API_URL } from "../../api-url";
import { familyRefusal, renameFamily } from "../../families/api";

const NAME_REQUIRED = ["name"] as const;
const SMALL = { height: 44, padding: "0 16px", fontSize: 14.5 } as const;

/** The family's name, corrected where it stands (one field: no dialog). */
export function FamilyNameEditor({ familyId, name, onRenamed }: { familyId: string; name: string; onRenamed: (name: string) => void }): ReactElement {
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(name);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const required = useRequired(NAME_REQUIRED);

  if (!editing) {
    return (
      <Btn kind="ghost" icon="edit" onClick={() => { setValue(name); setEditing(true); }} style={SMALL}>Renombrar familia</Btn>
    );
  }

  const save = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (saving || !required.check({ name: value })) return;
    setSaving(true);
    setError(undefined);
    try {
      const saved = await renameFamily(API_URL, familyId, value);
      onRenamed(saved.name);
      setEditing(false);
    } catch (cause) {
      setError(familyRefusal(cause, "No se pudo cambiar el nombre."));
    } finally {
      setSaving(false);
    }
  };

  return (
    <form onSubmit={(e) => void save(e)} onKeyDown={(e) => { if (e.key === "Escape") { e.stopPropagation(); setEditing(false); } }} style={{ display: "flex", gap: 8, flexWrap: "wrap", alignItems: "flex-end" }}>
      <div style={{ flex: "1 1 220px", minWidth: 0 }}>
        <TextField label="Nombre de la familia" value={value} onChange={setValue} uppercase required error={required.errors.name} onClearError={() => required.clear("name")} />
      </div>
      <Btn type="submit" icon="check" busy={saving} style={SMALL}>Guardar nombre</Btn>
      <Btn kind="ghost" onClick={() => setEditing(false)} style={SMALL}>Cancelar</Btn>
      {error !== undefined && <p role="alert" style={{ flexBasis: "100%", color: "var(--bad-fg)", fontSize: 13.5, margin: 0 }}>{error}</p>}
    </form>
  );
}
