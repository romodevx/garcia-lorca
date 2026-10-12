# Eng review: PLAN-redesign-avisos (borrador v1)

Reviewer: `/plan-eng-review`, spawned and non-interactive (recommended option auto-chosen at every decision point) · 2026-10-12 · branch `feature/redesign-avisos` @ `403832ea`
Target: `.claude/docs/design/PLAN-redesign-avisos.md`. Contract: `.gstack/redesign/avisos/behavior.md`.
Evidence kinds: static read of the files cited, plus two live runs:
- `pnpm vitest run app/_screens/announcements`: 3 files, 31 tests green on the current UI.
- A throwaway jsdom/user-event probe of `form=` submission. It was deleted afterwards and `git status` is clean apart from the plan file.

No source was edited and nothing was committed.

## Verdict in one paragraph

The approach holds up: one hook, two views, the characterization suite run against both, and a presentation-only flag. These are the right building blocks. Three gaps would let the "same calls, same order" promise break without any test noticing, and one gap would stop the build from compiling:

- **E1** The plan never touches the flag key union, so the build does not compile.
- **E2** A duplicated summary for tablet would break the role queries in every test.
- **E3** Submitting through `form=` hides a regression in Enter-to-submit. The probe confirmed this.
- **E4** Nothing asserts the exact set of network calls.

Fix those four in the plan and it is ready to build.

## Step 0: Scope challenge

- Scope accepted as-is (FULL_REVIEW). This is a presentation change behind a flag. The six new components are the minimum the variant needs. The hook extraction is "make the change easy first" (Beck), done as a refactor under green before any new view exists.
- Section 7 (BC1 to BC5) correctly keeps behavior changes out. Nothing to cut.
- The estimate is plausible (CC ~2 to 3 h). Add roughly 30 min for E3, E4 and E12.

## What already exists (reuse ladder)

| Need | Already in repo | Plan uses it? |
|---|---|---|
| Level, then grade ordinal, then letter ordering (es-MX, numeric) | `apps/web/app/config/schoolOrder.ts:18` `schoolOrder`, `sortBySchoolOrder` (used by `useDirectorClasses.ts:86`, `useDirectorAssignments.ts:5`) | **No.** The plan writes a new ordering (E6) |
| Accent-folding search | `_design/textSearch.ts:8` `matchesSearch(query, ...fields)` | Yes |
| Required mark, help, a11y wiring | `_design/form/fields.tsx:178` `TextArea` (`aria-required` only, **no native `required`**, so it cannot block submit) | Yes. Default `rows=5` (E13) |
| Presentation-flag precedent | `module_config_setup`: `flags.go:43`, `flags_test.go:123`, `features_test.go:54`, `AppShell.configSetup.test.tsx`, `e2e/config-setup.flag.spec.ts` + its own Playwright project (`playwright.config.ts:64,79`) | Partly (E1, E11, E12) |
| Dead-prop guard | `_design/contratos.test.ts`: every exported `*Props` field and every exported `_design/` interface field needs a reader | Yes. `LabelledGroup` lives in `config/`, so the guard does not watch it |

## Section 1: Architecture

**E1 · HIGH (blocker): the flag key is not in the typed vocabulary.**
Evidence: `useFlag(flag: FlagKey)` at `packages/ui/src/flags/FlagsProvider.tsx:34`. `FlagKey` is a closed union in `packages/ui/src/flags/types.ts:6-27` that ends at `"module_ui_refresh"`. The plan's file list (section 6) leaves out `packages/ui`, so `useFlag("module_announcements_view")` fails `tsc` and `next build`.
Amendment: add `| "module_announcements_view"` to `types.ts` and list it in section 6 under "Cambian". Do it in the same commit as the gateway registration.

**E2 · HIGH: the responsive summary must be ONE DOM node.**
Evidence: section 2 says that below 1024 px "el resumen va después del redactor y queda sticky bottom con sólo 'Va a …' + Enviar". If that is built as a second copy hidden with a media query, jsdom ignores the media query and renders both copies. Then `getByRole("button", { name: /Enviar aviso/ })` (`behavior.test.tsx:58`), `findByRole("alert")` (`:138`, `:148`, `AnnouncementsScreen.test.tsx:85`) and `findByRole("status")` all throw "multiple elements". In a real browser two `role=status`/`role=alert` regions get read twice.
Amendment: one `ComposeSummary` instance. Only CSS changes at < 1024 px (grid reflow and `position: sticky`). Hide the preview part with `display: none` under the media query. Do not mount a second component. Add a sentence to section 3.2 and a test: exactly one "Enviar aviso" button, one alert, one status.

