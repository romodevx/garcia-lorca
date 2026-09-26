import { useCallback, useEffect, useRef, useState } from "react";

import type { Teacher, TeacherAccessResult } from "../../teachers/api";
import { describeAccessError } from "../../teachers/accessErrors";

const TOAST_MS = 4000;

/** "none": no login; "linked": a login whose state this page has not asked about yet. */
export type TeacherAccessState = "none" | "linked" | TeacherAccessResult["status"];

export interface TeacherAccess {
  stateOf: (teacher: Teacher) => TeacherAccessState;
  grant: (teacher: Teacher) => Promise<void>;
  grantingId: string | undefined;
  errorOf: (teacherId: string) => string | undefined;
  inviteUrlOf: (teacherId: string) => string | undefined;
  toast: string | undefined;
}

/**
 * The "Dar acceso al sistema" flow of the roster, one teacher at a time: what
 * each row's login looks like, the press itself (never sent twice while in
 * flight), the confirmation and the per-row failure. The server call is the
 * caller's, so the screen can relink the roster row it owns.
 */
export function useTeacherAccess(onGrantAccess?: (teacherId: string) => Promise<TeacherAccessResult>): TeacherAccess {
  const [outcomes, setOutcomes] = useState<Record<string, TeacherAccessResult>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [grantingId, setGrantingId] = useState<string | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  const inFlight = useRef(false);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(toastTimer.current), []);

  const announce = useCallback((message: string): void => {
    clearTimeout(toastTimer.current);
    setToast(message);
    toastTimer.current = setTimeout(() => setToast(undefined), TOAST_MS);
  }, []);

  const grant = useCallback(
    async (teacher: Teacher): Promise<void> => {
      if (onGrantAccess === undefined || inFlight.current) return;
      inFlight.current = true;
      setGrantingId(teacher.id);
      setErrors(({ [teacher.id]: _dropped, ...rest }) => rest);
      try {
        const result = await onGrantAccess(teacher.id);
        setOutcomes((prev) => ({ ...prev, [teacher.id]: result }));
        announce(
          result.status === "active"
            ? `${teacher.firstName} ${teacher.lastName} ya entra al sistema con su contraseña.`
            : `Enviamos la invitación a ${result.email}`,
        );
      } catch (cause) {
        setErrors((prev) => ({ ...prev, [teacher.id]: describeAccessError(cause) }));
      } finally {
        inFlight.current = false;
        setGrantingId(undefined);
      }
    },
    [onGrantAccess, announce],
  );

  const stateOf = useCallback(
    (teacher: Teacher): TeacherAccessState => outcomes[teacher.id]?.status ?? (teacher.userId !== "" ? "linked" : "none"),
    [outcomes],
  );

  return {
    stateOf,
    grant,
    grantingId,
    errorOf: (id) => errors[id],
    inviteUrlOf: (id) => outcomes[id]?.inviteUrl,
    toast,
  };
}
