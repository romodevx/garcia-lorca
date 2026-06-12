# @garcia-lorca/config

Shared presets for the monorepo. Single source of truth for lint/TS/styling.

## Exports

| Import | Purpose |
|--------|---------|
| `@garcia-lorca/config/eslint` | Flat-config ESLint preset (`baseConfig`). |
| `@garcia-lorca/config/tailwind` | Tailwind preset with brand design tokens. |
| `@garcia-lorca/config/tsconfig.base.json` | Base TS options (re-exports root `tsconfig.base.json`). |
| `@garcia-lorca/config/tsconfig.react.json` | React/JSX TS preset. |

## Usage

`eslint.config.js`:

```js
import { baseConfig } from "@garcia-lorca/config/eslint";
export default [...baseConfig];
```

`tailwind.config.js`:

```js
import preset from "@garcia-lorca/config/tailwind";
export default { presets: [preset], content: ["./app/**/*.{ts,tsx}"] };
```
