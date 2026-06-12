# @garcia-lorca/api-client

Thin wrapper over the **generated** Connect-Query clients. App code imports only
`@garcia-lorca/api-client` — never deep paths into `gen/ts`.

## Generated-code dependency

This package consumes `gen/ts`, produced by `buf generate` (Protobuf-ES +
Connect-Query/ES, see root `buf.gen.yaml`). `gen/` is gitignored and never
hand-edited — change a `.proto`, run `buf generate`, get new types.

The path alias `@garcia-lorca/gen` → `gen/ts` is wired in `tsconfig.base.json`
and this package's `tsconfig.json`. Until generation runs, `src/clients.ts`
re-exports nothing (its examples are commented out).

## Usage (after generation)

```ts
import { createTransport, studentsQuery } from "@garcia-lorca/api-client";

const transport = createTransport({ baseUrl: process.env.NEXT_PUBLIC_API_URL! });
// pass transport to the Connect-Query TransportProvider; call generated hooks.
```

Base URL MUST be a PUBLIC env var — never an internal URL baked into the bundle.
