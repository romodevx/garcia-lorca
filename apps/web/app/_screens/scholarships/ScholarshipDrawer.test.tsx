import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as api from "../../scholarships/api";
import { ScholarshipDrawer } from "./ScholarshipDrawer";

vi.mock("../../scholarships/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../scholarships/api")>();
  return { ...real, createScholarship: vi.fn(), updateScholarship: vi.fn() };
});
const m = vi.mocked(api);
afterEach(() => vi.clearAllMocks());

const CONCEPT = "concept:7d1c2a4e-5b6f-4c3d-8e9f-0a1b2c3d4e5f";
const ACTIVITY = "activity:a1000000-0000-0000-0000-000000000001";
const targets: api.ScholarshipTarget[] = [
  { key: "enrollment", label: "Inscripción", group: "enrollment" },
  { key: "tuition", label: "Colegiatura mensual", group: "tuition" },
  { key: CONCEPT, label: "Cuota de materiales", group: "concept" },
  { key: ACTIVITY, label: "Natación", group: "activity" },
];
const stored: api.Scholarship = {
  id: "b1000000-0000-0000-0000-000000000001", name: "BECA HERMANOS", kind: "percent", value: 5000,
  targets: ["tuition"], active: true, assignedCount: 14,
};

function open(scholarship?: api.Scholarship): { onSaved: ReturnType<typeof vi.fn>; onClose: ReturnType<typeof vi.fn> } {
  const onSaved = vi.fn();
  const onClose = vi.fn();
  render(<ScholarshipDrawer scholarship={scholarship} targets={targets} onClose={onClose} onSaved={onSaved} />);
  return { onSaved, onClose };
}

const save = (): HTMLElement => screen.getByRole("button", { name: /Guardar beca/ });

describe("ScholarshipDrawer (T18)", () => {
  it("is a labelled modal dialog with the targets grouped", async () => {
    open();
    const dialog = await screen.findByRole("dialog", { name: "Nueva beca" });
    const appliesTo = within(dialog).getByRole("group", { name: /Aplica a/ });
    for (const heading of ["Inscripción", "Colegiatura", "Conceptos", "Actividades"]) {
      expect(within(appliesTo).getByText(heading, { selector: "h3" })).toBeInTheDocument();
    }
    expect(within(appliesTo).getByRole("checkbox", { name: "Natación" })).not.toBeChecked();
  });

  it("saving empty marks every missing field and focuses the first; the button is never disabled", async () => {
    const user = userEvent.setup();
    open();
    await screen.findByRole("dialog");
    expect(save()).toBeEnabled();
    await user.click(save());
    expect(m.createScholarship).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Nombre")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Porcentaje de descuento")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText("Elige al menos a qué se aplica.")).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("creates a percent scholarship: the percent travels as basis points", async () => {
    const user = userEvent.setup();
    m.createScholarship.mockResolvedValue({ ...stored, name: "BECA EXCELENCIA" });
    const { onSaved } = open();
    await screen.findByRole("dialog");
    await user.type(screen.getByLabelText("Nombre"), "Beca excelencia");
    await user.type(screen.getByLabelText("Porcentaje de descuento"), "12.5");
    await user.click(screen.getByRole("checkbox", { name: "Colegiatura mensual" }));
    await user.click(screen.getByRole("checkbox", { name: "Cuota de materiales" }));
    await user.click(save());
    expect(m.createScholarship).toHaveBeenCalledWith(expect.any(String), {
      name: "BECA EXCELENCIA", kind: "percent", value: 1250, targets: ["tuition", CONCEPT], active: true,
    });
    expect(onSaved).toHaveBeenCalledWith("BECA EXCELENCIA guardada");
  });

  it("a fixed price is typed in pesos and travels in cents", async () => {
    const user = userEvent.setup();
    m.createScholarship.mockResolvedValue(stored);
    open();
    await screen.findByRole("dialog");
    await user.type(screen.getByLabelText("Nombre"), "Convenio");
    await user.click(screen.getByRole("radio", { name: "Precio fijo" }));
    await user.type(screen.getByLabelText("La familia paga"), "1000");
    await user.click(screen.getByRole("checkbox", { name: "Natación" }));
    await user.keyboard("{Enter}");
    expect(m.createScholarship).toHaveBeenCalledWith(expect.any(String), expect.objectContaining({ kind: "fixed_price", value: 100000, targets: [ACTIVITY] }));
  });

  it("a badly written value is said on leaving the field and forgiven on the keystroke that fixes it", async () => {
    const user = userEvent.setup();
    open();
    await screen.findByRole("dialog");
    const percent = screen.getByLabelText("Porcentaje de descuento");
    await user.type(percent, "12,5");
    expect(percent).not.toHaveAttribute("aria-invalid");
    await user.tab();
    expect(percent).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByText(/con punto decimal/)).toBeInTheDocument();
    await user.clear(percent);
    await user.type(percent, "12.5");
    expect(percent).not.toHaveAttribute("aria-invalid");
  });

  it("edits a stored scholarship with PUT, its values already in place", async () => {
    const user = userEvent.setup();
    m.updateScholarship.mockResolvedValue(stored);
    open(stored);
    expect(await screen.findByRole("dialog", { name: "Editar BECA HERMANOS" })).toBeInTheDocument();
    expect(screen.getByLabelText("Porcentaje de descuento")).toHaveValue("50");
    expect(screen.getByRole("checkbox", { name: "Colegiatura mensual" })).toBeChecked();
    await user.click(screen.getByRole("checkbox", { name: "Inscripción" }));
    await user.click(save());
    expect(m.updateScholarship).toHaveBeenCalledWith(expect.any(String), stored.id, expect.objectContaining({ value: 5000, targets: ["tuition", "enrollment"] }));
  });

  it("warns that deactivating takes it away from its holders", async () => {
    const user = userEvent.setup();
    open(stored);
    await screen.findByRole("dialog");
    expect(screen.queryByText(/se le quita a sus 14 alumnos/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: /Activa/ }));
    expect(screen.getByText(/se le quita a sus 14 alumnos/)).toBeInTheDocument();
  });

  it("shows the gateway's refusal and stays open", async () => {
    const user = userEvent.setup();
    m.updateScholarship.mockRejectedValue(new Error("Ya existe una beca con ese nombre."));
    const { onSaved } = open(stored);
    await screen.findByRole("dialog");
    await user.click(save());
    expect(await screen.findByText("Ya existe una beca con ese nombre.")).toBeInTheDocument();
    expect(onSaved).not.toHaveBeenCalled();
  });

  it("Cancelar closes without saving", async () => {
    const user = userEvent.setup();
    const { onClose } = open();
    await screen.findByRole("dialog");
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(onClose).toHaveBeenCalled();
    expect(m.createScholarship).not.toHaveBeenCalled();
  });
});
