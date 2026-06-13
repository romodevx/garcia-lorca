# Package manager: pnpm ONLY — npm/yarn forbidden

**Rule (binding, from the user):** In this project `npm` and `yarn` are **prohibited**.
Dependencies are installed **only with `pnpm`**.

**Why:** user policy for this repo (consistency, disk-efficient store, strict
node_modules, faster CI). Stated 2026-06-12.

**How to apply:**
- Install/add deps: `pnpm install`, `pnpm add <pkg>`, `pnpm add -D <pkg>`,
  workspace-scoped `pnpm --filter @garcia-lorca/<pkg> add <dep>`.
- Run scripts: `pnpm run <script>`, `pnpm --filter @garcia-lorca/web <script>`,
  `pnpm turbo run <task>`.
- Activate: `corepack enable pnpm` (corepack ships with Node; pnpm pinned via
  `packageManager` in root `package.json`).
- Workspaces are declared in `pnpm-workspace.yaml` (NOT the `workspaces` field).
- Only lockfile committed: `pnpm-lock.yaml`. Never commit `package-lock.json` /
  `yarn.lock` (both gitignored).
- A `preinstall` guard (`only-allow pnpm`) hard-blocks npm/yarn installs.
- CI (`deploy/cloudbuild/cloudbuild.yaml`) and the root `Makefile` use pnpm.

Any new agent doing frontend/TS work MUST use pnpm. See [[web-frontend-design]].
