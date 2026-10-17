import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ConfigError } from "../../config/api";
import { CycleBar, cycleFailureMessage } from "./CycleBar";

// Deliberately its own file: Catalog.test.tsx mocks the whole config/api module,
// and these need the REAL ConfigError.
describe("cycleFailureMessage", () => {
  /**
   * The commonest failure is a name the school already used, and the old message
   * for it was "Intenta de nuevo" — advice a person can follow forever without
   * getting anywhere. Every branch here has to point at something the user can
   * actually change.
   */
  it("names a duplicate instead of suggesting a retry that cannot work", () => {
    const message = cycleFailureMessage(new ConfigError(409, "Conflict"));
    expect(message).toContain("Ya existe un ciclo con ese nombre");
    expect(message).not.toContain("Intenta de nuevo");
  });

  it("points at the dates when they are the problem", () => {
    expect(cycleFailureMessage(new ConfigError(400, "Bad Request"))).toContain("fechas");
  });

  it("says the session expired rather than blaming the form", () => {
    expect(cycleFailureMessage(new ConfigError(401, ""))).toContain("sesión");
  });

  it("says it is a permission when it is", () => {
    expect(cycleFailureMessage(new ConfigError(403, ""))).toContain("permiso");
  });

  it("falls back to something honest for anything else", () => {
    expect(cycleFailureMessage(new Error("network down"))).toContain("conexión");
  });
});

describe("putting a school year into service", () => {
  const cycles = [
    { id: "c1", name: "2025-2026", status: "active" as const, startsOn: "2025-08-01", endsOn: "2026-07-01" },
    { id: "c2", name: "2026-2027", status: "draft" as const, startsOn: "2026-08-31", endsOn: "2027-07-09" },
  ];

  // The complaint that started this: every year read "Borrador" and nothing on
  // the screen could change it, because nothing in the system could.
  it("offers to activate the selected draft year", async () => {
    const user = userEvent.setup();
    const onActivate = vi.fn().mockResolvedValue(undefined);
    render(
      <CycleBar cycles={cycles} selectedCycleId="c2" onSelect={() => {}} onCreate={vi.fn()} onActivate={onActivate} />,
    );

    await user.click(screen.getByRole("button", { name: /Activar ciclo/ }));
    // The live 2025-2026 is paused, so it asks first (QA CC-102).
    await user.click(await screen.findByRole("button", { name: "Activar 2026-2027" }));

    expect(onActivate).toHaveBeenCalledWith("c2");
  });

  // Activating the year that is already live is the click of somebody who is not
  // sure. Offering it would invite a no-op; hiding it answers the question.
  it("does not offer it for the year already in service", () => {
    render(
      <CycleBar cycles={cycles} selectedCycleId="c1" onSelect={() => {}} onCreate={vi.fn()} onActivate={vi.fn()} />,
    );

    expect(screen.queryByRole("button", { name: /Activar ciclo/ })).toBeNull();
    expect(screen.getByText(/es el ciclo en curso/i)).toBeInTheDocument();
  });

  it("says so when the school is not allowed to change the live year", async () => {
    const user = userEvent.setup();
    const onActivate = vi.fn().mockRejectedValue(new ConfigError(403, "forbidden"));
    render(
      <CycleBar cycles={cycles} selectedCycleId="c2" onSelect={() => {}} onCreate={vi.fn()} onActivate={onActivate} />,
    );

    await user.click(screen.getByRole("button", { name: /Activar ciclo/ }));
    // The live 2025-2026 is paused, so it asks first (QA CC-102).
    await user.click(await screen.findByRole("button", { name: "Activar 2026-2027" }));

    expect(await screen.findByRole("alert")).toHaveTextContent(/permiso/i);
  });
});

describe("la captura del ciclo", () => {
  it("el nombre del ciclo se captura en mayúsculas y es obligatorio", async () => {
    const user = userEvent.setup();
    render(<CycleBar cycles={[]} selectedCycleId={undefined} onSelect={vi.fn()} onCreate={vi.fn()} onActivate={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: /Nuevo ciclo/ }));

    const nombre = screen.getByLabelText("Nombre del ciclo");
    await user.clear(nombre);
    await user.type(nombre, "2026-2027 anual");
    expect(nombre).toHaveValue("2026-2027 ANUAL");
    expect(nombre).toHaveAttribute("aria-required", "true");
  });

  it("abre con el ciclo escolar propuesto: nombre y fechas de agosto a julio (ISSUE-T14-023)", async () => {
    const onCreate = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(<CycleBar cycles={[]} selectedCycleId={undefined} onSelect={vi.fn()} onCreate={onCreate} onActivate={vi.fn()} today="2026-10-17" />);
    await user.click(screen.getByRole("button", { name: /Nuevo ciclo/ }));

    expect(screen.getByLabelText("Nombre del ciclo")).toHaveValue("2026-2027");
    expect(screen.getByLabelText("Inicia")).toHaveValue("2026-08-01");
    expect(screen.getByLabelText("Termina")).toHaveValue("2027-07-31");
    await user.click(screen.getByRole("button", { name: /Crear ciclo/ }));
    expect(onCreate).toHaveBeenCalledWith({ name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-07-31" });
  });

  it("guardar en blanco marca los tres campos en rojo y no llama al API", async () => {
    const onCreate = vi.fn();
    const user = userEvent.setup();
    render(<CycleBar cycles={[]} selectedCycleId={undefined} onSelect={vi.fn()} onCreate={onCreate} onActivate={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: /Nuevo ciclo/ }));
    await user.clear(screen.getByLabelText("Nombre del ciclo"));
    await user.clear(screen.getByLabelText("Inicia"));
    await user.clear(screen.getByLabelText("Termina"));
    await user.click(screen.getByRole("button", { name: /Crear ciclo/ }));

    expect(screen.getByLabelText("Nombre del ciclo")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Inicia")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Termina")).toHaveAttribute("aria-invalid", "true");
    expect(onCreate).not.toHaveBeenCalled();
  });
});
