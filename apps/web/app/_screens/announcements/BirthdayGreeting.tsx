"use client";

import { useEffect, useState, type FormEvent, type ReactElement } from "react";
import { Btn } from "../../_design/Controls";
import { TextArea, TextField, useFocusFirstInvalid } from "../../_design/form";
import { Icon } from "../../_design/Icon";
import { getBirthdayTemplate, updateBirthdayTemplate } from "../../comms/api";
import { API_URL } from "../../api-url";

/** Where the school's text says the pupil's name. */
const NAME_PLACEHOLDER = "{nombre}";

/** The name the preview greets: what the family will read, with a real name in it. */
const SAMPLE_NAME = "Ana";

/** The limits comms enforces, in characters. */
const MAX_TITLE = 80;
const MAX_BODY = 500;

/** The greeting's title id; a holder that draws the title itself gives it this id. */
export const BIRTHDAY_HEADING_ID = "cumple-titulo";

const CARD_SURFACE = { background: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--r-xl)", padding: "22px 24px" } as const;
const FORM_LAYOUT = { display: "flex", flexDirection: "column", gap: 16 } as const;

/** The text with the pupil's first name wherever the school wrote {nombre}. */
export function renderGreeting(text: string, firstName: string): string {
  return text.split(NAME_PLACEHOLDER).join(firstName);
}

type Errors = Partial<Record<"title" | "body", string>>;

function titleError(title: string): string | undefined {
  if (title.trim() === "") return "Escribe el título de la felicitación.";
  if ([...title.trim()].length > MAX_TITLE) return `El título lleva como máximo ${MAX_TITLE} caracteres.`;
  return undefined;
}

function bodyError(body: string): string | undefined {
  if (body.trim() === "") return "Escribe el mensaje de la felicitación.";
  if ([...body.trim()].length > MAX_BODY) return `El mensaje lleva como máximo ${MAX_BODY} caracteres.`;
  return undefined;
}

/** What is wrong with the greeting, by field; empty when it can be saved. */
function validate(title: string, body: string): Errors {
  return { title: titleError(title), body: bodyError(body) };
}

/**
 * The birthday greeting the school sends (T11).
 *
 * Every morning at 7:00 the system greets, in the parents' app, each pupil
 * whose birthday it is. This card is where the school writes what it says.
 * Two fields, so it edits in place (no drawer), with a live preview carrying a
 * real name — `{nombre}` reads as a typo until you see it replaced.
 */
export interface BirthdayGreetingProps {
  /**
   * Inside a panel that already draws the card, the star and the title (the
   * folded panel of Avisos, PLAN-redesign-avisos D13). Presentation only.
   */
  embedded?: boolean;
}

