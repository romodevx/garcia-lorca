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

export function rememberSelectedChild(id: string): void {
  AsyncStorage.setItem(STORAGE_KEY, id).catch(() => {
    // Non-fatal: the in-memory choice still applies for this session.
  });
}