**E3 · HIGH: the `form=` id submit loses Enter-to-submit in tests. Put the aside inside the `<form>` instead.**
Evidence: live probe, jsdom + `@testing-library/user-event`, with a `<form id="f">` holding two inputs and an `<aside><button type="submit" form="f">` outside it:
- clicking the button submitted once;
- `{Enter}` in a field submitted **0** times.

Real browsers do implicit submission through the form's default button, which includes `form=`-associated buttons. So production would work while the tests could never catch a regression. Enter-to-submit exists today: the button is inside the form at `AnnouncementsScreen.tsx:149-229`, and `ui-forms-and-modals.md` requires it.
Amendment:
- Make the `<form onSubmit={submit}>` itself the two-column grid. Both the composer column and the sticky aside go inside it, and `form=` goes away.
- `AutomaticMessages`/`BirthdayGreeting` stays **outside** that form, because it has its own `<form>` (`BirthdayGreeting.tsx:125`) and forms cannot nest.
- `position: sticky` works the same inside the form.
- Add a characterization test, green on the old view first and then run through `describe.each`: "Enter in Título submits".

**E10 · MEDIUM: pin the hook's shape so both views share every rule, not just the state.**
Evidence: today B2 lives inline in JSX: `onClick={() => { setScope(s.value); setPicked([]); }}` at `AnnouncementsScreen.tsx:159`. The `targets` derivation (`:116-120`) and `toggle` (`:122-131`) are render-local.
Amendment: `useAnnouncementComposer()` returns:
- `{ cycle, levels, groups, students, scope, picked, title, body, sending, sent, error }`;
- the handlers `chooseScope(scope)` (set the scope and clear the selection, which is B2 in one place), `toggle(id)` (B4/B5), `setTitle`, `setBody` and `submit(e: FormEvent)` (keeps `preventDefault` and the D1 to D3 order verbatim);
- no JSX.

`newAttemptKey` and `SCOPES` move with it, or `SCOPES` moves to `audience.ts`. Keep `cycle = cycles.find(...) ?? cycles[0]` exactly as it is: the effect dependency `[cycle]` at `:75` relies on the object identity coming from state, so do not wrap it in a `useMemo` that changes identity. Reword "sin cambiar su JSX" to "the JSX structure stays the same; it only reads the hook's names". The old screen's `targets` can be a pure `targetsFor(scope, levels, groups, students)` in `audience.ts`. Do not add a separate `renderHook` suite: the characterization suite running against both views is the hook's test, and a third copy of the same assertions would not catch anything new.

**E16 · MEDIUM: the view flag must sit inside the `module_comms` gate.**
Evidence: `AppShell.tsx:645-652`, `commsEnabled ? <AnnouncementsScreen/> : <ModulePlaceholder/>`.
Amendment: `commsEnabled ? (announcementsView ? <AnnouncementsCompose/> : <AnnouncementsScreen/>) : <ModulePlaceholder/>`, which mirrors the config one at `:670-678`. The AppShell test has three cases: comms ON with view OFF shows the old view, comms ON with view ON shows the new one, comms OFF with view ON shows the placeholder.

## Section 2: Code quality

**E6 · MEDIUM: reuse `schoolOrder`, do not write a third ordering.**
Evidence: `config/schoolOrder.ts:18-30` already sorts level index, then grade ordinal, then `localeCompare(es-MX, numeric)`, with unknown entries last. Section 3.2's `groupTargetsByLevel` re-specifies the same rule.
Amendment:
- `audience.ts` builds a `SchoolCatalogue` from `levels` (A2, already loaded) and the additive group fields (`{ id: gradeLevelId, levelId, ordinal, name: gradeName }`, de-duplicated).
- It sorts with `sortBySchoolOrder` and only then buckets by level. `audience.test.ts` tests the bucketing and the "Otros" rule, not a second comparator.
- Edge case: when no group has a known level (for example `listGradeLevels` or `listLevels` failed, both silent: `api.ts:698-699`, A2), render one flat list with **no** "Otros" header.

