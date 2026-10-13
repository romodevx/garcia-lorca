import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as commsApi from "../../comms/api";
import { AutomaticMessages } from "./AutomaticMessages";

vi.mock("../../comms/api", () => ({ getBirthdayTemplate: vi.fn(), updateBirthdayTemplate: vi.fn() }));
const comms = vi.mocked(commsApi);

beforeEach(() => {
  comms.getBirthdayTemplate.mockResolvedValue({ title: "¡Feliz cumpleaños, {nombre}!", body: "Hola {nombre}", isDefault: true, updatedAt: "" });
});
afterEach(() => vi.clearAllMocks());

describe("AutomaticMessages", () => {
  it("is a folded panel, closed by default, whose summary names the greeting", () => {
    const { container } = render(<AutomaticMessages />);
    const panel = container.querySelector("details");
    expect(panel).not.toBeNull();
    expect(panel?.open).toBe(false);
    const summary = panel?.querySelector("summary");
    expect(summary).toHaveTextContent("Felicitación de cumpleaños");
    expect(summary).toHaveTextContent("Mensaje automático · 7:00");
    expect(summary?.querySelector("button")).toBeNull();
  });

  // A5 still loads on mount: a closed <details> does not unmount its content.
  it("mounts the greeting at once, folded inside", async () => {
    const { container } = render(<AutomaticMessages />);
    expect(await screen.findByDisplayValue("¡Feliz cumpleaños, {nombre}!")).toBeInTheDocument();
    expect(comms.getBirthdayTemplate).toHaveBeenCalledTimes(1);
    expect(container.querySelector("details")).toContainElement(screen.getByRole("form", { name: "Felicitación de cumpleaños" }));
    expect(screen.getAllByRole("heading", { name: "Felicitación de cumpleaños" })).toHaveLength(1);
  });
});
