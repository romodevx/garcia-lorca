import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import HomePage from "./page";
import { Providers } from "../test-utils";

/**
 * El catch-all no mira el slug: quien decide qué pintar es <AppShell> al leer
 * la dirección (app/routing/routes.ts). Lo que esta suite defiende es que la
 * ruta EXISTE para cualquier pantalla del shell — antes de septiembre de 2026
 * sólo existía "/", así que /finanzas era un 404 y F5 devolvía al tablero.
 */
describe("la cáscara de la web", () => {
  it("monta el shell, que arranca en la pantalla de entrada", () => {
    render(
      <Providers>
        <HomePage />
      </Providers>
    );
    expect(screen.getByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(screen.getByText("Todo el colegio,", { exact: false })).toBeInTheDocument();
  });
});
