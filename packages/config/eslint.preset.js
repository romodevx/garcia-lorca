// Shared ESLint flat-config preset for the García Lorca monorepo.
// Apps/packages import and spread this into their own eslint.config.mjs.
// Enforces the Clean Code rule from ~/.claude/rules: no `any` abuse, no
// console/debugger left in source (build also strips them — defense in depth).
import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";

/** @type {import("eslint").Linter.Config[]} */
export const baseConfig = [
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    // App/source files run in browser + modern runtime; allow those globals.
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.es2022
      }
    },
    rules: {
      "@typescript-eslint/no-explicit-any": "error",
      "@typescript-eslint/no-unused-vars": [
        "error",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" }
      ],
      "no-console": ["warn", { allow: ["warn", "error"] }],
      "no-debugger": "error",
      // TypeScript already reports undefined identifiers; `no-undef` is noisy
      // and discouraged on TS (it cannot see type-only/ambient globals).
      "no-undef": "off"
    }
  },
  {
    // Config files, build scripts, and Node tooling run in Node (CommonJS or ESM).
    files: [
      "**/*.config.{js,mjs,cjs,ts}",
      "**/scripts/**",
      "**/*.cjs"
    ],
    languageOptions: {
      globals: {
        ...globals.node
      }
    },
    rules: {
      // CLI/build scripts legitimately print to stdout.
      "no-console": "off"
    }
  },
  {
    ignores: [
      "**/dist/**",
      "**/.next/**",
      "**/.expo/**",
      "**/.turbo/**",
      "**/gen/**"
    ]
  }
];

export default baseConfig;
