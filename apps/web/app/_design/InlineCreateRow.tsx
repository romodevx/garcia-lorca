"use client";

import {
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactElement,
} from "react";

import { Icon } from "./Icon";
import { useFormatError, type FormatRule } from "./form/useFormatError";

/**
 * Dar de alta sin irse a ningún lado.
 *
 * Un registro de uno a cuatro campos no merece un cajón: el viaje —abrirlo,
 * mirar a otro lado, guardar, volver, buscar dónde quedó el renglón— cuesta más
 * que la captura misma. Aquí el alta ocurre al pie de la lista que ya se está
 * viendo, y Enter guarda y reabre, para que doce feriados sean doce Enter.
 *
 * Carbon lo dice para este caso exacto: "si un usuario necesita realizar una
 * tarea repetidamente, considere hacer la tarea completable en la página
 * principal".
 *
 * El error vive EN LA CELDA, y eso es el punto: el defecto que este trabajo vino
 * a arreglar era un mensaje rojo dibujado a metros del campo que lo causaba.
 */
export interface InlineColumn {
  key: string;
  label: string;
  kind: "text" | "date" | "number";
  /** El catálogo se captura en mayúsculas, como en el resto del sistema. */
  uppercase?: boolean;
  required?: boolean;
  placeholder?: string;
  /** Ancho fijo de la celda; sin él reparte el espacio disponible. */
  width?: number;
}

export interface InlineCreateRowProps {
  columns: readonly InlineColumn[];
  onCreate: (values: Record<string, string>) => Promise<void>;
  /** Lo que dice el renglón cerrado. "Agregar día cerrado" gana a "Agregar". */
  addLabel: string;
  /**
   * Lo que está mal escrito, revisado al salir de la celda. Lo que FALTA no va
   * aquí: eso se reclama al guardar, sobre la celda vacía.
   */
  rules?: Readonly<Record<string, FormatRule>>;
  /**
   * Lo que el sistema ya sabe y por lo tanto no pregunta: el orden que sigue, el
   * cupo del último salón. Llega escrito, así que Enter solo basta — pero SIGUE
   * SIENDO UN CAMPO, porque quitarlo quitaría la capacidad de intercalar un
   * grado o abrir un salón más chico, y no hay otra pantalla donde corregirlo.
   */
  defaults?: Readonly<Record<string, string>>;
}

const NO_RULES: Readonly<Record<string, FormatRule>> = {};
const NO_DEFAULTS: Readonly<Record<string, string>> = {};

const blank = (
  columns: readonly InlineColumn[],
  defaults: Readonly<Record<string, string>>,
): Record<string, string> =>
  Object.fromEntries(columns.map((c) => [c.key, defaults[c.key] ?? ""]));

function cellStyle(invalid: boolean): CSSProperties {
  return {
    height: 42,
    width: "100%",
    minWidth: 0,
    padding: "0 12px",
    borderRadius: "var(--r-md)",
    border: `1px solid ${invalid ? "var(--red)" : "var(--border)"}`,
    boxShadow: invalid
      ? "0 0 0 3px color-mix(in oklab, var(--red) 22%, transparent)"
      : "none",
    background: "var(--surface)",
    color: "var(--text)",
    fontFamily: "var(--font-ui)",
    fontSize: 15,
    outline: "none",
  };
}

