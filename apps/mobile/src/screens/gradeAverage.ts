/**
 * A class's average the way a report card writes it: the arithmetic mean,
 * rounded half-up to one decimal. Computed in integer hundredths so 8.45 is
 * 8.5 and not 8.4 (binary floats store 8.45 as 8.4499…).
 *
 * Kept byte-for-byte in step with apps/web/app/grades/average.ts — both files
 * run the same test vectors.
 */

const HUNDREDTHS = 100;
const TENTHS_PER_HUNDREDTH = 10;

export function averageHalfUp(values: readonly number[]): number | undefined {
  if (values.length === 0) return undefined;
  const sum = values.reduce((acc, v) => acc + Math.round(v * HUNDREDTHS), 0);
  const divisor = values.length * TENTHS_PER_HUNDREDTH;
  // floor(sum / divisor + 1/2), in integers: grades are never negative.
  const tenths = Math.floor((2 * sum + divisor) / (2 * divisor));
  return tenths / TENTHS_PER_HUNDREDTH;
}
