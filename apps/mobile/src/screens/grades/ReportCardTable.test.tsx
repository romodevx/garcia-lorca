import { render } from "@testing-library/react-native";
import type { ReportCard, ReportCardLine } from "../../api/reportCards";
import { ThemeProvider } from "../../theme";
import { lineKey, ReportCardTable } from "./ReportCardTable";

function line(subjectId: string, subjectName: string): ReportCardLine {
  return { subjectId, subjectName, periods: {}, letters: {}, months: {}, average: null };
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

describe("ReportCardTable — letters (preescolar)", () => {
  it("shows each period's letter and no average column", async () => {
    const card = {
      periods: [{ id: "p1", ordinal: 1, name: "1.er trimestre" }],
      months: [],
      gradingLetters: true,
      lines: [{ subjectId: "sub-len", subjectName: "Lenguajes", periods: {}, letters: { p1: "B" }, months: {}, average: null }],
      scale: null,
    } as unknown as ReportCard;

    const view = await render(
      <ThemeProvider>
        <ReportCardTable card={card} />
      </ThemeProvider>,
    );

    expect(view.getByText("B")).toBeTruthy();
    expect(view.queryByText("Prom.")).toBeNull();
    expect(view.getByLabelText("Lenguajes: 1.er trimestre B, Bien")).toBeTruthy();
  });
});