export function InlineCreateRow({
  columns,
  onCreate,
  addLabel,
  rules = NO_RULES,
  defaults = NO_DEFAULTS,
}: InlineCreateRowProps): ReactElement {
  const [open, setOpen] = useState(false);
  const [values, setValues] = useState<Record<string, string>>(() => blank(columns, defaults));
  // Las celdas donde la persona ya escribió. Una propuesta puede ponerse al día
  // sola; lo que alguien escribió, no se le mueve debajo de las manos.
  const [touched, setTouched] = useState<Readonly<Record<string, true>>>({});
  const [missing, setMissing] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const firstRef = useRef<HTMLInputElement | null>(null);
  const format = useFormatError(rules);

  // El foco va en un efecto y no en el manejador del clic: cuando el clic
  // termina de correr el input todavía no existe, porque es este mismo clic el
  // que lo crea.
  useEffect(() => {
    if (open) firstRef.current?.focus();
  }, [open]);

  // La lista crece al guardar, así que la propuesta cambia (el orden que sigue
  // ya no es el mismo). Se pone al día sola donde nadie ha escrito.
  useEffect(() => {
    setValues((prev) => {
      let next = prev;
      for (const [key, proposed] of Object.entries(defaults)) {
        if (touched[key] === true || prev[key] === proposed) continue;
        if (next === prev) next = { ...prev };
        next[key] = proposed;
      }
      return next;
    });
  }, [defaults, touched]);

  const cancel = (): void => {
    setOpen(false);
    setValues(blank(columns, defaults));
    setTouched({});
    setMissing(undefined);
  };

  const save = async (): Promise<void> => {
    const gap = columns.find(
      (c) => c.required === true && (values[c.key] ?? "").trim() === "",
    );
    if (gap !== undefined) {
      setMissing(gap.key);
      document.querySelector<HTMLInputElement>(`[data-cell="${gap.key}"]`)?.focus();
      return;
    }
    if (Object.keys(format.errors).length > 0) return;

    setSaving(true);
    try {
      await onCreate({ ...values });
      setValues(blank(columns, defaults));
      setTouched({});
      setMissing(undefined);
      // El renglón no se desmonta entre un alta y la siguiente, así que aquí sí
      // basta con pedir el foco de vuelta.
      firstRef.current?.focus();
    } finally {
      setSaving(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>): void => {
    if (e.key === "Enter") {
      e.preventDefault();
      void save();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  };

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 9,
          width: "100%",
          padding: "13px 16px",
          borderTop: "1px solid var(--border)",
          background: "transparent",
          color: "var(--text-muted)",
          fontSize: 14.5,
          fontWeight: 600,
          cursor: "pointer",
        }}
      >
        <Icon name="plus" size={18} />
        {addLabel}
      </button>
    );
  }

  return (
    <div
      onKeyDown={onKeyDown}
      style={{
        borderTop: "1px solid var(--border)",
        background: "color-mix(in oklab, var(--lime) 8%, var(--surface))",
        padding: "10px 16px",
      }}
    >
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        {columns.map((c, i) => {
          const invalid = missing === c.key || format.errors[c.key] !== undefined;
          return (
            <div
              key={c.key}
              style={{ flex: c.width === undefined ? 1 : `0 0 ${c.width}px`, minWidth: 0 }}
            >
              <input
                ref={i === 0 ? firstRef : undefined}
                data-cell={c.key}
                aria-label={c.label}
                aria-invalid={invalid ? "true" : undefined}
                type={c.kind === "text" ? "text" : c.kind}
                placeholder={c.placeholder}
                value={values[c.key] ?? ""}
                onChange={(e) => {
                  const next =
                    c.uppercase === true ? e.target.value.toUpperCase() : e.target.value;
                  const all = { ...values, [c.key]: next };
                  setValues(all);
                  setTouched((prev) => (prev[c.key] === true ? prev : { ...prev, [c.key]: true }));
                  if (missing === c.key && next.trim() !== "") setMissing(undefined);
                  format.onChange(c.key, next, all);
                }}
                onBlur={(e) => format.onBlur(c.key, e.target.value, values)}
                style={cellStyle(invalid)}
              />
            </div>
          );
        })}
        <span style={{ fontSize: 12, color: "var(--text-faint)", whiteSpace: "nowrap" }}>
          {saving ? "Guardando…" : "Enter guarda"}
        </span>
        <button
          type="button"
          onClick={cancel}
          aria-label="Cancelar"
          style={{
            background: "transparent",
            border: "none",
            color: "var(--text-muted)",
            cursor: "pointer",
            display: "grid",
            placeItems: "center",
          }}
        >
          <Icon name="close" size={18} />
        </button>
      </div>

      {Object.entries(format.errors).map(([key, message]) => (
        <p
          key={key}
          role="alert"
          style={{
            display: "flex",
            alignItems: "center",
            gap: 5,
            margin: "6px 0 0",
            fontSize: 12.5,
            fontWeight: 600,
            color: "var(--bad-fg)",
          }}
        >
          <Icon name="warning" size={14} stroke={2.4} />
          {message}
        </p>
      ))}
    </div>
  );
}
