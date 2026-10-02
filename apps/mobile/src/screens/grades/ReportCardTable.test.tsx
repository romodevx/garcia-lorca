import { render } from "@testing-library/react-native";
import type { ReportCard, ReportCardLine } from "../../api/reportCards";
import { ThemeProvider } from "../../theme";
import { lineKey, ReportCardTable } from "./ReportCardTable";

function line(subjectId: string, subjectName: string): ReportCardLine {
  return { subjectId, subjectName, periods: {}, months: {}, average: null };
}

describe("lineKey", () => {
  it("keys a row by its subject id", () => {
    expect(lineKey(line("sub-math", "Matemáticas"), 0)).toBe("sub-math");
  });

  it("falls back to position and name when the id is missing", () => {
    expect(lineKey(line("", "Matemáticas"), 3)).toBe("3-Matemáticas");
  });
});

describe("ReportCardTable rows", () => {
  it("renders two subjects that share a name as two rows", async () => {
    const card = {
      periods: [],
      months: ["2026-09"],
      lines: [line("sub-a", "Inglés"), line("sub-b", "Inglés")],
      scale: null,
    } as unknown as ReportCard;

    const view = await render(
      <ThemeProvider>
        <ReportCardTable card={card} />
      </ThemeProvider>,
    );

    expect(view.getAllByText("Inglés")).toHaveLength(2);
  });
});
