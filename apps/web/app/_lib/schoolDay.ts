/**
 * The school's calendar day (ADR 0035). García Lorca is in Morelia, and every
 * "today" the server decides — the counter's default payment date, its "no
 * future date" check, the cash cut's day — is Morelia's. The browser must
 * agree with it whatever zone the device is set to: UTC's day turns at 18:00
 * local, and a laptop set to another zone turns at yet another hour.
 */
export const SCHOOL_TIME_ZONE = "America/Mexico_City";

const schoolDayParts = new Intl.DateTimeFormat("en-US", {
  timeZone: SCHOOL_TIME_ZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

/** The school's calendar day at an instant, as aaaa-mm-dd. */
export function schoolISODate(at: Date = new Date()): string {
  const part = (type: Intl.DateTimeFormatPartTypes): string =>
    schoolDayParts.formatToParts(at).find((p) => p.type === type)?.value ?? "";
  return `${part("year")}-${part("month")}-${part("day")}`;
}

/** The first day of the school's current month, as aaaa-mm-dd. */
export function schoolMonthStart(at: Date = new Date()): string {
  return `${schoolISODate(at).slice(0, 7)}-01`;
}
