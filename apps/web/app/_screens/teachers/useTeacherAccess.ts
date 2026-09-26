import { useCallback, useEffect, useRef, useState } from "react";

import type { UserStatus } from "../../admin/api";
import {
  ReuseConfirmationRequired,
  type GrantTeacherAccessOptions,
  type Teacher,
  type TeacherAccessResult,
} from "../../teachers/api";
import { describeAccessError } from "../../teachers/accessErrors";

const TOAST_MS = 4000;

/**
 * "none": no login; "linked": a login whose state this page cannot read (the
 * principal cannot list users) — nothing is claimed, but a resend is safe: the
 * server answers with the account's real state; "unrecognised": iam answered a
 * status this build does not know — nothing is claimed and nothing is offered;
 * "invited": waiting for its password, no mail known to have left;
 * "invitationSent": invited, and this page saw the mail go out.
 */
export type TeacherAccessState = "none" | "linked" | "unrecognised" | "invitationSent" | TeacherAccessResult["status"];

/** The server call: the plain press, or the resend after the user confirmed a reuse. */
export type GrantAccessCall = (teacherId: string, options?: GrantTeacherAccessOptions) => Promise<TeacherAccessResult>;

export interface TeacherAccess {
  stateOf: (teacher: Teacher) => TeacherAccessState;
  grant: (teacher: Teacher) => Promise<void>;
  /** Resend after the user agreed to link the existing account. */
  confirmReuse: (teacher: Teacher) => Promise<void>;
  /** Drop the question without sending anything. */
  cancelReuse: (teacherId: string) => void;
  /** The server's question when an existing account needs the user's say-so. */
  confirmationOf: (teacherId: string) => string | undefined;
  /** Whether this teacher's press is still waiting for the server. */
  isGranting: (teacherId: string) => boolean;
  errorOf: (teacherId: string) => string | undefined;
  inviteUrlOf: (teacherId: string) => string | undefined;
  /** Why the invitation mail failed, when a send was tried; undefined when no mail is configured. */
  sendErrorOf: (teacherId: string) => string | undefined;
  toast: string | undefined;
}

/** A status iam reported for the account, as the row reads it. */
function stateOfStatus(status: UserStatus): TeacherAccessState {
  return status === "active" || status === "invited" || status === "suspended" ? status : "unrecognised";
}

/** What a press's answer says about the login: "invited" only claims a mail when it left. */
function stateOfOutcome(result: TeacherAccessResult): TeacherAccessState {
  return result.status === "invited" && result.invitation?.sent === true ? "invitationSent" : result.status;
}

function without<V>(record: Record<string, V>, key: string): Record<string, V> {
  const { [key]: _dropped, ...rest } = record;
  return rest;
}

/**
 * The "Dar acceso al sistema" flow of the roster: what each row's login looks
 * like, the press itself (never sent twice for the same teacher while in
 * flight; different rows proceed independently), the question when an
 * existing account would be reused, the confirmation and the per-row failure.
 * The server call is the caller's, so the screen can relink the roster row it
 * owns. `accountStatuses` (user id → status) is what the screen could read
 * from Usuarios; without it a linked login is only "linked".
 */
export function useTeacherAccess(onGrantAccess?: GrantAccessCall, accountStatuses?: ReadonlyMap<string, UserStatus>): TeacherAccess {
  const [outcomes, setOutcomes] = useState<Record<string, TeacherAccessResult>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [confirmations, setConfirmations] = useState<Record<string, string>>({});
  const [granting, setGranting] = useState<ReadonlySet<string>>(() => new Set());
  const [toast, setToast] = useState<string | undefined>(undefined);
  const inFlight = useRef(new Set<string>());
  const toastTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(toastTimer.current), []);

  const announce = useCallback((message: string): void => {
    clearTimeout(toastTimer.current);
    setToast(message);
    toastTimer.current = setTimeout(() => setToast(undefined), TOAST_MS);
  }, []);

  const send = useCallback(
    async (teacher: Teacher, options?: GrantTeacherAccessOptions): Promise<void> => {
      if (onGrantAccess === undefined || inFlight.current.has(teacher.id)) return;
      inFlight.current.add(teacher.id);
      setGranting(new Set(inFlight.current));
      setErrors((prev) => without(prev, teacher.id));
      setConfirmations((prev) => without(prev, teacher.id));
      try {
        const result = await (options === undefined ? onGrantAccess(teacher.id) : onGrantAccess(teacher.id, options));
        setOutcomes((prev) => ({ ...prev, [teacher.id]: result }));
        announce(outcomeMessage(teacher, result));
      } catch (cause) {
        if (cause instanceof ReuseConfirmationRequired) {
          setConfirmations((prev) => ({ ...prev, [teacher.id]: cause.detail }));
        } else {
          setErrors((prev) => ({ ...prev, [teacher.id]: describeAccessError(cause) }));
        }
      } finally {
        inFlight.current.delete(teacher.id);
        setGranting(new Set(inFlight.current));
      }
    },
    [onGrantAccess, announce],
  );

  const grant = useCallback((teacher: Teacher) => send(teacher), [send]);
  const confirmReuse = useCallback((teacher: Teacher) => send(teacher, { confirmReuse: true }), [send]);
  const cancelReuse = useCallback((teacherId: string) => setConfirmations((prev) => without(prev, teacherId)), []);

  const stateOf = useCallback(
    (teacher: Teacher): TeacherAccessState => {
      const known = outcomes[teacher.id];
      if (known !== undefined) return stateOfOutcome(known);
      if (teacher.userId === "") return "none";
      const status = accountStatuses?.get(teacher.userId);
      return status === undefined ? "linked" : stateOfStatus(status);
    },
    [outcomes, accountStatuses],
  );

  return {
    stateOf,
    grant,
    confirmReuse,
    cancelReuse,
    confirmationOf: (id) => confirmations[id],
    isGranting: (id) => granting.has(id),
    errorOf: (id) => errors[id],
    inviteUrlOf: (id) => unsentLink(outcomes[id]),
    sendErrorOf: (id) => failedSendReason(outcomes[id]),
    toast,
  };
}

/** The link to hand over by hand: only when one was minted and not mailed. */
function unsentLink(result: TeacherAccessResult | undefined): string | undefined {
  const invitation = result?.invitation ?? null;
  return invitation !== null && !invitation.sent ? invitation.url : undefined;
}

/** The mail provider's reason, when one was configured and the send failed. */
function failedSendReason(result: TeacherAccessResult | undefined): string | undefined {
  const invitation = result?.invitation ?? null;
  if (invitation === null || invitation.sent) return undefined;
  const reason = invitation.sendError?.trim() ?? "";
  return reason === "" ? undefined : reason;
}

/**
 * What the office is told. Each branch says what actually happened: "Enviamos"
 * only when the mail went out, never about a link that is waiting to be copied.
 */
function outcomeMessage(teacher: Teacher, result: TeacherAccessResult): string {
  const name = `${teacher.firstName} ${teacher.lastName}`;
  if (result.status === "active") return `${name} ya entra al sistema con su contraseña.`;
  if (result.status === "suspended") return `La cuenta de ${name} está suspendida. Reactívala en Administración › Usuarios.`;
  if (result.invitation?.sent === true) return `Enviamos la invitación a ${result.email}`;
  return `Acceso creado para ${name}: copia la liga y entrégasela.`;
}
