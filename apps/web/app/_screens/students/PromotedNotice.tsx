"use client";

import { useEffect, useRef, useState, type ReactElement } from "react";

import { API_URL } from "../../api-url";
import { Btn } from "../../_design/Controls";
import { listCycles, listLabelledGroups } from "../../config/api";
import { enroll, StudentsError } from "../../students/api";

const ENROLL_FAILED = "No se pudo inscribir. Intenta de nuevo desde su ficha.";

type EnrollState = { kind: "idle" } | { kind: "saving" } | { kind: "done" } | { kind: "failed"; message: string };

/** The classroom's label ("2° PRIMARIA · A"), read from the active cycle; undefined while unknown. */
function useGroupLabel(groupId: string | undefined): string | undefined {
  const [label, setLabel] = useState<string | undefined>(undefined);
  useEffect(() => {
    if (groupId === undefined) return;
    let live = true;
    listCycles(API_URL)
      .then((cycles) => {
        const active = cycles.find((c) => c.status === "active") ?? cycles[0];
        return active === undefined ? [] : listLabelledGroups(API_URL, active.id);
      })
      .then((groups) => live && setLabel(groups.find((g) => g.id === groupId)?.label))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [groupId]);
  return label;
}

/**
 * A draft just got its CURP (module_import_review): it is a pupil like any
 * other now, and the import already said where it goes, so the enrollment is
 * one click away. Without that group, the sheet's «Inscribir» is the way.
 */
export function PromotedNotice({ studentId, groupId }: { studentId: string; groupId: string | undefined }): ReactElement {
  const label = useGroupLabel(groupId);
  const [state, setState] = useState<EnrollState>({ kind: "idle" });
  const inFlight = useRef(false);
  const room = label ?? "el grupo de la importación";

  const enrollThere = async (): Promise<void> => {
    if (groupId === undefined || inFlight.current) return;
    inFlight.current = true;
    setState({ kind: "saving" });
    try {
      await enroll(API_URL, { studentId, groupId });
      setState({ kind: "done" });
    } catch (err) {
      setState({ kind: "failed", message: err instanceof StudentsError ? err.message : ENROLL_FAILED });
    } finally {
      inFlight.current = false;
    }
  };

  return (
    <section aria-label="Ya no es borrador" style={{ display: "grid", gap: 10, padding: "14px 16px", borderRadius: "var(--r-md)", background: "var(--ok-bg)", color: "var(--ok-fg)" }}>
      <p role="status" style={{ margin: 0, fontWeight: 700, fontSize: 14.5 }}>
        {state.kind === "done" ? `Inscrito en ${room}.` : "Ya tiene CURP: dejó de ser borrador y ya se puede inscribir."}
      </p>
      {groupId === undefined && <p style={{ margin: 0, fontSize: 14 }}>Inscríbelo desde su ficha, con «Inscribir».</p>}
      {groupId !== undefined && state.kind !== "done" && (
        <div>
          <Btn icon="check" busy={state.kind === "saving"} onClick={() => void enrollThere()} style={{ height: 44, fontSize: 14.5 }}>{`Inscribir en ${room}`}</Btn>
        </div>
      )}
      {state.kind === "failed" && <p role="alert" style={{ margin: 0, color: "var(--bad-fg)", fontSize: 14, fontWeight: 600 }}>{state.message}</p>}
    </section>
  );
}
