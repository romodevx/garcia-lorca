/**
 * The format rules of a pupil's file, as the web checks them on blur. The
 * students service applies the same rules; these only spare a round trip.
 */

const PHONE_DIGITS = 10;
const POSTAL_CODE = /^\d{5}$/;

/** A phone as the service stores it: its digits alone. */
export function phoneDigits(raw: string): string {
  return raw.replace(/\D/g, "");
}

export function phoneError(raw: string): string | undefined {
  return phoneDigits(raw).length === PHONE_DIGITS ? undefined : "Un teléfono lleva 10 dígitos.";
}

export function postalCodeError(raw: string): string | undefined {
  return POSTAL_CODE.test(raw.trim()) ? undefined : "El código postal lleva 5 dígitos.";
}

/** Both ISO dates (aaaa-mm-dd), so comparing the strings compares the days. */
export function birthDateError(value: string, today: string): string | undefined {
  return value > today ? "La fecha de nacimiento no puede ser futura." : undefined;
}

/** The 32 federal entities, spelled as the service expects them. */
export const MEXICAN_STATES = [
  "Aguascalientes", "Baja California", "Baja California Sur", "Campeche", "Chiapas", "Chihuahua",
  "Ciudad de México", "Coahuila", "Colima", "Durango", "Estado de México", "Guanajuato", "Guerrero",
  "Hidalgo", "Jalisco", "Michoacán", "Morelos", "Nayarit", "Nuevo León", "Oaxaca", "Puebla",
  "Querétaro", "Quintana Roo", "San Luis Potosí", "Sinaloa", "Sonora", "Tabasco", "Tamaulipas",
  "Tlaxcala", "Veracruz", "Yucatán", "Zacatecas",
] as const;