**E7 · MEDIUM: chips that read "1° A" need the bare grade name, which the plan does not provide.**
Evidence: `labelGroups` builds `label = "1° PRIMARIA · A"` (`api.ts:716-719`; `labelGroups.regression-1.test.ts:36`). The mock shows chips that read "1° A" under a PRIMARIA heading. Getting there from `label` means string surgery on a localized label.
Amendment: the additive set becomes `levelId?`, `ordinal?`, `gradeName?`, all taken from the grade `labelGroups` already has. The visible chip text is `${gradeName} ${name}`. **`aria-label` stays `g.label`** (B3, and the tests query `"1° A"` from the mock label). There is still no new call.

**E9 · MEDIUM: the "mute hint" duplicates the D3 error text.**
Evidence: section 3.2's copy uses "Elige a quién va dirigido" as a hint, and D3's alert text is "Elige a quién va dirigido." (`AnnouncementsScreen.tsx:88`). After a failed submit the same sentence shows twice, once as `role=alert`, which is confusing and also a trap for `getByText` in tests.
Amendment: hint wording "Todavía no eliges a nadie" (passed through `humanizer`), and the hint is hidden while `error` is set. The error text does not change.

**E13 · LOW: `TextArea` defaults to 5 rows, and C2 says 4.**
Evidence: `fields.tsx:179` (`rows = 5`) against `behavior.md` C2.
Amendment: `<TextArea label="Mensaje" rows={4} required help="El aviso completo…" />` with the help text copied verbatim. Pass no `error` prop: `Field` would render a second `role=alert` (`Field.tsx:79-82`). The generated help id replacing `aviso-cuerpo-help` is fine, because nothing outside the file references it.

**E8 · LOW: the additive `labelGroups` change is safe, and the plan should show the evidence.**
Evidence:
- Production callers: `config/api.ts:701` (`listLabelledGroups`) and `_screens/teachers/useTeacherLoadData.ts:104` (direct).
- Tests that run the real function assert only `.label` arrays: `config/api.test.ts:305,439,447` and `config/labelGroups.regression-1.test.ts:36,40,54,64`.
- The other roughly 30 test files mock `listLabelledGroups`.
- No consumer spreads a `LabelledGroup` into a request body (grep for `...g` / `JSON.stringify`).

Amendment: in the `labelGroups` test, assert the fields are **omitted** (`not.toHaveProperty("levelId")`) when the grade is unknown, rather than set to `undefined`. Run the whole web suite once after the change, not only the announcements folder.

## Section 3: Tests

**E4 · HIGH: the freeze has no call-level assertion.**
Evidence: the suite checks `announce` payload fields and `listLabelledGroups` called with `"c2"` (`behavior.test.tsx:80`), but never how many times each load runs or that nothing else runs. A new view that refetches students on every keystroke, or calls `listGradeLevels` for grouping, would stay green. (Note: in the behavior file, a call to an unmocked export would throw, because the factory at `:22` only defines three functions. That is an accident, and it holds in that one file only.)
Amendment: add **A0** to the `describe.each` suite and write it green against the old view first:
- After mount and settle: `listCycles`, `listLevels`, `listStudents` and `getBirthdayTemplate` are each called exactly once; `listLabelledGroups` is called once with `(API_URL, "c2")`; `announce` is not called.
- After switching to every scope and typing in the student search, the call counts have not changed.
- For each of the four scopes, the full `announce` argument is checked with `toEqual`, not field by field. Use `expect.any(String)` for the key.

