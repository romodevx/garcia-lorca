/**
 * A cycle's "YYYY-MM" months as people read them: "Sep", "Oct"… with the year
 * only when a short name would repeat, plus the full name for titles.
 */

const SHORT = ["Ene", "Feb", "Mar", "Abr", "May", "Jun", "Jul", "Ago", "Sep", "Oct", "Nov", "Dic"] as const;
const FULL = ["enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"] as const;

export interface MonthChip {
  month: string;
  label: string;
  fullName: string;
}

function splitMonth(month: string): { year: string; index: number } {
  const [year = "", mm = "1"] = month.split("-");
  return { year, index: Number(mm) - 1 };
}

export function monthChips(months: readonly string[]): MonthChip[] {
  const shortOf = (m: string): string => SHORT[splitMonth(m).index] ?? m;
  const seen = new Map<string, number>();
  for (const m of months) seen.set(shortOf(m), (seen.get(shortOf(m)) ?? 0) + 1);
  return months.map((month) => {
    const { year, index } = splitMonth(month);
    const short = shortOf(month);
    return {
      month,
      label: (seen.get(short) ?? 0) > 1 ? `${short} ${year}` : short,
      fullName: `${FULL[index] ?? month} de ${year}`,
    };
  });
}
