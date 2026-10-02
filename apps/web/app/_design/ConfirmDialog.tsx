"use client";

import { useEffect, useRef, type ReactElement, type ReactNode } from "react";

import { Btn } from "./Controls";
import { DrawerShell } from "./DrawerShell";
import type { IconName } from "./Icon";

export interface ConfirmDialogProps {
  title: string;
  confirmLabel: string;
  icon?: IconName;
  onConfirm: () => void;
  onCancel: () => void;
  children: ReactNode;
}

/**
 * "¿Seguro?" — only for what does not undo (ui-forms-and-modals: everything
 * else gets an undo toast). DrawerShell gives the APG contract: modal, focus
 * trapped, Esc cancels, focus returns to the trigger. Here the confirm button
 * takes the first focus so Enter confirms, as the person just asked for it.
 */
export function ConfirmDialog({ title, confirmLabel, icon = "warning", onConfirm, onCancel, children }: ConfirmDialogProps): ReactElement {
  const footerRef = useRef<HTMLDivElement>(null);

  // After DrawerShell's own first focus (it lands on its first control), which
  // runs in the same commit, after this effect: hence the next task.
  useEffect(() => {
    const t = setTimeout(() => footerRef.current?.querySelector<HTMLButtonElement>('button[type="submit"]')?.focus(), 0);
    return () => clearTimeout(t);
  }, []);

  return (
    <DrawerShell title={title} icon={icon} size="sm" onClose={onCancel} onSubmit={onConfirm}
      footer={
        <div ref={footerRef} style={{ display: "flex", alignItems: "center", gap: 12, width: "100%" }}>
          <Btn kind="ghost" onClick={onCancel}>Cancelar</Btn>
          <div style={{ flex: 1 }} />
          <Btn type="submit" icon="check" style={{ height: 52 }}>{confirmLabel}</Btn>
        </div>
      }>
      <div style={{ fontSize: 15, lineHeight: 1.55, color: "var(--text)" }}>{children}</div>
    </DrawerShell>
  );
}
