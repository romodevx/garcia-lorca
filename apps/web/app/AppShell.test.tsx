import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { AppShell } from "./AppShell";
import { Providers } from "./test-utils";

describe("AppShell navigation flow", () => {
  it("goes login → dashboard → finance → back, then logs out", async () => {
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    // Login.
    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // Open Finanzas (KPI shortcut) — finance flag is ON, so the real surface shows.
    await user.click(screen.getByRole("button", { name: /Adeudos del mes/ }));
    expect(await screen.findByRole("heading", { name: "Finanzas y Adeudos" })).toBeInTheDocument();

    // Back home.
    await user.click(screen.getByRole("button", { name: /Volver al inicio/ }));
    expect(await screen.findByRole("heading", { name: /Módulos/ })).toBeInTheDocument();

    // Logout → back to login.
    await user.click(screen.getByRole("button", { name: "Cerrar sesión" }));
    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
  });

  it("falls back to the placeholder when module_finance is OFF (UX gating)", async () => {
    const user = userEvent.setup();
    render(
      <Providers flags={{}}>
        <AppShell />
      </Providers>
    );

    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));
    await user.click(screen.getByRole("button", { name: /Adeudos del mes/ }));

    // Flag OFF → friendly stub, not the real finance table. The placeholder
    // shows the "Módulo «...»" heading; the real surface shows the status tablist.
    expect(await screen.findByRole("heading", { name: /Módulo «Finanzas y Adeudos»/ })).toBeInTheDocument();
    expect(screen.queryByRole("tablist", { name: "Filtrar por estado" })).not.toBeInTheDocument();
  });

  it("shows the teacher identity in the top bar when logging in as Maestro", async () => {
    const user = userEvent.setup();
    render(
      <Providers flags={{ module_finance: true }}>
        <AppShell />
      </Providers>
    );

    await user.click(screen.getByRole("button", { name: "Maestro" }));
    await user.click(screen.getByRole("button", { name: "Entrar al sistema" }));

    expect(await screen.findByText("Miguel Vázquez")).toBeInTheDocument();
  });
});
