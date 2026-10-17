import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { NoModules } from "./HomeNotices";

/*
 * QA T14 §2b: a guardian signing in to the web read "No tienes módulos
 * asignados · Solicita acceso a tu administrador" — true, but it sends them to
 * ask for access they will never get. Their access is the family app.
 */
describe("NoModules", () => {
  it("tells a family-only user that their access is the family app", () => {
    render(<NoModules permissions={["family:read"]} />);
    expect(screen.getByText("Tu acceso es desde la app de García Lorca para familias.")).toBeInTheDocument();
    expect(screen.queryByText(/Solicita acceso/)).toBeNull();
  });

  // ISSUE-T14-301: the seeded guardian role also owns its uniform orders
  // (migrations/iam/0014), so a real guardian is not "family:*" only.
  it("recognises the real guardian role, which also orders its own uniforms", () => {
    render(<NoModules permissions={["family:read", "uniforms_own:read", "uniforms_own:write"]} />);
    expect(screen.getByText("Tu acceso es desde la app de García Lorca para familias.")).toBeInTheDocument();
  });

  it.each([
    ["no permissions at all", []],
    ["staff permissions that match no module", ["reports:read"]],
    ["a guardian who is also staff", ["family:read", "reports:read"]],
  ])("keeps the ask-your-administrator notice for %s", (_case, permissions) => {
    render(<NoModules permissions={permissions} />);
    expect(screen.getByText("No tienes módulos asignados")).toBeInTheDocument();
    expect(screen.getByText(/Solicita acceso a tu administrador/)).toBeInTheDocument();
  });
});
