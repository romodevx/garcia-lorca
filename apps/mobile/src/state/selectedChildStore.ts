import AsyncStorage from "@react-native-async-storage/async-storage";

/**
 * The child a family last chose (ISSUE-CC-303), so a relaunch opens on the
 * same one. Only an id: it is honoured only while it is among the signed-in
 * guardian's own children, so another guardian on the same phone never lands
 * on it. Storage failing is non-fatal: the app falls back to the first child.
 */
const STORAGE_KEY = "gl.selectedChildId";

export async function readSelectedChild(): Promise<string | null> {
  try {
    return await AsyncStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

/**
 * Drops the remembered child: on logout and on a lost session, so the next
 * guardian on a shared phone starts from nothing of the previous one's.
 */
export async function forgetSelectedChild(): Promise<void> {
  try {
    await AsyncStorage.removeItem(STORAGE_KEY);
  } catch {
    // Non-fatal: an id that is not among the next guardian's children is ignored.
  }
}

export function rememberSelectedChild(id: string): void {
  AsyncStorage.setItem(STORAGE_KEY, id).catch(() => {
    // Non-fatal: the in-memory choice still applies for this session.
  });
}
