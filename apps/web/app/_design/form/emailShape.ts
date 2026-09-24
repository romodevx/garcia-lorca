/**
 * The shape an address must have, in Spanish, for the person typing it — the
 * same rule the server applies (pkg/email.Validate): no inner spaces, exactly
 * one "@" with something before it, and a dotted domain.
 *
 * It exists because EmailField is `type="text"` (so lowering keeps the caret):
 * the browser no longer judges the address, and the server's refusal would
 * otherwise arrive in English, after the save.
 *
 * Returns undefined when the address is plausible, or empty — whether an empty
 * email is allowed is the form's call, not this rule's.
 */
export function emailShapeError(raw: string): string | undefined {
  const value = raw.trim().toLowerCase();
  if (value === "" || plausible(value)) {
    return undefined;
  }
  return "Revisa el correo: debe verse como nombre@dominio.mx.";
}

function plausible(value: string): boolean {
  if (/\s/.test(value)) return false;
  const at = value.indexOf("@");
  if (at <= 0 || value.indexOf("@", at + 1) !== -1) return false;
  const domain = value.slice(at + 1);
  const dot = domain.indexOf(".");
  return dot > 0 && dot !== domain.length - 1;
}
