import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { InviteLink } from "./InviteLink";

const URL = "http://localhost:3002/activar?token=abc";

describe("InviteLink", () => {
  it("shows the link read-only, so it can be selected but not edited", () => {
    render(<InviteLink url={URL} />);

    const field = screen.getByLabelText("Liga de acceso");
    expect(field).toHaveValue(URL);
    expect(field).toHaveAttribute("readonly");
  });

  it("copies the link with one press and says so", async () => {
    const user = userEvent.setup();
    const writeText = vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    render(<InviteLink url={URL} />);

    await user.click(screen.getByRole("button", { name: "Copiar liga" }));

    expect(writeText).toHaveBeenCalledWith(URL);
    expect(await screen.findByRole("button", { name: "Liga copiada" })).toBeInTheDocument();
  });

  it("when the browser refuses the clipboard, selects the link so Ctrl+C still works", async () => {
    const user = userEvent.setup();
    vi.spyOn(navigator.clipboard, "writeText").mockRejectedValue(new Error("denied"));
    render(<InviteLink url={URL} />);

    await user.click(screen.getByRole("button", { name: "Copiar liga" }));

    const field = screen.getByLabelText<HTMLInputElement>("Liga de acceso");
    expect(field).toHaveFocus();
    expect(field.selectionStart).toBe(0);
    expect(field.selectionEnd).toBe(URL.length);
    expect(screen.getByRole("button", { name: "Copiar liga" })).toBeInTheDocument();
  });
});
