"use client";

import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactElement } from "react";

import { Avatar } from "./Brand";
import type { UserProfile } from "./data";

export interface AccountMenuProps {
  user: UserProfile;
  onChangePassword: () => void;
  onLogout: () => void;
}

interface MenuAction {
  label: string;
  run: () => void;
}

/**
 * The name chip in the top bar and the account menu it opens (WAI-ARIA APG
 * menu button). It used to BE "Cerrar sesión": one click on your own name
 * logged you out without asking (ISSUE-T14-103).
 *
 * Every style lives in globals.css (.topbar-user*, .account-menu*), like the
 * rest of the bar: an inline style would beat the look layer.
 */
export function AccountMenu({ user, onChangePassword, onLogout }: AccountMenuProps): ReactElement {
  const [open, setOpen] = useState(false);
  const menuId = useId();
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([]);

  const actions: MenuAction[] = [
    { label: "Cambiar contraseña", run: onChangePassword },
    { label: "Cerrar sesión", run: onLogout },
  ];

  const close = (returnFocus: boolean): void => {
    setOpen(false);
    if (returnFocus) triggerRef.current?.focus();
  };

  useEffect(() => {
    if (!open) return;
    itemRefs.current[0]?.focus();
    // `mousedown` on the document, not a capture-phase click handler on the
    // page: the item being clicked must still receive its own click.
    const onOutside = (e: MouseEvent): void => {
      if (rootRef.current?.contains(e.target as Node) !== true) setOpen(false);
    };
    document.addEventListener("mousedown", onOutside);
    return () => document.removeEventListener("mousedown", onOutside);
  }, [open]);

  const focusItem = (index: number): void => {
    const count = actions.length;
    itemRefs.current[(index + count) % count]?.focus();
  };

  const onMenuKey = (e: KeyboardEvent<HTMLDivElement>): void => {
    const current = itemRefs.current.indexOf(document.activeElement as HTMLButtonElement);
    const moves: Record<string, () => void> = {
      ArrowDown: () => focusItem(current + 1),
      ArrowUp: () => focusItem(current - 1),
      Home: () => focusItem(0),
      End: () => focusItem(actions.length - 1),
      Escape: () => close(true),
      Tab: () => close(false),
    };
    const move = moves[e.key];
    if (move === undefined) return;
    if (e.key !== "Tab") e.preventDefault();
    move();
  };

  const choose = (action: MenuAction): void => {
    // Focus goes back to the chip FIRST, so a dialog opened by the action
    // records the chip as the place to return to when it closes.
    close(true);
    action.run();
  };

  return (
    <div ref={rootRef} className="topbar-account">
      <button
        ref={triggerRef}
        type="button"
        aria-label={`Mi cuenta, ${user.name}`}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        onClick={() => setOpen((v) => !v)}
        className="topbar-user"
      >
        <div className="topbar-user-text">
          <div className="topbar-user-name">{user.name}</div>
          <div className="topbar-user-role">{user.role}</div>
        </div>
        <Avatar name={user.name} color={user.color} size={40} />
      </button>
      {open && (
        <div className="account-menu" onKeyDown={onMenuKey}>
          <div className="account-menu-who">
            <div className="account-menu-name">{user.name}</div>
            <div className="account-menu-role">{user.role}</div>
          </div>
          <div id={menuId} role="menu" aria-label="Mi cuenta" className="account-menu-list">
            {actions.map((action, index) => (
              <button
                key={action.label}
                ref={(node) => { itemRefs.current[index] = node; }}
                type="button"
                role="menuitem"
                tabIndex={-1}
                className="account-menu-item"
                onClick={() => choose(action)}
              >
                {action.label}
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