**E5 · MEDIUM: `describe.each` plus per-view `pickStudent()`: keep the assertions shared and make B5 independent of the view.**
Evidence: B5 (`behavior.test.tsx:110-119`) queries the *unpicked* pupil by role after picking the second one. In the new view `StudentSearch` filters by the typed text, so "Cruz Valentina" may not be in the DOM after typing "Andrade", and `getByRole` throws. The test then fails for a reason that has nothing to do with B5.
Amendment:
- `pickStudent(user, name)` is the **only** per-view step. In the old view it clicks. In the new view it types the surname into "Buscar alumno" and then clicks.
- B5's shared assertions become: the pupil shown last is `aria-pressed=true`, and after sending, `announce` gets `targetIds: ["s2"]`, which is the actual freeze.
- Specify the empty-query state in section 3.2. Recommended: no result list until the user types, with the picked pupil pinned above the list.
- Result buttons keep `aria-label={formatStudentName(s)}` **exactly**, so the `gradeGroup` text does not change the accessible name.
- The suites run as `describe.each([["old", AnnouncementsScreen, oldPick], ["new", AnnouncementsCompose, newPick]])`, with both views sharing the `{ onBack }` props.
- Also parametrize `AnnouncementsScreen.test.tsx`. Its "offers the birthday greeting" test passes in the new view only because jsdom keeps closed `<details>` content in the accessibility tree (E14).

**E12 · MEDIUM: the e2e needs its own Playwright project or `pnpm e2e` picks it up.**
Evidence: the `chromium` project ignores `config-setup.flag.spec.ts` explicitly (`playwright.config.ts:64-65`) and that spec has a dedicated project (`:79-86`) and script (`package.json:17`). The plan only says "como config-setup.flag.spec.ts". From memory, `globalSetup` wipes the DB for every project unless `E2E_SKIP_RESET=1` (`e2e-demo-project-wipes-db.md`).
Amendment: add a project `announcements-view`, a `testIgnore` entry in `chromium`, and a script `e2e:announcements-view`. The spec sends to **one pupil** only, because a local send fans out through comms. Assert the success `role=status` text with the flag both ON and OFF.

**E14 · LOW: a closed `<details>` hides the A5 load error in a real browser, and jsdom cannot see that.**
Evidence: `BirthdayGreeting.tsx:118` renders the load-failure `role=alert`. Inside a closed `<details>` it is mounted but not visible. jsdom does not render `<details>`, so the role queries still find it.
Amendment (auto-chosen, conservative and with no change to `BirthdayGreeting`): accept it, and record it in section 4's map as "A5 error lives inside the collapsed panel". The `AutomaticMessages` test asserts the component is mounted and the summary is visible. The visible `/qa` checks that opening the panel shows the error. If the owner wants the error visible while collapsed, that needs state from `BirthdayGreeting` and becomes a BC item.

**E11 · MEDIUM: flag registration is 21 → 22, with concrete test edits.**
Evidence: the `AllModuleFlags` count of 21 was verified at `flags.go:15-47`. `flags_test.go:46-53` holds the hard-coded 21 and its comment, the presentation table is at `flags_test.go:117-127`, and the features test is at `features_test.go:54-72`.
Amendment:
- Add `featureflag.Key("announcements_view")` with a comment in the `config_setup` style.
- `flags_test.go`: change 21 to 22 and update the comment.
- Add the table row `{key: "module_announcements_view", baseline: "comms", enabling: "comms,announcements_view"}`.
- Table-drive `TestFeaturesReportsConfigSetupOff` over `{config_setup: "config", announcements_view: "comms"}` instead of copying it.
- Leave `TestEveryGuardedModuleIsReportedByFeatures` (`flags_registry_test.go:18`) alone; it does not apply because the flag guards no route.
- Run it with `cd services/gateway && go test ./internal/config/... ./internal/handler/...`. `./...` from the root does not work.
- The `FLAGS.md` row must state "**no** extra reads" (unlike `config_setup`, which documents three extra reads). E4 is the test behind that claim.
- Terraform and `docker-compose.yml` are not touched, because the flag defaults to OFF. Say this explicitly. Enabling it in dev is a gateway deploy after the gates.

### Test diagram

```
                      behavior.md (frozen)
                              │
         ┌────────────────────┴────────────────────┐
  describe.each([old, new]) × {behavior.test, screen.test}
   A0 calls exact (E4)  A1 A6 B1 B2 B4 B5* B6 B7 D1 D2 D3 D4×4 D5×3 D6 F1  Enter-submits (E3)
   single submit/alert/status node (E2)        *pickStudent per view (E5)
         │                                          │
   unit (new, red first)                      AppShell (E16)
   audience.test (bucket + Otros + flat)       comms×view: 3 cases
   labelGroups additive (omitted when unknown)
   AudienceTabs aria-pressed · StudentSearch (fold, pinned, ≤8, count, aria-label)
   ComposeSummary (live preview, "Va a", 1 button) · AutomaticMessages (closed, mounted)
         │
   gateway: flags_test (22, table row) · features_test (table-driven)
         │
   e2e announcements-view project (ON/OFF, one pupil)  →  /qa visible, after/
```

