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

  // Review: nothing to edit until the current text has arrived, so a slow
  // answer cannot overwrite what the director typed, and a failed one cannot
  // let them replace a text they never saw.
  it("shows no fields until the current text arrives, and none if it never does", async () => {
    comms.getBirthdayTemplate.mockRejectedValueOnce(new Error("down"));
    render(<BirthdayGreeting />);
    expect(await screen.findByRole("alert")).toHaveTextContent(/no pudimos leer/i);
    expect(screen.queryByLabelText("Mensaje de cumpleaños")).toBeNull();
    expect(screen.queryByRole("button", { name: /guardar felicitación/i })).toBeNull();
  });

  // ui-forms-and-modals: validate on blur, not only on save.
  it("says what is wrong when leaving an emptied field", async () => {
    const user = userEvent.setup();
    render(<BirthdayGreeting />);
    const title = await screen.findByLabelText("Título de la felicitación");
    await user.clear(title);
    await user.tab();
    expect(await screen.findByText(/escribe el título/i)).toBeInTheDocument();
  });

  // ui-forms-and-modals: the primary button is never disabled; a second press
  // while saving is simply ignored.
  it("never disables the save button, and saves once on a double press", async () => {
    const user = userEvent.setup();
    let release: () => void = () => {};
    comms.updateBirthdayTemplate.mockImplementationOnce(
      (_b, input) => new Promise((resolve) => { release = () => resolve({ ...input, isDefault: false, updatedAt: "" }); }),
    );
    render(<BirthdayGreeting />);
    await screen.findByLabelText("Mensaje de cumpleaños");
    const save = screen.getByRole("button", { name: /guardar felicitación/i });
    await user.click(save);
    expect(screen.getByRole("button", { name: /guardando/i })).toBeEnabled();
    await user.click(screen.getByRole("button", { name: /guardando/i }));
    release();
    await screen.findByRole("status");
    expect(comms.updateBirthdayTemplate).toHaveBeenCalledTimes(1);
  });
});
