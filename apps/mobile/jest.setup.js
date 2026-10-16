/* Test environment shims. */

// Default network stub: the AppState provider fetches family notifications on
// mount, so every screen rendered through the provider stack issues a request.
// Return an empty, well-formed envelope by default; individual tests override
// `global.fetch` with jest.fn() when they need specific responses.
global.fetch = jest.fn(async () => ({
  ok: true,
  status: 200,
  json: async () => ({ notifications: [] }),
}));

// expo-router: stub the imperative router + hooks used by screens.
jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn(), navigate: jest.fn() },
  useLocalSearchParams: () => ({}),
}));


// AsyncStorage: use the official in-memory jest mock.
jest.mock("@react-native-async-storage/async-storage", () =>
  require("@react-native-async-storage/async-storage/jest/async-storage-mock"),
);

// expo-secure-store is a native keychain module; keep its backing in memory.
jest.mock("expo-secure-store", () => {
  const values = new Map();
  return {
    getItemAsync: jest.fn(async (key) => values.get(key) ?? null),
    setItemAsync: jest.fn(async (key, value) => values.set(key, value)),
    deleteItemAsync: jest.fn(async (key) => values.delete(key)),
  };
}, { virtual: true });

// expo-font: fonts are always "loaded" in tests so screens render immediately.
jest.mock("expo-font", () => ({
  useFonts: () => [true, null],
  isLoaded: () => true,
  loadAsync: jest.fn(() => Promise.resolve()),
}));

// The google-fonts packages only export font-asset modules used by useFonts,
// which is mocked above; stub them so Metro asset requires don't run in Jest.
jest.mock("@expo-google-fonts/baloo-2", () => ({
  Baloo2_700Bold: "Baloo2_700Bold",
  Baloo2_600SemiBold: "Baloo2_600SemiBold",
}));
jest.mock("@expo-google-fonts/nunito", () => ({
  Nunito_700Bold: "Nunito_700Bold",
  Nunito_600SemiBold: "Nunito_600SemiBold",
  Nunito_800ExtraBold: "Nunito_800ExtraBold",
  Nunito_500Medium: "Nunito_500Medium",
}));

// expo-notifications is a native module; the default stub keeps every screen
// and route renderable. Push tests replace it with their own jest.mock.
jest.mock("expo-notifications", () => ({
  setNotificationHandler: jest.fn(),
  addNotificationReceivedListener: jest.fn(() => ({ remove: jest.fn() })),
  addNotificationResponseReceivedListener: jest.fn(() => ({ remove: jest.fn() })),
  getLastNotificationResponseAsync: jest.fn(async () => null),
  getPermissionsAsync: jest.fn(async () => ({ status: "undetermined", granted: false, canAskAgain: true })),
  requestPermissionsAsync: jest.fn(async () => ({ status: "denied", granted: false, canAskAgain: false })),
  getExpoPushTokenAsync: jest.fn(async () => ({ type: "expo", data: "ExponentPushToken[test]" })),
  setNotificationChannelAsync: jest.fn(async () => null),
  AndroidImportance: { HIGH: 4 },
}));

// jest-expo answers requireNativeModule() for ANY name, so expo's
// isRunningInExpoGo() reads true under Jest. Tests model a dev/store build,
// where push works; the Expo Go case mocks this back to true itself.
jest.mock("expo", () => ({ ...jest.requireActual("expo"), isRunningInExpoGo: () => false }));
