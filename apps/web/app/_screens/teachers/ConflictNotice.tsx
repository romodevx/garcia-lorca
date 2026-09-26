"use client";

import { useId, useState, type ReactElement } from "react";

import { Btn } from "../../_design/Controls";

/** A requested slot another teacher holds, named for people. */
export interface NamedConflict {
  key: string;
  subjectName: string;
  groupLabel: string;
  teacherName: string;
}

export interface ConflictNoticeProps {
  conflicts: readonly NamedConflict[];
  onReplace: () => Promise<void>;
  onKeep: () => void;
}

/**
 * One teacher per group and subject: a slot someone else holds is never taken
 * silently. The notice names who holds each one and asks — Reemplazar hands
 * over exactly those slots, "Dejar como está" leaves them where they were.
 */
export function ConflictNotice({ conflicts, onReplace, onKeep }: ConflictNoticeProps): ReactElement {
  const titleId = useId();
  const [pending, setPending] = useState(false);
  const count = conflicts.length;
  const title = count === 1 ? "1 grupo ya tiene docente" : `${count} grupos ya tienen docente`;

  const replace = async (): Promise<void> => {
    if (pending) return;
    setPending(true);
    try {
      await onReplace();
    } finally {
      setPending(false);
    }
  };

  return (
    <section
      aria-labelledby={titleId}
      style={{ marginTop: 14, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--warn-bg)", color: "var(--warn-fg)" }}
    >
      <h3 id={titleId} style={{ fontSize: 15, fontWeight: 700 }}>
        {title}
      </h3>
      <ul style={{ margin: "8px 0 12px", paddingLeft: 18, fontSize: 14, lineHeight: 1.5 }}>
        {conflicts.map((c) => (
          <li key={c.key}>
            {c.subjectName} · {c.groupLabel} — La imparte {c.teacherName}
          </li>
        ))}
      </ul>
      <div style={{ display: "flex", flexWrap: "wrap", gap: 10, alignItems: "center" }}>
        <Btn type="button" kind="forest" onClick={() => void replace()}>
          Reemplazar
        </Btn>
        <button type="button" onClick={onKeep} style={{ minHeight: 44, padding: "0 10px", fontWeight: 600, fontSize: 14.5, color: "inherit", textDecoration: "underline", cursor: "pointer" }}>
          Dejar como está
        </button>
      </div>
    </section>
  );
}
