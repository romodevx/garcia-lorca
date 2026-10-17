/**
 * The services' own reasons, in words the school reads (ISSUE-CC-113).
 *
 * Go services write their refusals in English ("user is not a guardian"), and
 * several panels printed them as they came. The rule: a person never reads
 * English. A reason this table knows is translated; one the service already
 * wrote in Spanish passes through; any other English is dropped, so the caller
 * shows its own Spanish sentence instead.
 *
 * Branch on the HTTP status or the machine code, never on this text: it is for
 * the human in front of the screen.
 */

/** What to say when the server's reason cannot be shown. */
export const GENERIC_SERVER_FAILURE = "No se pudo completar la acción. Revisa los datos e intenta de nuevo.";

const KNOWN: Readonly<Record<string, string>> = {
  // uniforms
  "that correction would leave fewer pieces than families are already holding":
    "Ese ajuste dejaría menos piezas de las que ya están apartadas para familias.",
  "an adjustment cannot take the count below what is held":
    "Ese ajuste dejaría menos piezas de las que ya están apartadas para familias.",
  "an adjustment cannot take the count below zero": "El ajuste no puede dejar la existencia por debajo de cero.",
  "an adjustment must move the count": "Escribe cuánto cambia la existencia; no puede ser cero.",
  "an adjustment needs a reason": "Escribe el motivo del ajuste.",
  "a garment with that code already exists": "Ya existe una prenda con esa clave.",
  "a receipt line must be positive": "Cada talla de la entrada necesita al menos una pieza.",
  "a receipt needs at least one line": "Escribe cuántas piezas llegaron de al menos una talla.",
  "a size needs a label": "Cada talla necesita un nombre.",
  "give at least one size": "Agrega al menos una talla.",
  "not enough stock available": "No hay suficientes piezas de esa talla.",
  "item name is required": "Escribe el nombre del artículo.",
  "item name must be 1 to 120 characters": "El nombre del artículo debe tener de 1 a 120 caracteres.",
  "a minimum cannot be negative": "El mínimo no puede ser negativo.",
  "a sale with no student needs a note saying who bought it": "Una venta sin alumno necesita una nota que diga quién compró.",
  "a cancellation needs a reason": "Escribe el motivo de la cancelación.",
  "an extension needs a reason": "Escribe el motivo de la prórroga.",
  "no such garment": "Ese artículo ya no existe. Actualiza la página.",
  "no such student": "Ese alumno ya no existe. Actualiza la página.",
  // students · contacts and family access
  "a contact without an account needs a name": "Escribe el nombre del contacto.",
  "user is not a guardian":
    "Ese correo no es de un tutor. Para dar acceso a la app, usa el correo de un padre, madre o tutor.",
  "guardian user not found": "No encontramos una cuenta con ese correo.",
  "guardian account cannot be linked in its current state":
    "Esa cuenta está suspendida o desactivada; no se le puede dar acceso.",
  "student is already enrolled this cycle": "Ya está inscrito en un grupo este ciclo. Usa «Cambiar de grupo».",
  "enrollment is not active": "Su inscripción ya no está activa.",
};

/** Words that only English uses; one of them means the text is not for the school. */
const ENGLISH = /\b(the|is|are|not|cannot|must|needs?|would|already|than|such|with|only|this|that|be|of|an)\b/i;
/** A Spanish signal: an accent, an ñ, or a word English does not use. */
const SPANISH = /[áéíóúñ¿¡]|\b(de|del|la|las|el|los|que|ya|una?|por|para|con|esa?|ese|este|esta|se|le|hay)\b/i;

function normalise(raw: string): string {
  return raw.trim().replace(/[.\s]+$/, "").toLowerCase();
}

/** The longest reason a screen shows as the server wrote it. */
const MAX_SHOWN = 200;
const ELLIPSIS = "…";

const isSpanish = (text: string): boolean => SPANISH.test(text) && !ENGLISH.test(text);

const capped = (text: string): string =>
  text.length <= MAX_SHOWN ? text : `${text.slice(0, MAX_SHOWN - ELLIPSIS.length).trimEnd()}${ELLIPSIS}`;

/**
 * The service's own sentence, without what it wrapped. A Spanish lead-in can
 * carry a driver's English after a colon ("no se pudo guardar: duplicate key
 * value…"), so that tail goes. But our own messages use the colon to say what
 * to do ("no quedan lugares en ese grado: abre otro grupo…"), and a Spanish
 * tail stays (gate B3 on #142). A very long text is a dump: it is capped.
 */
function ownSentence(raw: string): string {
  const text = raw.trim();
  const colon = text.indexOf(":");
  if (colon < 0) return capped(text);
  const tail = text.slice(colon + 1).trim();
  return capped(isSpanish(tail) ? text : text.slice(0, colon).trim());
}

/**
 * The reason, safe to show, or undefined when there is nothing safe to say.
 */
export function spanishServerReason(raw: string | undefined): string | undefined {
  if (raw === undefined || raw.trim() === "") return undefined;
  const known = KNOWN[normalise(raw)];
  if (known !== undefined) return known;
  const sentence = ownSentence(raw);
  if (sentence === "") return undefined;
  return KNOWN[normalise(sentence)] ?? (isSpanish(sentence) ? sentence : undefined);
}
