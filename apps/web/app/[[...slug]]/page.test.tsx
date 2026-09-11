import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import HomePage from "./page";
import { Providers, fakeRouter, resetFakeRouter, useFakePathname } from "../test-utils";

vi.mock("next/navigation", () => ({
  useRouter: () => fakeRouter,
  usePathname: () => useFakePathname(),
}));

vi.mock("../auth/api", () => ({
  login: vi.fn(),
  changePassword: vi.fn(),
  logout: vi.fn(),
  refresh: vi.fn(() => Promise.reject(new Error("no session"))),
  me: vi.fn(() => Promise.reject(new Error("no session"))),
}));

/**
 * El catch-all no mira el slug: quien decide qué pintar es <AppShell> al leer
 * la dirección (app/routing/routes.ts). Lo que esta suite defiende es que la
 * ruta EXISTE para cualquier pantalla del shell — antes de septiembre de 2026
 * sólo existía "/", así que /finanzas era un 404 y F5 devolvía al tablero.
 */
describe("la cáscara de la web", () => {
  beforeEach(() => resetFakeRouter("/"));

  it("monta el shell, que sin sesión aterriza en la pantalla de entrada", async () => {
    render(
      <Providers>
        <HomePage />
      </Providers>
    );

    // Asíncrono a propósito: primero se pregunta si hay sesión. Pintar la
    // entrada de inmediato la haría parpadear a quien recarga con sesión viva.
    expect(await screen.findByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(screen.getByText("Todo el colegio,", { exact: false })).toBeInTheDocument();
  });
});
