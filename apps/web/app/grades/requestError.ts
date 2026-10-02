/**
 * A refused grades request (`/grades/*`, `/teaching/grades/*`): the server
 * answers text/plain — a sentence shown as is — with its machine code in
 * `X-Error-Code`.
 */

export class GradeRequestError extends Error {
  readonly status: number;
  readonly detail: string;
  /** The refusal's machine code (`X-Error-Code`), when the server sent one. */
  readonly code: string | undefined;

  constructor(status: number, detail: string, code?: string) {
    super(detail !== "" ? detail : `HTTP ${status}`);
    this.name = "GradeRequestError";
    this.status = status;
    this.detail = detail;
    this.code = code;
  }
}

export const ERROR_CODE_HEADER = "X-Error-Code";

export async function gradeFailure(res: Response): Promise<GradeRequestError> {
  const body = await res.text().catch(() => "");
  return new GradeRequestError(res.status, body.trim(), res.headers.get(ERROR_CODE_HEADER) ?? undefined);
}
