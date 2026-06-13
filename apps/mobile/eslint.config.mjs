import { baseConfig } from "@garcia-lorca/config/eslint";
import globals from "globals";

/** @type {import("eslint").Linter.Config[]} */
export default [
  ...baseConfig,
  {
    // Jest's config + setup are CommonJS by necessity (the jest-expo preset and
    // module mocks are resolved via require). Allow require() + jest/node globals
    // in these files only.
    files: ["jest.config.js", "jest.setup.js"],
    languageOptions: {
      globals: {
        ...globals.node,
        ...globals.jest
      }
    },
    rules: {
      "@typescript-eslint/no-require-imports": "off"
    }
  },
  {
    // Test files use jest globals (describe/it/expect/jest).
    files: ["**/*.test.{ts,tsx}"],
    languageOptions: {
      globals: {
        ...globals.jest
      }
    }
  }
];
