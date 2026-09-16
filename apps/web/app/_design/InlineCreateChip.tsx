"use client";

import { useEffect, useRef, useState, type KeyboardEvent, type ReactElement } from "react";

import { Icon } from "./Icon";

/**
 * El alta de un solo dato, ahí donde vive la lista.
 *
 * Niveles, grados, materias y grupos se muestran como CHIPS, no como tablas. El
 * envase correcto ahí no es un renglón: es un chip vacío que se vuelve campo.
 * Sin cajón, sin viaje, y sin convertir en tabla una fila de chips que funciona
 * bien — el rediseño no tiene por qué cobrarle a lo que ya servía.
 *
 * Un chip captura UN valor. Donde el registro trae un segundo campo, ese campo
 * nace con un default y se corrige después desde el propio registro (el orden de
 * un grado, el cupo de un grupo). Si alguna vez necesitara un tercero, el alta
 * deja de ser chip y sube a cajón compacto: el envase lo decide el número de
 * campos, y esa regla aplica también hacia arriba.
 */
export interface InlineCreateChipProps {
  /** Lo que dice el chip cerrado: "Nueva materia". */
  label: string;
  /** El nombre del campo, para quien no ve el chip. */
  fieldLabel: string;
  placeholder?: string;
  onCreate: (name: string) => Promise<void> | void;
}

export function InlineCreateChip({
  label,
  fieldLabel,
  placeholder,
  onCreate,
}: InlineCreateChipProps): ReactElement {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const ref = useRef<HTMLInputElement | null>(null);

  // El foco va en un efecto: cuando el clic que abre el chip termina de correr,
  // el input que debe recibirlo todavía no existe.
  useEffect(() => {
    if (open) ref.current?.focus();
  }, [open]);

  const save = async (): Promise<void> => {
    const name = value.trim();
    if (name === "") return;
    setSaving(true);
    try {
      await onCreate(name);
      setValue("");
      ref.current?.focus();
    } finally {
      setSaving(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === "Enter") {
      e.preventDefault();
      void save();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      setValue("");
      setOpen(false);
    }
  };

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 7,
          height: 40,
          padding: "0 16px",
          borderRadius: "var(--r-pill)",
          border: "1.5px dashed var(--border-strong)",
          background: "transparent",
          color: "var(--text-muted)",
          fontSize: 14,
          fontWeight: 600,
          cursor: "pointer",
        }}
      >
        <Icon name="plus" size={17} />
        {label}
      </button>
    );
  }

  return (
    <input
      ref={ref}
      aria-label={fieldLabel}
      value={value}
      placeholder={placeholder}
      disabled={saving}
      onChange={(e) => setValue(e.target.value.toUpperCase())}
      onKeyDown={onKeyDown}
      // Sólo se cierra solo si está VACÍO. Irse a otro lado con algo escrito no
      // puede tirarlo.
      onBlur={() => {
        if (value.trim() === "") setOpen(false);
      }}
      style={{
        height: 40,
        padding: "0 16px",
        borderRadius: "var(--r-pill)",
        border: "1.5px solid var(--forest)",
        boxShadow: "0 0 0 3px color-mix(in oklab, var(--forest) 15%, transparent)",
        background: "var(--surface)",
        color: "var(--text)",
        fontFamily: "var(--font-ui)",
        fontSize: 14,
        fontWeight: 600,
        outline: "none",
        minWidth: 200,
      }}
    />
  );
}
