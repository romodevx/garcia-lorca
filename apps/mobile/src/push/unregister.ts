import { unregisterDevice } from "../api/devices";
import { forgetRegisteredToken, readRegisteredToken } from "./pushStore";

/** Sign-out must never wait on a slow network longer than this. */
const UNREGISTER_TIMEOUT_MS = 3_000;

/**
 * Removes this phone's push token from the gateway on sign-out, so the next
 * person holding a shared family phone stops receiving the child's news.
 * Best-effort and time-boxed: it must run BEFORE the session tokens are cleared
 * (the DELETE needs the Bearer), and it must never strand someone in a session
 * they asked to leave. No native import on purpose — sign-out stays cheap.
 */
export async function unregisterPush(): Promise<void> {
  const token = await readRegisteredToken().catch(() => null);
  if (token === null) {
    return;
  }
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<void>((resolve) => {
    timer = setTimeout(resolve, UNREGISTER_TIMEOUT_MS);
  });
  await Promise.race([unregisterDevice(token).catch(() => undefined), timeout]);
  clearTimeout(timer);
  await forgetRegisteredToken().catch(() => undefined);
}
