import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as commsApi from "../../comms/api";
import { BirthdayGreeting, renderGreeting } from "./BirthdayGreeting";

vi.mock("../../comms/api", () => ({ getBirthdayTemplate: vi.fn(), updateBirthdayTemplate: vi.fn() }));
const comms = vi.mocked(commsApi);

beforeEach(() => {
  comms.getBirthdayTemplate.mockResolvedValue({
    title: "¡Feliz cumpleaños, {nombre}!",
    body: "Toda la comunidad le desea a {nombre} un feliz día.",
    isDefault: true,
    updatedAt: "",
  });
  comms.updateBirthdayTemplate.mockImplementation(async (_b, input) => ({ ...input, isDefault: false, updatedAt: "2026-10-01T13:00:00Z" }));
});
afterEach(() => vi.clearAllMocks());

describe("renderGreeting", () => {
  it("puts the name wherever {nombre} appears", () => {
    expect(renderGreeting("{nombre}, hoy es tu día, {nombre}", "Ana")).toBe("Ana, hoy es tu día, Ana");
  });
});

describe("BirthdayGreeting", () => {
  it("shows the school's text, says when it is the default, and previews it", async () => {
    render(<BirthdayGreeting />);

    expect(await screen.findByDisplayValue("¡Feliz cumpleaños, {nombre}!")).toBeInTheDocument();
    expect(screen.getByText(/texto predeterminado/i)).toBeInTheDocument();
    expect(screen.getByText("Toda la comunidad le desea a Ana un feliz día.")).toBeInTheDocument();
  });

  it("saves the edited text", async () => {
    const user = userEvent.setup();
    render(<BirthdayGreeting />);
    const message = await screen.findByLabelText("Mensaje de cumpleaños");

    await user.clear(message);
    await user.type(message, "Felicidades, {{nombre}!");
    await user.click(screen.getByRole("button", { name: /guardar felicitación/i }));

    await waitFor(() =>
      expect(comms.updateBirthdayTemplate).toHaveBeenCalledWith(expect.any(String), {
        title: "¡Feliz cumpleaños, {nombre}!",
        body: "Felicidades, {nombre}!",
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(/guardada/i);
  });

  // Never a dead button: an empty message is refused on press, in its field.
  it("refuses an empty message on save and keeps the button alive", async () => {
    const user = userEvent.setup();
    render(<BirthdayGreeting />);
    const message = await screen.findByLabelText("Mensaje de cumpleaños");

    await user.clear(message);
    const save = screen.getByRole("button", { name: /guardar felicitación/i });
    expect(save).toBeEnabled();
    await user.click(save);

    expect(await screen.findByText(/escribe el mensaje/i)).toBeInTheDocument();
    expect(comms.updateBirthdayTemplate).not.toHaveBeenCalled();
  });

  it("refuses a message over 500 characters", async () => {
    const user = userEvent.setup();
    render(<BirthdayGreeting />);
    const message = await screen.findByLabelText("Mensaje de cumpleaños");

    await user.clear(message);
    await user.click(message);
    await user.paste("a".repeat(501));
    await user.click(screen.getByRole("button", { name: /guardar felicitación/i }));

    expect(await screen.findByText(/como máximo 500 caracteres/i)).toBeInTheDocument();
    expect(comms.updateBirthdayTemplate).not.toHaveBeenCalled();
  });
});
