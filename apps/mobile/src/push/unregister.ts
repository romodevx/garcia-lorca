import { getAccessToken } from "../api/tokenStore";
import { unregisterDevice } from "../api/devices";
import { ApiError } from "../api/errors";
import {
  forgetPendingUnregister,
  forgetRegisteredToken,
  readPendingUnregisters,
  readRegisteredToken,
  savePendingUnregister,
} from "./pushStore";

/** Sign-out must never wait on a slow network longer than this. */
const UNREGISTER_TIMEOUT_MS = 3_000;

const HTTP_BAD_REQUEST = 400;
const HTTP_UNAUTHORIZED = 401;
const HTTP_SERVER_ERROR = 500;

/**
 * Whether a DELETE outcome is final. Success is; so is a client error other
 * than 401 (a malformed token — a retry could never succeed). A network
 * failure, a timeout, a 401 or a 5xx is not: the gateway may still hold it.
 */
function isSettled(failure: unknown): boolean {
  if (!(failure instanceof ApiError)) {
    return false;
  }
  const { status } = failure;
  return status >= HTTP_BAD_REQUEST && status < HTTP_SERVER_ERROR && status !== HTTP_UNAUTHORIZED;
}

async function deleteToken(token: string): Promise<boolean> {
  try {
    await unregisterDevice(token);
    return true;
  } catch (failure) {
    return isSettled(failure);
  }
}

async function deleteWithin(token: string, ms: number): Promise<boolean> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<boolean>((resolve) => {
    timer = setTimeout(() => resolve(false), ms);
  });
  const deleted = await Promise.race([deleteToken(token), timeout]);
  clearTimeout(timer);
  return deleted;
}

/**
 * Removes this phone's push token from the gateway on sign-out (and on a lost
 * session), so the next person holding a shared family phone stops receiving
 * the child's news. Best-effort and time-boxed: it runs BEFORE the session
 * tokens are cleared (the DELETE needs the Bearer), and it never strands
 * someone in a session they asked to leave. When the DELETE is not confirmed —
 * offline, timeout, error, or the session is already gone — the token is kept
 * as a pending delete and retried before the next registration.
 */
export async function unregisterPush(): Promise<void> {
  const token = await readRegisteredToken().catch(() => null);
  if (token === null) {
    return;
  }
  const deleted = getAccessToken() !== null && (await deleteWithin(token, UNREGISTER_TIMEOUT_MS));
  if (!deleted) {
    await savePendingUnregister(token).catch(() => undefined);
  }
  await forgetRegisteredToken().catch(() => undefined);
}

/**
 * Retries every pending delete. True when none is left, so registration may
 * proceed; false when one still could not be deleted — registering the same
 * token now would let that later DELETE remove the fresh registration.
 */
export async function retryPendingUnregisters(): Promise<boolean> {
  const pending = await readPendingUnregisters().catch(() => [] as string[]);
  let allDone = true;
  for (const token of pending) {
    if (await deleteToken(token)) {
      await forgetPendingUnregister(token).catch(() => undefined);
    } else {
      allDone = false;
    }
  }
  return allDone;
}
