// Shared ESLint flat-config preset for the García Lorca monorepo.
// Apps/packages import and spread this into their own eslint.config.mjs.
// Enforces the Clean Code rule from ~/.claude/rules: no `any` abuse, no
// console/debugger left in source (build also strips them — defense in depth).
import js from "@eslint/js";
import globals from "globals";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";

/** @type {import("eslint").Linter.Config[]} */
export const baseConfig = [
  js.configs.recommended,
  ...tseslint.configs.recommended,
  {
    /*
     * LAS REGLAS DE LOS HOOKS.
     *
     * Faltaban. El monorepo linteaba TypeScript y no linteaba React, así que
     * `pnpm lint` pasaba en verde sobre un componente con un `useMemo` debajo de
     * un `return null` — que revienta con "Rendered more hooks than during the
     * previous render" en cuanto se monta. Pasó de verdad, en el buscador de
     * alumnos de Finanzas (2026-09-11), y ni las 818 pruebas ni el typecheck lo
     * vieron: sólo se rompe al renderizar ese componente, y nada lo renderizaba.
     *
     * `tsc` no puede ver esto (no es de tipos) y una prueba sólo lo ve si
     * alguien se acordó de escribirla para ESE componente. La regla lo ve en los
     * sesenta.
     */
    files: ["**/*.{jsx,tsx}"],
    plugins: { "react-hooks": reactHooks },
    rules: {
      "react-hooks/rules-of-hooks": "error",
      // Como aviso y no como error: el repo tiene efectos deliberadamente de una
      // sola corrida, y convertirlos en error a la brava invita a silenciarlos.
      "react-hooks/exhaustive-deps": "warn",
    },
  },
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
