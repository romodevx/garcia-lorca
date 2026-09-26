"use client";

import { useRef, useState, type ReactElement } from "react";
import { Btn } from "./Controls";

export interface InviteLinkProps {
  /** The activation link the office hands over itself. */
  url: string;
}

/**
 * An activation link nobody mailed, ready to hand over: read-only (it is
 * selected, never edited) with a one-press "Copiar liga". Shared by every
 * surface that invites someone — a guardian from Contactos, a teacher from
 * Docentes — so the office learns one gesture.
 *
 * When the browser refuses the clipboard (an http origin, a denied
 * permission), the link is selected instead, so Ctrl+C still does the job.
 */
export function InviteLink({ url }: InviteLinkProps): ReactElement {
  const field = useRef<HTMLInputElement>(null);
  const [copied, setCopied] = useState(false);

  const copy = async (): Promise<void> => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
    } catch {
      field.current?.focus();
      field.current?.select();
    }
  };

  return (
    <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
      <input
        ref={field}
        readOnly
        value={url}
        aria-label="Liga de acceso"
        onFocus={(e) => e.currentTarget.select()}
        style={{ flex: "1 1 220px", minWidth: 0, minHeight: 44, padding: "8px 10px", borderRadius: 8, border: "1px solid var(--border)", background: "var(--surface)", color: "var(--text)", fontSize: 12.5, fontFamily: "monospace" }}
      />
      <Btn kind="ghost" icon={copied ? "check" : "documents"} onClick={() => void copy()} style={{ minHeight: 44 }}>
        {copied ? "Liga copiada" : "Copiar liga"}
      </Btn>
    </div>
  );
}
