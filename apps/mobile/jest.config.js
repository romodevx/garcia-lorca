/** @type {import('jest').Config} */
module.exports = {
  preset: "jest-expo",
  // RNTL v14 auto-registers its jest-native matchers on import of the main
  // module; the legacy `@testing-library/react-native/extend-expect` subpath was
  // removed. Point directly at the built matcher entry to extend `expect`.
  setupFilesAfterEnv: [
    "@testing-library/react-native/dist/matchers/extend-expect.js"
  ],
  transformIgnorePatterns: [
    "node_modules/(?!((jest-)?react-native|@react-native(-community)?|expo(nent)?|@expo(nent)?/.*|@expo-google-fonts/.*|react-navigation|@react-navigation/.*|@unimodules/.*|unimodules|sentry-expo|native-base|react-native-svg|@garcia-lorca/.*))"
  ],
  testMatch: ["**/*.test.{ts,tsx}"]
};
