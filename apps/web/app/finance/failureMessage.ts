import { failureKind, type FailureKind } from "./failureKind";

/** What one write says for each way it can fail; `network` and `forbidden` have house defaults. */
export interface FailureCopy {
  conflict: string;
  other: string;
  /** The verb phrase after "no tiene permiso para…": "anular pagos". */
  forbiddenAction: string;
}

const NETWORK = "No hubo conexión con el servidor. Revisa la red e intenta de nuevo.";

/** The Spanish sentence for a rejected finance write — never the server's English. */
export function failureMessage(cause: unknown, copy: FailureCopy): string {
  const byKind: Record<FailureKind, string> = {
    conflict: copy.conflict,
    forbidden: `Tu cuenta no tiene permiso para ${copy.forbiddenAction}.`,
    network: NETWORK,
    other: copy.other,
  };
  return byKind[failureKind(cause)];
}
