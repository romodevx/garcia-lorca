import AsyncStorage from "@react-native-async-storage/async-storage";

/**
 * The facts push registration must remember across launches: the token this
 * phone registered (so sign-out can remove exactly that one), the tokens whose
 * removal did not reach the gateway yet (retried on the next launch), and that
 * we have already asked the parent for permission (so a "no" is never asked
 * again).
 */

const TOKEN_KEY = "garcialorca.push.registeredToken";
const ASKED_KEY = "garcialorca.push.permissionAsked";
const PENDING_KEY = "garcialorca.push.pendingUnregister";

export async function readRegisteredToken(): Promise<string | null> {
  return AsyncStorage.getItem(TOKEN_KEY);
}

export async function saveRegisteredToken(token: string): Promise<void> {
  await AsyncStorage.setItem(TOKEN_KEY, token);
}

export async function forgetRegisteredToken(): Promise<void> {
  await AsyncStorage.removeItem(TOKEN_KEY);
}

export async function wasPermissionAsked(): Promise<boolean> {
  return (await AsyncStorage.getItem(ASKED_KEY)) === "1";
}

export async function markPermissionAsked(): Promise<void> {
  await AsyncStorage.setItem(ASKED_KEY, "1");
}

/** Tokens whose DELETE has not been confirmed by the gateway yet. */
export async function readPendingUnregisters(): Promise<string[]> {
  const raw = await AsyncStorage.getItem(PENDING_KEY);
  if (raw === null) {
    return [];
  }
  try {
    const parsed: unknown = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((t): t is string => typeof t === "string") : [];
  } catch {
    return [];
  }
}

async function writePendingUnregisters(tokens: string[]): Promise<void> {
  if (tokens.length === 0) {
    await AsyncStorage.removeItem(PENDING_KEY);
    return;
  }
  await AsyncStorage.setItem(PENDING_KEY, JSON.stringify(tokens));
}

export async function savePendingUnregister(token: string): Promise<void> {
  const pending = await readPendingUnregisters();
  if (!pending.includes(token)) {
    await writePendingUnregisters([...pending, token]);
  }
}

export async function forgetPendingUnregister(token: string): Promise<void> {
  const pending = await readPendingUnregisters();
  await writePendingUnregisters(pending.filter((t) => t !== token));
}