## Section 4: Performance

**E15 · LOW.** `StudentSearch` folds about 401 names on every keystroke. That is O(n), well under 1 ms, but `useMemo` the folded names keyed on `students`. `sortStudents` already runs on every render today (`AnnouncementsScreen.tsx:119`), so nothing gets worse. There are no new requests (E4 guards that).

## Failure modes

| Failure | Test catches it? | Visible to user? | Critical gap |
|---|---|---|---|
| Enter no longer submits (aside outside the form) | **No**, and the probe proved it (E3) | Yes, silently | **yes** |
| New view adds or repeats a fetch | **No** (E4) | No, it just costs load | **yes** |
| Duplicate summary node | Yes, loudly (E2) | Double announcement | no |
| View flag bypasses `module_comms` | Only with E16's test | Yes | no |
| `FlagKey` missing | `tsc` (E1) | Build fails | no |

Failure modes: 2 critical gaps flagged (E3, E4).

## NOT in scope

BC1 to BC5 (behavior changes, the owner decides), the phone layout, `comms/api.ts`, `BirthdayGreeting.tsx` internals, any `.proto`, any Go service beyond the flag registry.

## Outside voice

Skipped: this was a spawned, non-interactive run with no external reviewer invoked. It is not counted as outside coverage.

## Implementation tasks

| ID | P | Task | Source |
|---|---|---|---|
| T1 | P1 | `FlagKey` + gateway `announcements_view`, 22, table-driven tests, `FLAGS.md` row stating no extra reads | E1, E11 |
| T2 | P1 | Characterization additions green on the old view: A0 exact calls, Enter submits, B5 view-independent + `pickStudent` seam | E4, E3, E5 |
| T3 | P1 | Extract `useAnnouncementComposer` (`chooseScope`, `toggle`, `submit`) under green | E10 |
| T4 | P2 | `labelGroups` additive `levelId`/`ordinal`/`gradeName`, omitted when unknown; full web suite | E7, E8 |
| T5 | P2 | `audience.ts` on `sortBySchoolOrder` + buckets, flat when no level | E6 |
| T6 | P2 | Components; `<form>` is the grid; one `ComposeSummary`; `TextArea rows={4}`; hint wording | E2, E3, E9, E13 |
| T7 | P2 | AppShell nested ternary + 3-case test | E16 |
| T8 | P3 | e2e project/script/testIgnore, one-pupil send | E12 |
| T9 | P3 | Record the A5-in-collapsed-panel note in plan section 4 | E14 |

## Completion summary

- Step 0: scope accepted as-is (FULL_REVIEW)
- Architecture: 5 issues: E1, E2, E3, E10, E16
- Code quality: 5 issues: E6, E7, E8, E9, E13
- Tests: 5 issues: E4, E5, E11, E12, E14
- Performance: 1 issue: E15
- Total: **16 findings (4 HIGH · 8 MEDIUM · 4 LOW)**. Blockers before coding: E1, E2, E3, E4.
- Failure modes: 2 critical gaps flagged
- Outside voice: skipped
- Unresolved decisions: 0 (every choice auto-resolved to the recommended option and recorded above)

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Outside Review | none | Independent 2nd opinion | 0 | skipped | — |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 1 | ISSUES OPEN | 16 issues, 2 critical gaps |
| Design Review | `/plan-design-review` | UI/UX gaps | 1 | issues_found | score: 4/10 → 8/10 (logged concurrently, 3 unresolved) |
| DX Review | `/plan-devex-review` | Developer experience gaps | 0 | — | — |

- **OUTSIDE COVERAGE:** none. The outside voice was skipped in this spawned run.
- **VERDICT:** no review CLEAR. The issues are mapped to amendments T1 to T9; apply E1 to E4 before implementation. Eng review required (re-run after the amendments).

**UNRESOLVED DECISIONS:**
- None from this eng review; + 3 unresolved from prior reviews (plan-design-review, see PLAN-redesign-avisos.design-review.md)
