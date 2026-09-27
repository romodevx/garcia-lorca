import { averageHalfUp } from "./gradeAverage";

// Shared vectors: apps/web/app/grades/average.test.ts runs the same table.
const VECTORS: readonly [readonly number[], number][] = [
  [[8.45], 8.5],
  [[8.25], 8.3],
  [[5.55], 5.6],
  [[5.5], 5.5],
  [[8.4, 8.5], 8.5],
  [[8, 8.5], 8.3],
  [[5.5, 5.6], 5.6],
  [[5, 6], 5.5],
  [[9, 5.5], 7.3],
  [[10, 10, 9], 9.7],
];

describe("averageHalfUp", () => {
  it.each(VECTORS)("mean of %j is %d", (values, expected) => {
    expect(averageHalfUp(values)).toBe(expected);
  });

  it("is undefined for no values", () => {
    expect(averageHalfUp([])).toBeUndefined();
  });
});
