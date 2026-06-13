const path = require("path");

// Resolve a single physical React root so the app code, react-native, and
// react-test-renderer all share ONE React instance. pnpm's symlinked store can
// otherwise surface two identities to Jest, which silently breaks hooks
// (state updates never flush; useRef reads null).
const reactDir = path.dirname(require.resolve("react/package.json"));

/** @type {import('jest').Config} */
module.exports = {
  preset: "jest-expo",
  // RNTL v14 auto-registers its jest-native matchers on import of the main
  // module; the legacy `@testing-library/react-native/extend-expect` subpath was
  // removed. Point directly at the built matcher entry to extend `expect`.
  setupFilesAfterEnv: [
    "@testing-library/react-native/dist/matchers/extend-expect.js",
    "<rootDir>/jest.setup.js"
  ],
  // Whitelist the RN/Expo packages that ship untranspiled ESM/TS so Babel
  // transforms them. expo-modules-core, expo-*, async-storage, safe-area,
  // svg and worklets are all consumed transitively by the providers under test.
  transformIgnorePatterns: [
    "node_modules/(?!(?:.pnpm/[^/]+/node_modules/)?((jest-)?react-native|@react-native(-community)?|@react-native-async-storage/.*|expo|expo-.*|@expo(nent)?/.*|@expo-google-fonts/.*|react-navigation|@react-navigation/.*|react-native-safe-area-context|react-native-svg|react-native-worklets|@unimodules/.*|unimodules|sentry-expo|native-base|@garcia-lorca/.*))"
  ],
  testMatch: ["**/*.test.{ts,tsx}"],
  // pnpm's symlinked store can surface two physical React module instances to
  // Jest's resolver (the app's `node_modules/react` symlink vs the .pnpm
  // target), which breaks hooks with "Cannot read properties of null". Pin a
  // single instance of React and the renderer.
  moduleNameMapper: {
    "^react$": path.join(reactDir, "index.js"),
    "^react/jsx-runtime$": path.join(reactDir, "jsx-runtime.js"),
    "^react/jsx-dev-runtime$": path.join(reactDir, "jsx-dev-runtime.js")
  }
};
