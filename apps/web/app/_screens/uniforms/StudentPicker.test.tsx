import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import * as studentsApi from "../../students/api";
import { StudentPicker } from "./StudentPicker";

vi.mock("../../students/api", () => ({ listStudents: vi.fn() }));
vi.mock("../../_design/students/roomLabels", () => ({ useRoomLabels: () => new Map([["s1", "3º PRIMARIA · B"]]) }));

const pupil: studentsApi.Student = {
  id: "s1", curp: "XEXX010101HNEXXXA4", firstName: "Alumna", lastName: "Prueba",
  level: "PRIMARIA", levelId: "lv", gradeGroup: "B", guardianName: "Tutor Prueba", admissionStatus: "enrolled",
};

// ISSUE-T14-204: the counter's search said "PRUEBA ALUMNA · B" — no grade.
describe("StudentPicker", () => {
  it("names each match and the chosen pupil by classroom, grade included", async () => {
    vi.mocked(studentsApi.listStudents).mockResolvedValue([pupil]);
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = render(<StudentPicker value={null} onChange={onChange} />);

    await user.type(screen.getByLabelText("Alumno"), "Prueba");

    expect(await screen.findByRole("button", { name: /3º PRIMARIA · B/ })).toBeInTheDocument();

    rerender(<StudentPicker value={pupil} onChange={onChange} />);
    expect(screen.getByText("3º PRIMARIA · B")).toBeInTheDocument();
  });
});
