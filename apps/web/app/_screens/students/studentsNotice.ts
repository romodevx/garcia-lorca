/**
 * The sentence the alta page leaves for the list it returns to.
 *
 * Module scope, not state: Next remounts <AppShell> on every route change, so
 * state set on /alumnos/nuevo dies before /alumnos paints. Module scope lives
 * through client-side navigation and is gone on a full reload — exactly the
 * lifetime of "you just created a pupil". Same shape as the level notice.
 */
let pending: string | undefined;

/** Leaves the confirmation for the next Estudiantes screen to show. */
export function leaveStudentsNotice(message: string): void {
  pending = message;
}

/** The waiting confirmation, consumed: it is said once, not on every visit. */
export function takeStudentsNotice(): string | undefined {
  const got = pending;
  pending = undefined;
  return got;
}
