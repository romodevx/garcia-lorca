"use client";

import {
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactElement,
  type Ref,
} from "react";

import { Icon } from "./Icon";
import { HelpTip } from "./form/HelpTip";
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
  kind: "text" | "date" | "number" | "select";
  /** Sólo para `select`: lo que se puede elegir. */
  options?: readonly { value: string; label: string }[];
  /** El catálogo se captura en mayúsculas, como en el resto del sistema. */
  uppercase?: boolean;
  required?: boolean;
  /**
   * Qué va en esta celda. Sale en el `(?)` junto a la etiqueta.
   *
   * Ocupa el lugar del antiguo `placeholder`, que aquí era especialmente
   * dañino: sin encabezado de tabla, un "A" o un "T1" en gris DENTRO de la
   * celda era lo único que se veía, y se leía como un dato ya capturado.
   */
  help?: string;
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
  /** The closed row's button, for a screen that sends the cursor to it. */
  addButtonRef?: Ref<HTMLButtonElement>;
}

const etiquetaFilaStyle: CSSProperties = {
  display: "flex",
  alignItems: "baseline",
  gap: 6,
  marginBottom: 4,
};

const etiquetaStyle: CSSProperties = {
  fontSize: 12,
  fontWeight: 600,
  color: "var(--text-muted)",
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
};

const NO_RULES: Readonly<Record<string, FormatRule>> = {};

/** What a cell says when it is empty at save (ISSUE-CC-104: the red border alone said nothing). */
const missingMessage = (c: InlineColumn): string => `Falta ${c.label}.`;

const errorStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 5,
  margin: "6px 0 0",
  fontSize: 12.5,
  fontWeight: 600,
  color: "var(--bad-fg)",
};
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
  addButtonRef,
}: InlineCreateRowProps): ReactElement {
  // Un prefijo por renglón: dos tablas en la misma pantalla no se roban el id
  // de su celda "Nombre", ni la etiqueta apunta a la celda del vecino.
  const uid = useId();
  const [open, setOpen] = useState(false);
  const [values, setValues] = useState<Record<string, string>>(() => blank(columns, defaults));
  // Las celdas donde la persona ya escribió. Una propuesta puede ponerse al día
  // sola; lo que alguien escribió, no se le mueve debajo de las manos.
  const [touched, setTouched] = useState<Readonly<Record<string, true>>>({});
  // TODOS los que faltan, no sólo el primero. Marcar de uno en uno manda a
  // quien captura a descubrir el siguiente hueco hasta que se acaben; es la
  // misma decisión que `useRequired` ya había tomado para los cajones.
  const [missing, setMissing] = useState<Readonly<Record<string, true>>>({});
  const [saving, setSaving] = useState(false);
  const firstRef = useRef<HTMLInputElement | null>(null);
  // El renglón mismo, para buscar SUS celdas y no las del vecino: dos renglones
  // en la misma pantalla comparten nombres de celda (`startsOn` en días cerrados
  // y en actividades), y un `document.querySelector` mandaba el foco al otro.
  const rowRef = useRef<HTMLDivElement | null>(null);
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
    setMissing({});
    // También lo mal escrito: sin esto el renglón se reabría con las celdas
    // pintadas de rojo y el reclamo viejo debajo, sobre campos vacíos.
    format.reset();
  };

  const save = async (): Promise<void> => {
    // Dos Enter seguidos en una conexión lenta son el caso normal en un renglón
    // hecho para ir rápido, no el raro: sin esto se dan de alta dos registros.
    if (saving) return;
    const gaps = columns.filter(
      (c) => c.required === true && (values[c.key] ?? "").trim() === "",
    );
    if (gaps.length > 0) {
      setMissing(Object.fromEntries(gaps.map((c) => [c.key, true as const])));
      // El foco va al PRIMERO que falta en el orden de la pantalla, que es a
      // donde el ojo va a ir de todas formas.
      rowRef.current?.querySelector<HTMLElement>(`[data-cell="${gaps[0]?.key ?? ""}"]`)?.focus();
      return;
    }
    // Se revisan las reglas AQUÍ y no sólo al salir de la celda: Enter guarda
    // desde dentro del campo que se está escribiendo, y ese campo nunca pierde
    // el foco. Sin esta línea la regla existe y el dato imposible sale igual.
    if (!format.checkAll(values)) return;

    setSaving(true);
    try {
      await onCreate({ ...values });
      setValues(blank(columns, defaults));
      setTouched({});
      setMissing({});
      // El renglón no se desmonta entre un alta y la siguiente, así que aquí sí
      // basta con pedir el foco de vuelta.
      firstRef.current?.focus();
    } catch {
      /*
       * Se ATRAPA, y eso importa dos veces.
       *
       * Lo escrito se conserva: el panel ya dijo en su mensaje qué pasó, y
       * vaciar el renglón obligaría a reescribir todo. Y la promesa no puede
       * quedar suelta: `save()` se llama con `void` desde el manejador de
       * teclas, así que un rechazo sin atrapar sale como "unhandled rejection"
       * -- en la consola de quien usa la app, y como error de la suite.
       */
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
        ref={addButtonRef}
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
      ref={rowRef}
      onKeyDown={onKeyDown}
      style={{
        borderTop: "1px solid var(--border)",
        background: "color-mix(in oklab, var(--lime) 8%, var(--surface))",
        padding: "10px 16px",
      }}
    >
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", gap: 10 }}>
        {columns.map((c, i) => {
          const invalid = missing[c.key] === true || format.errors[c.key] !== undefined;
          const describedBy = [
            c.help !== undefined ? `${uid}-${c.key}-help` : undefined,
            invalid ? `${uid}-${c.key}-error` : undefined,
          ].filter((id) => id !== undefined).join(" ") || undefined;
          return (
            <div
              key={c.key}
              style={{ flex: c.width === undefined ? 1 : `0 0 ${c.width}px`, minWidth: 0 }}
            >
              {/*
                La etiqueta se VE, y no es adorno: este renglón no tiene
                encabezado de tabla, así que sin ella la celda no dice qué va en
                ella. Antes ese trabajo lo hacía el placeholder, y por eso un
                "A" gris parecía un grupo ya capturado.
              */}
              <div style={etiquetaFilaStyle}>
                <label htmlFor={`${uid}-${c.key}`} style={etiquetaStyle}>
                  {c.label}
                </label>
                {c.help !== undefined && (
                  <HelpTip id={`${uid}-${c.key}-help`} label={c.label} text={c.help} />
                )}
              </div>
              {c.kind === "select" ? (
                <select
                  id={`${uid}-${c.key}`}
                  data-cell={c.key}
                  aria-describedby={describedBy}
                  aria-invalid={invalid ? "true" : undefined}
                  value={values[c.key] ?? ""}
                  onChange={(e) => {
                    const all = { ...values, [c.key]: e.target.value };
                    setValues(all);
                    setTouched((prev) =>
                      prev[c.key] === true ? prev : { ...prev, [c.key]: true },
                    );
                  }}
                  style={cellStyle(invalid)}
                >
                  {(c.options ?? []).map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              ) : (
              <input
                ref={i === 0 ? firstRef : undefined}
                id={`${uid}-${c.key}`}
                data-cell={c.key}
                aria-describedby={describedBy}
                aria-invalid={invalid ? "true" : undefined}
                type={c.kind === "text" ? "text" : c.kind}
                value={values[c.key] ?? ""}
                onChange={(e) => {
                  const next =
                    c.uppercase === true ? e.target.value.toUpperCase() : e.target.value;
                  const all = { ...values, [c.key]: next };
                  setValues(all);
                  setTouched((prev) => (prev[c.key] === true ? prev : { ...prev, [c.key]: true }));
                  if (missing[c.key] === true && next.trim() !== "") {
                    setMissing((prev) => {
                      const rest = { ...prev };
                      delete rest[c.key];
                      return rest;
                    });
                  }
                  format.onChange(c.key, next, all);
                }}
                onBlur={(e) => format.onBlur(c.key, e.target.value, values)}
                style={cellStyle(invalid)}
              />
              )}
            </div>
          );
        })}
        {/*
          A visible way to save (owner, 20-oct): Enter alone was not obvious.
          Never disabled (house rule): while saving it says so and a repeat
          click is ignored by `save` itself.
        */}
        <button
          type="button"
          onClick={() => void save()}
          aria-busy={saving || undefined}
          style={{
            alignSelf: "flex-end",
            display: "inline-flex",
            alignItems: "center",
            gap: 6,
            minHeight: 44,
            padding: "0 14px",
            borderRadius: "var(--r-md)",
            border: "none",
            background: "var(--primary)",
            color: "var(--primary-ink)",
            fontWeight: 700,
            fontSize: 14.5,
            whiteSpace: "nowrap",
            cursor: "pointer",
          }}
        >
          <Icon name="check" size={18} stroke={2.6} />
          {saving ? "Guardando…" : "Guardar"}
        </button>
        <button
          type="button"
          onClick={cancel}
          aria-label="Cancelar"
          style={{
            alignSelf: "flex-end",
            minWidth: 44,
            minHeight: 44,
            borderRadius: "var(--r-md)",
            background: "transparent",
            border: "1px solid var(--border)",
            color: "var(--text-muted)",
            cursor: "pointer",
            display: "grid",
            placeItems: "center",
          }}
        >
          <Icon name="close" size={18} />
        </button>
      </div>
      <p style={{ margin: "6px 0 0", fontSize: 12.5, color: "var(--text-faint)" }}>También puedes presionar Enter.</p>

      {columns.map((c) => {
        const message = format.errors[c.key] ?? (missing[c.key] === true ? missingMessage(c) : undefined);
        if (message === undefined) return null;
        return (
          <p key={c.key} id={`${uid}-${c.key}-error`} role="alert" style={errorStyle}>
            <Icon name="warning" size={14} stroke={2.4} />
            {message}
          </p>
        );
      })}
    </div>
  );
}
