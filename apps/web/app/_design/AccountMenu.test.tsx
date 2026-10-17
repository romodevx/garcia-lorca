import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { AccountMenu } from "./AccountMenu";
import { USERS } from "./data";

/*
 * ISSUE-T14-103: the name chip WAS "Cerrar sesión" — one click on your own name
 * logged you out without asking. It now opens a small account menu.
 */

function renderMenu() {
  const onChangePassword = vi.fn();
  const onLogout = vi.fn();
  render(<AccountMenu user={USERS.admin} onChangePassword={onChangePassword} onLogout={onLogout} />);
  return { onChangePassword, onLogout, trigger: screen.getByRole("button", { name: /Mi cuenta/ }) };
}

describe("AccountMenu", () => {
  it("a click on the name opens a menu instead of logging out", async () => {
    const user = userEvent.setup();
    const { trigger, onLogout } = renderMenu();
    expect(trigger).toHaveAttribute("aria-haspopup", "menu");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveAccessibleName(new RegExp(USERS.admin.name));

    await user.click(trigger);
    expect(onLogout).not.toHaveBeenCalled();
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    const menu = screen.getByRole("menu");
    expect(menu).toBeInTheDocument();
    expect(screen.getAllByRole("menuitem").map((i) => i.textContent)).toEqual(["Cambiar contraseña", "Cerrar sesión"]);
    expect(screen.getByRole("menuitem", { name: "Cambiar contraseña" })).toHaveFocus();
  });

  it("moves with the arrows and closes on Esc, returning focus to the chip", async () => {
    const user = userEvent.setup();
    const { trigger } = renderMenu();
    await user.click(trigger);
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Cerrar sesión" })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("menuitem", { name: "Cambiar contraseña" })).toHaveFocus();
    await user.keyboard("{End}");
    expect(screen.getByRole("menuitem", { name: "Cerrar sesión" })).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("menu")).toBeNull();
    expect(trigger).toHaveFocus();
  });

  it("closes on a click outside", async () => {
    const user = userEvent.setup();
    const { trigger } = renderMenu();
    await user.click(trigger);
    await user.click(document.body);
    expect(screen.queryByRole("menu")).toBeNull();
  });

  it("each item does its job and closes the menu, focus back on the chip", async () => {
    const user = userEvent.setup();
    const { trigger, onChangePassword, onLogout } = renderMenu();
    await user.click(trigger);
    await user.click(screen.getByRole("menuitem", { name: "Cambiar contraseña" }));
    expect(onChangePassword).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole("menu")).toBeNull();
    expect(trigger).toHaveFocus();

    await user.click(trigger);
    await user.click(screen.getByRole("menuitem", { name: "Cerrar sesión" }));
    expect(onLogout).toHaveBeenCalledTimes(1);
  });
});
