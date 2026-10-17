import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { SubjectLoad } from "../../teachers/load";
import { CurrentLoadPanel } from "./CurrentLoadPanel";

const slot = (subjectName: string, groupId: string, groupLabel: string) => ({
  assignmentId: `${subjectName}-${groupId}`,
  groupId,
  subjectId: subjectName,
  groupLabel,
  subjectName,
});

describe("CurrentLoadPanel summary", () => {
  // ISSUE-T14-014: two subjects in the SAME group read "2 asignaturas · 2 grupos".
  it("counts each group once, however many subjects the teacher has in it", () => {
    const load: SubjectLoad[] = [
      { subjectName: "ESPAÑOL", slots: [slot("ESPAÑOL", "g1", "1º A")] },
      { subjectName: "MATEMÁTICAS", slots: [slot("MATEMÁTICAS", "g1", "1º A")] },
    ];
    render(<CurrentLoadPanel load={load} />);

    expect(screen.getByText("2 asignaturas · 1 grupo")).toBeInTheDocument();
  });

  it("still counts distinct groups across subjects", () => {
    const load: SubjectLoad[] = [
      { subjectName: "ESPAÑOL", slots: [slot("ESPAÑOL", "g1", "1º A"), slot("ESPAÑOL", "g2", "1º B")] },
      { subjectName: "MATEMÁTICAS", slots: [slot("MATEMÁTICAS", "g2", "1º B")] },
    ];
    render(<CurrentLoadPanel load={load} />);

    expect(screen.getByText("2 asignaturas · 2 grupos")).toBeInTheDocument();
  });
});
