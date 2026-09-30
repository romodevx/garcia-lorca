import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import type { Student } from "../../students/api";
import { StudentDirectoryPanel } from "./StudentDirectoryPanel";

const mk = (id: string, first: string, last: string, group: string, curp: string, guardian: string): Student => ({
  id,
  firstName: first,
  lastName: last,
  level: "primaria",
  levelId: "",
  gradeGroup: group,
  curp,
  guardianName: guardian,
  admissionStatus: "enrolled",
});

describe("StudentDirectoryPanel", () => {
  const students = [
    mk("1", "Valentina", "Gutiérrez", "1° A", "AAAA000000MDFRRN01", "Ana Gutiérrez"),
    mk("2", "Mateo", "Sánchez", "2° A", "BBBB000000HDFRRN02", "Luis Sánchez"),
  ];

  it("lists every student with its count", () => {
    render(<StudentDirectoryPanel students={students} />);
    expect(screen.getByText("Gutiérrez Valentina")).toBeInTheDocument();
    expect(screen.getByText("Sánchez Mateo")).toBeInTheDocument();
    expect(screen.getByText(/2 alumnos/)).toBeInTheDocument();
  });

  it("filters by the search box (name, curp, group or guardian)", async () => {
    const user = userEvent.setup();
    render(<StudentDirectoryPanel students={students} />);

    await user.type(screen.getByLabelText("Buscar"), "mateo");
    expect(screen.getByText("Sánchez Mateo")).toBeInTheDocument();
    expect(screen.queryByText("Gutiérrez Valentina")).not.toBeInTheDocument();
  });

  it("shows an empty hint when nothing matches", async () => {
    const user = userEvent.setup();
    render(<StudentDirectoryPanel students={students} />);

    await user.type(screen.getByLabelText("Buscar"), "zzz-nobody");
    expect(screen.getByText(/Sin resultados/)).toBeInTheDocument();
  });
});
