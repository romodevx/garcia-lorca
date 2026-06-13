/* Test environment shims. */

// expo-router: stub the imperative router + hooks used by screens.
jest.mock("expo-router", () => ({
  router: { push: jest.fn(), replace: jest.fn(), back: jest.fn(), navigate: jest.fn() },
  useLocalSearchParams: () => ({}),
}));


// AsyncStorage: use the official in-memory jest mock.
jest.mock("@react-native-async-storage/async-storage", () =>
  require("@react-native-async-storage/async-storage/jest/async-storage-mock"),
);

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
