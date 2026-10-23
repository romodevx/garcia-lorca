"use client";

import { useCallback, useEffect, useRef, type ReactElement, type ReactNode } from "react";

import { Btn } from "./Controls";
import { DrawerShell } from "./DrawerShell";
import type { IconName } from "./Icon";

export interface ConfirmDialogProps {
  title: string;
  confirmLabel: string;
  icon?: IconName;
  onConfirm: () => void;
  onCancel: () => void;
  /** It cuts or removes something (an app access, a family): the red tone, no check. */
  danger?: boolean;
  children: ReactNode;
}

/**
 * "¿Seguro?" — only for what does not undo (ui-forms-and-modals: everything
 * else gets an undo toast). DrawerShell gives the APG contract: modal, focus
 * trapped, Esc cancels, focus returns to the trigger. Here the confirm button
 * takes the first focus so Enter confirms, as the person just asked for it.
 */
export function ConfirmDialog({ title, confirmLabel, icon = "warning", onConfirm, onCancel, danger = false, children }: ConfirmDialogProps): ReactElement {
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);

  // The footer exists only once DrawerShell's portal has mounted (a render
  // AFTER this component's first commit), and DrawerShell then focuses its
  // first control (the close button) in that same commit. So the confirm
  // button is focused from the footer's own ref, on the next task — after
  // DrawerShell's focus — never from a mount effect, which ran before the
  // footer existed and left Enter on "Cerrar" (QA T17).
  const footerRef = useCallback((node: HTMLDivElement | null) => {
    if (node === null) return;
    clearTimeout(timer.current);
    timer.current = setTimeout(() => node.querySelector<HTMLButtonElement>('button[type="submit"]')?.focus(), 0);
  }, []);

  return (
    <DrawerShell title={title} icon={icon} size="sm" onClose={onCancel} onSubmit={onConfirm}
      footer={
        <div ref={footerRef} style={{ display: "flex", alignItems: "center", gap: 12, width: "100%" }}>
          <Btn kind="ghost" onClick={onCancel}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          <Btn type="submit" icon={danger ? undefined : "check"} danger={danger} style={{ height: 52 }}>{confirmLabel}</Btn>
        </div>
      }>
      <div style={{ fontSize: 15, lineHeight: 1.55, color: "var(--text)" }}>{children}</div>
    </DrawerShell>
  );
}
