# `proto/` — Contract source of truth

All service contracts for García Lorca live here as Protobuf. This directory is
the **single source of truth**: Go and TypeScript types are generated from it,
never written by hand.

## The golden rule

**Nobody edits `gen/`.** Change a `.proto` here → run `buf generate` → Go (`gen/go`)
and TS (`gen/ts`) receive the new types. `gen/` is in `.gitignore` and is
regenerated in CI; any manual edit there will be blown away on the next generate.

## Layout

```
proto/garcialorca/<domain>/v1/<domain>.proto   package garcialorca.<domain>.v1
```

One module rooted at `proto/`. Domains: `iam`, `students`, `finance`, `docs`,
`attendance`, `grades`, `uniforms`, `comms`, `billing`. Scholarships (becas) are
part of `finance` (ADR 0022); their old stub package was retired.

`go_package` is **not** declared in the `.proto` files — buf **managed mode**
injects the prefix `github.com/garcia-lorca/gen/go` (see `buf.yaml` / `buf.gen.yaml`).

## Workflow

```bash
buf lint        # style + layout checks (STANDARD rules)
buf breaking    # backward-compatibility check (FILE rules) against the base ref
buf generate    # regenerate gen/go (protobuf-go + connect-go) and gen/ts (es + connect-query)
```

Plugins are **remote** (`buf.build/...`), so no local `protoc` or plugin install
is required. A network connection is needed for `buf generate`.

## Generation targets

| Target | Plugins | Output |
|--------|---------|--------|
| Go     | `protocolbuffers/go`, `connectrpc/go` | `gen/go` |
| TS     | `bufbuild/es`, `connectrpc/query-es`  | `gen/ts` |

## Fase 0 status

These are **contract-first stubs**: each service has a `Health` RPC plus the key
message types implied by the events each module publishes (see `MODULES.md`).
Fields marked `TODO(<module>)` are fleshed out when that module is built, in the
fixed flow: contract → data+RLS → backend (TDD) → frontend → flag.

`billing/v1` is **FUTURE / post-MVP** — a frozen, reserved boundary only. Do not
implement it until billing is promoted out of post-MVP (gated by spike S01 + CSD).
