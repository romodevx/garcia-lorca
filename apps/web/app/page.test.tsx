import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AppShell } from "./AppShell";
import { Providers } from "./test-utils";

// AppShell is the web entry surface rendered by HomePage. On boot it shows the
// login screen; this keeps a fast smoke test that the shell mounts.
describe("AppShell (HomePage surface)", () => {
  it("renders the login screen on boot", () => {
    render(
      <Providers>
        <AppShell />
      </Providers>
    );
    expect(screen.getByRole("button", { name: "Entrar al sistema" })).toBeInTheDocument();
    expect(screen.getByText("Todo el colegio,", { exact: false })).toBeInTheDocument();
  });
});
