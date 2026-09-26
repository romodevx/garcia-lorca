import AsyncStorage from "@react-native-async-storage/async-storage";

/**
 * The two facts push registration must remember across launches: the token this
 * phone registered (so sign-out can remove exactly that one), and that we have
 * already asked the parent for permission (so a "no" is never asked again).
 */

const TOKEN_KEY = "garcialorca.push.registeredToken";
const ASKED_KEY = "garcialorca.push.permissionAsked";

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
