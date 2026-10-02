/**
 * The system's grade averages (T15) — ONE rule for the web, the parents' app
 * and the report card (services/grades/internal/domain/average.go runs the same
 * table of cases, gradeAverage.vectors.json):
 *
 *   - an average is the EXACT arithmetic mean of the grades that exist (a
 *     missing grade is ignored, never a zero), computed with integers — no
 *     binary float takes part in the division;
 *   - it is TRUNCATED, never rounded, to 2 decimals: 8.666… is 8.66;
 *   - it prints without trailing zeros: 8.5, 9, 8.66.
 *
 * The general average is the mean of the subjects' EXACT averages (only the
 * subjects that have one), truncated once at the end: two subjects averaging
 * 26/3 and 25/3 make 8.5, not (8.66 + 8.33) / 2 = 8.49.
 *
 * Grades are captured as integers; a stored value may still carry up to two
 * decimals (grades captured before T15), which the hundredths keep exact.
 */

const HUNDREDTHS = 100;

/** A grade in exact hundredths (8.45 → 845). Grades carry at most 2 decimals. */
function toHundredths(value: number): bigint {
  return BigInt(Math.round(value * HUNDREDTHS));
}

/** An exact non-negative fraction of hundredths. */
interface Fraction {
  readonly num: bigint;
  readonly den: bigint;
}

function gcd(a: bigint, b: bigint): bigint {
  let x = a < 0n ? -a : a;
  let y = b < 0n ? -b : b;
  while (y !== 0n) [x, y] = [y, x % y];
  return x;
}

/** The exact mean of the values, in hundredths; undefined when there are none. */
function exactMean(values: readonly number[]): Fraction | undefined {
  if (values.length === 0) return undefined;
  const sum = values.reduce((acc, v) => acc + toHundredths(v), 0n);
  return { num: sum, den: BigInt(values.length) };
}

/** A fraction of hundredths truncated to whole hundredths, as a number. */
function truncate(f: Fraction): number {
  return Number(f.num / f.den) / HUNDREDTHS;
}

/** A subject's average: the exact mean of its grades, truncated to 2 decimals. */
export function subjectAverage(values: readonly number[]): number | undefined {
  const mean = exactMean(values);
  return mean === undefined ? undefined : truncate(mean);
}

/**
 * The general average: the exact mean of the subjects' exact averages (only
 * subjects with at least one grade), truncated to 2 decimals once.
 */
export function generalAverage(subjects: readonly (readonly number[])[]): number | undefined {
  const means = subjects.map(exactMean).filter((m): m is Fraction => m !== undefined);
  if (means.length === 0) return undefined;
  let total: Fraction = { num: 0n, den: 1n };
  for (const m of means) {
    const den = total.den * m.den;
    const num = total.num * m.den + m.num * total.den;
    const g = gcd(num, den) || 1n;
    total = { num: num / g, den: den / g };
  }
  return truncate({ num: total.num, den: total.den * BigInt(means.length) });
}

/** An average as people read it: "8.5", "9", "8.66"; "" for none. */
export function formatAverage(average: number | null | undefined): string {
  if (average === undefined || average === null) return "";
  return String(Math.round(average * HUNDREDTHS) / HUNDREDTHS);
}
