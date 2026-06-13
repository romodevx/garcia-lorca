import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Finances } from "./Finances";

function renderFinances() {
  return render(<Finances onBack={() => {}} />);
}

describe("Finances", () => {
  it("registers a payment: flips the row to paid and shows a confirmation toast", async () => {
    const user = userEvent.setup();
    renderFinances();

    // Mateo (GL-1187) is overdue and has a pay button.
    const mateoRow = screen.getByText("GL-1187").closest("div[style]")?.parentElement?.parentElement;
    expect(mateoRow).toBeTruthy();

    const payButtons = screen.getAllByRole("button", { name: "Registrar pago" });
    const initialPayCount = payButtons.length;

    // Pay the first pending student.
    await user.click(payButtons[0]!);

    // A success toast appears.
    expect(await screen.findByRole("status")).toHaveTextContent("Pago registrado");
    // One fewer "Registrar pago" button (that row is now "Al corriente").
    expect(screen.getAllByRole("button", { name: "Registrar pago" }).length).toBe(initialPayCount - 1);
    // A new "Al corriente" indicator was added.
    expect(screen.getAllByText("Al corriente").length).toBeGreaterThan(0);
  });

  it("filters rows by status tab and shows correct counts", async () => {
    const user = userEvent.setup();
    renderFinances();

    const tablist = screen.getByRole("tablist", { name: "Filtrar por estado" });
    const overdueTab = within(tablist).getByRole("tab", { name: /Vencidos/ });

    // Seed data: 3 overdue students.
    expect(overdueTab).toHaveTextContent("3");

    await user.click(overdueTab);
    expect(overdueTab).toHaveAttribute("aria-selected", "true");

    // Only overdue rows remain — every visible status pill says "Vencido".
    const pills = screen.getAllByText("Vencido");
    expect(pills.length).toBe(3);
    expect(screen.queryByText("Pagado")).not.toBeInTheDocument();
  });

  it("filters by level pill and by search query", async () => {
    const user = userEvent.setup();
    renderFinances();

    // Level pill: Kínder has 2 students (Renata, Isabella).
    await user.click(screen.getByRole("button", { name: "Kínder" }));
    expect(screen.getByText("Renata Flores Aguilar")).toBeInTheDocument();
    expect(screen.queryByText("Sofía Hernández Ríos")).not.toBeInTheDocument();

    // Reset level, then search by matrícula.
    await user.click(screen.getByRole("button", { name: "Todos" }));
    await user.type(screen.getByLabelText("Buscar alumno o matrícula"), "GL-2041");
    expect(screen.getByText("Sofía Hernández Ríos")).toBeInTheDocument();
    expect(screen.queryByText("Mateo Domínguez Lara")).not.toBeInTheDocument();
  });

  it("renders the empty state when no rows match", async () => {
    const user = userEvent.setup();
    renderFinances();

    await user.type(screen.getByLabelText("Buscar alumno o matrícula"), "no-existe-zzz");
    expect(screen.getByText("¡Todo en orden!")).toBeInTheDocument();
  });
});