export function BirthdayGreeting({ embedded = false }: BirthdayGreetingProps): ReactElement {
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [isDefault, setIsDefault] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [errors, setErrors] = useState<Errors>({});
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const focusFirstInvalid = useFocusFirstInvalid();

  useEffect(() => {
    let alive = true;
    getBirthdayTemplate(API_URL)
      .then((t) => {
        if (!alive) return;
        setTitle(t.title);
        setBody(t.body);
        setIsDefault(t.isDefault);
        setLoaded(true);
      })
      .catch(() => alive && setFailure("No pudimos leer la felicitación de cumpleaños."));
    return () => {
      alive = false;
    };
  }, []);

  const clear = (key: keyof Errors): void => {
    setSaved(false);
    setErrors((prev) => ({ ...prev, [key]: undefined }));
  };

  const save = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    // A second press while saving is ignored rather than the button disabled:
    // a primary action never goes dead (ui-forms-and-modals.md).
    if (saving) return;
    const found = validate(title, body);
    setErrors(found);
    if (Object.values(found).some((m) => m !== undefined)) {
      focusFirstInvalid();
      return;
    }
    setSaving(true);
    setFailure(undefined);
    try {
      const t = await updateBirthdayTemplate(API_URL, { title: title.trim(), body: body.trim() });
      setTitle(t.title);
      setBody(t.body);
      setIsDefault(t.isDefault);
      setSaved(true);
    } catch (cause) {
      setFailure(cause instanceof Error ? cause.message : "No se pudo guardar la felicitación.");
    } finally {
      setSaving(false);
    }
  };

  if (!loaded) {
    // Nothing editable until the current text has arrived: a slow answer must
    // not overwrite what somebody typed, and a failed one must not let them
    // replace a text they never saw.
    return (
      <section aria-labelledby={BIRTHDAY_HEADING_ID} className={embedded ? undefined : "fade-up"} style={embedded ? undefined : { marginTop: 24, ...CARD_SURFACE }}>
        {!embedded && <h2 id={BIRTHDAY_HEADING_ID} style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", margin: 0 }}>Felicitación de cumpleaños</h2>}
        {failure !== undefined
          ? <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: "8px 0 0" }}>{failure} Recarga la página para intentarlo de nuevo.</p>
          : <p style={{ color: "var(--text-muted)", fontSize: 14, margin: "8px 0 0" }}>Cargando…</p>}
      </section>
    );
  }

  return (
    <form
      onSubmit={(e) => void save(e)}
      aria-labelledby={BIRTHDAY_HEADING_ID}
      className={embedded ? undefined : "fade-up"}
      style={embedded ? FORM_LAYOUT : { marginTop: 24, ...CARD_SURFACE, boxShadow: "var(--shadow-card)", ...FORM_LAYOUT }}
    >
      {embedded ? <WhenItGoes isDefault={isDefault} /> : <CardHeader isDefault={isDefault} />}

      <TextField
        label="Título de la felicitación"
        value={title}
        onChange={setTitle}
        required
        error={errors.title}
        onClearError={() => clear("title")}
        onBlur={() => setErrors((prev) => ({ ...prev, title: titleError(title) }))}
        help={`Lo que se ve en la notificación del celular. Escribe ${NAME_PLACEHOLDER} donde quieras el nombre del alumno. Hasta ${MAX_TITLE} caracteres.`}
      />
      <TextArea
        label="Mensaje de cumpleaños"
        value={body}
        onChange={setBody}
        rows={3}
        required
        error={errors.body}
        onClearError={() => clear("body")}
        onBlur={() => setErrors((prev) => ({ ...prev, body: bodyError(body) }))}
        help={`El mensaje completo. ${NAME_PLACEHOLDER} se cambia por el nombre del alumno. Hasta ${MAX_BODY} caracteres.`}
      />

      <div aria-label="Vista previa" style={{ padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--surface-3)", border: "1px solid var(--border)" }}>
        <p style={{ fontSize: 12.5, fontWeight: 700, color: "var(--text-muted)", textTransform: "uppercase", letterSpacing: 0.4, margin: 0 }}>Así lo leen los papás</p>
        <p style={{ fontSize: 15, fontWeight: 700, color: "var(--text)", margin: "6px 0 2px" }}>{renderGreeting(title, SAMPLE_NAME)}</p>
        <p style={{ fontSize: 14.5, color: "var(--text)", margin: 0, lineHeight: 1.5 }}>{renderGreeting(body, SAMPLE_NAME)}</p>
      </div>

      {failure !== undefined && <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>{failure}</p>}
      {saved && <p role="status" style={{ color: "var(--ok-fg)", fontSize: 14.5, margin: 0 }}>Felicitación guardada. Se usa desde el próximo envío.</p>}

      <div style={{ display: "flex" }}>
        <div style={{ flex: 1 }} />
        <Btn icon="check" type="submit" style={{ opacity: saving ? 0.5 : 1, height: 48 }}>
          {saving ? "Guardando…" : "Guardar felicitación"}
        </Btn>
      </div>
    </form>
  );
}

/** The star, the title and when it goes out: the card's own head. */
function CardHeader({ isDefault }: { isDefault: boolean }): ReactElement {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
      <span style={{ width: 40, height: 40, borderRadius: 12, display: "grid", placeItems: "center", background: "color-mix(in oklab, var(--orange) 16%, var(--surface))", color: "var(--orange)", flexShrink: 0 }}>
        <Icon name="star" size={20} />
      </span>
      <div>
        <h2 id={BIRTHDAY_HEADING_ID} style={{ fontSize: 18, fontWeight: 800, color: "var(--text)", margin: 0 }}>Felicitación de cumpleaños</h2>
        <WhenItGoes isDefault={isDefault} />
      </div>
    </div>
  );
}

function WhenItGoes({ isDefault }: { isDefault: boolean }): ReactElement {
  return (
    <p style={{ color: "var(--text-muted)", fontSize: 14, margin: "2px 0 0" }}>
      Llega sola a la app de los papás cada mañana a las 7:00, el día que el alumno cumple años.
      {isDefault && " Hoy se envía el texto predeterminado."}
    </p>
  );
}
