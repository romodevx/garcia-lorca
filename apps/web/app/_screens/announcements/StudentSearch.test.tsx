import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import type { Student } from "../../students/api";
import { StudentSearch } from "./StudentSearch";

function pupil(id: string, firstName: string, lastName: string, gradeGroup = "1° A"): Student {
  return { id, curp: id, firstName, lastName, level: "Primaria", levelId: "pri", gradeGroup, guardianName: "", admissionStatus: "enrolled" };
}

const PUPILS = [pupil("s1", "Sofía", "Pérez"), pupil("s2", "Mateo", "Andrade", "2° B"), pupil("s3", "Valentina", "Cruz")];

/** The composer's B5 rule, so the component is exercised the way the screen uses it. */
function Harness({ students, loaded = true }: { students: Student[]; loaded?: boolean }): ReactElement {
  const [picked, setPicked] = useState<string[]>([]);
  return <StudentSearch students={students} picked={picked} onToggle={(id) => setPicked((p) => (p.includes(id) ? [] : [id]))} loaded={loaded} />;
}

const search = (): HTMLElement => screen.getByRole("searchbox", { name: "Buscar alumno" });

describe("StudentSearch", () => {
  it("lists nobody until something is typed, and says how many there are", () => {
    render(<Harness students={PUPILS} />);
    expect(screen.queryAllByRole("button")).toHaveLength(0);
    expect(screen.getByText("3 alumnos · escribe nombre o apellido")).toBeInTheDocument();
  });

  it("finds a pupil without accents or capitals, named exactly as today, with the group beside", async () => {
    const user = userEvent.setup();
    render(<Harness students={PUPILS} />);
    await user.type(search(), "perez");
    const result = screen.getByRole("button", { name: "Pérez Sofía" });
    expect(result).toHaveTextContent("Primaria · 1° A");
    expect(screen.getAllByRole("button")).toHaveLength(1);
    expect(screen.getByText("1 de 3 alumnos")).toBeInTheDocument();
  });

  it("finds a pupil by the spoken order too", async () => {
    const user = userEvent.setup();
    render(<Harness students={PUPILS} />);
    await user.type(search(), "mateo andrade");
    expect(screen.getByRole("button", { name: "Andrade Mateo" })).toBeInTheDocument();
  });

  // D10
  it("says when nobody matches", async () => {
    const user = userEvent.setup();
    render(<Harness students={PUPILS} />);
    await user.type(search(), "zz");
    expect(screen.getByText("Ningún alumno coincide con «zz».")).toBeInTheDocument();
  });

  it("shows at most eight and asks to keep typing", async () => {
    const user = userEvent.setup();
    const many = Array.from({ length: 10 }, (_, i) => pupil(`m${i}`, `Ana ${i}`, "López"));
    render(<Harness students={many} />);
    await user.type(search(), "lopez");
    expect(screen.getAllByRole("button")).toHaveLength(8);
    expect(screen.getByText("Mostrando 8 de 10. Sigue escribiendo para ver los demás.")).toBeInTheDocument();
  });

  it("keeps the chosen pupil above the results, once, whatever is typed next", async () => {
    const user = userEvent.setup();
    render(<Harness students={PUPILS} />);
    await user.type(search(), "cruz");
    await user.click(screen.getByRole("button", { name: "Cruz Valentina" }));
    await user.clear(search());
    await user.type(search(), "a");
    expect(screen.getAllByRole("button", { name: "Cruz Valentina" })).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Cruz Valentina" })).toHaveAttribute("aria-pressed", "true");
  });

  // D1: Enter in the search box never sends the announcement.
  it("does not submit the form on Enter", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn((e: { preventDefault: () => void }) => e.preventDefault());
    render(<form onSubmit={onSubmit}><Harness students={PUPILS} /><button type="submit">Enviar</button></form>);
    await user.type(search(), "cruz{Enter}");
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("says it is loading until the pupils arrive, and when there are none", () => {
    const { rerender } = render(<Harness students={[]} loaded={false} />);
    expect(screen.getByText("Cargando alumnos…")).toBeInTheDocument();
    rerender(<Harness students={[]} />);
    expect(screen.getByText("No encontramos alumnos.")).toBeInTheDocument();
  });
});
