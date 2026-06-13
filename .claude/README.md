# `.claude/` — Project workspace

Project-scoped memory, documents, and tasks live here so the repo root stays clean
(only `CLAUDE.md` and `README.md` are `.md` files at root).

```
.claude/
├── memory/   Project memory — durable notes/decisions across sessions (one file per topic)
├── docs/     Project documents
│   ├── MODULES.md          Module backlog (MoSCoW)
│   ├── FLAGS.md            Feature-flag registry
│   ├── PLAN-sprint-01.md   Sprint execution plan
│   ├── adr/               Architecture Decision Records
│   └── design/            Design references (handoff bundles, prototypes)
└── tasks/    Task tracking (one file per task or a running board)
```

Conventions:
- **Memory** = facts that should survive across sessions (decisions, gotchas, "why").
  One concern per file, kebab-case names.
- **Docs** = curated deliverables (backlog, flags, plan, ADRs, design refs).
- **Tasks** = in-flight work items and their status.
- Root keeps only `CLAUDE.md` (agent context) and `README.md` (human onboarding).
