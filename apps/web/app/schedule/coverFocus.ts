/**
 * "Cubrir ahora" from Docentes (T19): the absence whose dates Horarios opens
 * on, handed over in memory. The shell's catch-all page remounts on a new
 * path, so React state cannot carry it; a module value lives for the whole
 * single-page session and is taken (cleared) by the first Horarios that mounts.
 */
export interface CoverFocus {
  startsOn: string;
  endsOn: string;
}

let pending: CoverFocus | undefined;

/** Ask the next Horarios to open on "Por cubrir" for these dates. */
export function requestCoverFocus(focus: CoverFocus): void {
  pending = focus;
}

/** The pending request, once. */
export function takeCoverFocus(): CoverFocus | undefined {
  const focus = pending;
  pending = undefined;
  return focus;
}
