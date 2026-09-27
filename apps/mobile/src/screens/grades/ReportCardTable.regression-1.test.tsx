// Regression: ISSUE-001 — with the phone's large text, the boleta table broke a
// one-word subject mid-word ("MATEMÁTIC / AS"): the subject column had a fixed
// width while its text grew with the system font scale.
// Found by /qa on 2026-09-27
// Report: .gstack/qa-reports/qa-report-localhost-3002-2026-09-27-t8.md
import { render } from "@testing-library/react-native";
import { StyleSheet } from "react-native";
import type { ReportCard } from "../../api/reportCards";
import { ThemeProvider } from "../../theme";
import { ReportCardTable } from "./ReportCardTable";

const mockFontScale = { value: 1 };
jest.mock("react-native/Libraries/Utilities/useWindowDimensions", () => ({
  __esModule: true,
  default: () => ({ width: 411, height: 914, scale: 2.625, fontScale: mockFontScale.value }),
}));

const card = {
  id: "rc1",
  version: 1,
  issuedAt: "2026-12-10T18:00:00Z",
  termName: "TRIMESTRE 1",
  cycleName: "2026-2027",
  months: ["2026-09", "2026-10", "2026-11"],
  lines: [{ subjectName: "MATEMÁTICAS", months: { "2026-09": 9 }, average: 9 }],
  generalAverage: 9,
  scale: { min: 5, max: 10, decimals: 1, passing: 6 },
  hasGrades: true,
} as unknown as ReportCard;

async function subjectWidthAt(fontScale: number): Promise<number> {
  mockFontScale.value = fontScale;
  const view = await render(
    <ThemeProvider>
      <ReportCardTable card={card} />
    </ThemeProvider>,
  );
  const style = StyleSheet.flatten(view.getByText("MATEMÁTICAS").props.style) as { width?: number };
  await view.unmount();
  return style.width ?? 0;
}

describe("ReportCardTable subject column and system text size", () => {
  afterEach(() => {
    mockFontScale.value = 1;
  });

  it("keeps the base width at the default text size", async () => {
    expect(await subjectWidthAt(1)).toBeGreaterThan(0);
  });

  it("grows the subject column with the font scale so a word never splits", async () => {
    const base = await subjectWidthAt(1);
    expect(await subjectWidthAt(1.3)).toBeCloseTo(base * 1.3, 5);
  });

  it("never shrinks it below the base width when the text is smaller", async () => {
    const base = await subjectWidthAt(1);
    expect(await subjectWidthAt(0.85)).toBe(base);
  });
});
