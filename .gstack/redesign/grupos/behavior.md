# Behavior inventory: Configuración académica, steps 3 "Niveles y grados" and 6 "Grupos"

Behavior is frozen. All paths are under `apps/web/app/`. API functions live in `config/api.ts`, and every route starts with `/api/v1/config`.

## 1. Components and who renders them

- **Which view**: `AppShell.tsx` uses `useFlag("module_config_setup")`. With the flag ON it renders `<CatalogSetup>` (`_screens/config/setup/CatalogSetup.tsx`); with it OFF it renders the legacy `<Catalog>` (`_screens/config/Catalog.tsx`). Both get the same props: `canWriteConfig` (`config:write`), `canDeleteLevels` (`levels:delete`), `onDeleteLevel` (goes to the `levelDelete` screen), `notice`, `canReadTuition`. The permission helpers are in `config/permissions.ts`; an unknown permission list counts as false.
- **Shared state**: `useCatalogState.ts` (with `useLevelReorder.ts`) is used by both views. The setup view adds `setup/useSetupStatus.ts` (reads every grade, plus every group of the cycle) and `setup/useSetupActions.ts` (each handler, then a status refresh).
- **Step routing**: `setup/SetupPanes.tsx` mounts every step and hides the closed ones; they are never unmounted. Each step sits in `setup/StepFrame.tsx`, which draws "Paso N de 8" and the footer with Anterior/Siguiente.
- **Step 3**: `setup/steps/StepLevels.tsx` contains:
  - `LevelsPanel` (`showLevelSettings={false}`)
  - `GradeLevelsPanel`
  - `setup/steps/GradeGroups.tsx` (read-only group names plus a link)
  - `LevelSettings`, exported from `LevelsPanel.tsx`: `AttendanceModeControl`, `GradingControl` (with `useSerialGrading.ts` and `gradingDraft.ts`) and `DeleteLevelButton`
  - `LevelOrder.tsx`, drawn inside `LevelsPanel`
  - Grade chips are wrapped in `CorrectableName.tsx` plus `useDeletionFeedback.tsx`
- **Step 6**: `setup/steps/StepGroups.tsx` contains `Prerequisite.tsx` (`needFor`), `setup/LevelGradeChips.tsx` and `GroupsPanel` (with `CorrectableName`).
- **Legacy `Catalog.tsx`** uses the same `LevelsPanel` (with level settings under the chips), `GradeLevelsPanel` and `GroupsPanel`. It has no `GradeGroups`, `LevelGradeChips` or `Prerequisite`.
- **What changes in the setup view**: it is decided by `SetupPresentation.tsx` (`inSetup`):
  - Level chip shows the name only (adds "· por periodo" when it applies); legacy shows "NAME · por ciclo".
  - Grade chip shows the name; legacy shows "1. name".
  - Grade subtitle differs.
  - Guardar calificación is outline instead of lime.
  - Eliminar nivel is red text with a rule above it instead of a red pill.
- **Shared building blocks**: `_design/SlideOver.tsx` (level drawer) and `_design/InlineCreateRow.tsx` (grade and group rows).
- **Level deletion page**: `levelDeletion/LevelDeleteScreen.tsx`, `useLevelDeletion.ts`, `steps.tsx`. Papelera: `levelDeletion/LevelTrashPanel.tsx`.

## 2. User actions

### Step 3 · Niveles y grados

| Action | API / route | Validation and messages | Permission | Success / failure / focus |
|---|---|---|---|---|
| Select a level chip | `listGradeLevels` GET `/grade-levels?levelId=` | — | all | Clears the selected grade and loads its grades. A load error is swallowed silently. A school with one level has it picked automatically. |
| Nuevo nivel (opens SlideOver "Nuevo nivel educativo") | `createLevel` POST `/levels` `{name,sepCode,enrollmentGranularity}` | Nombre is required and upper-cased; empty gives "Un nivel necesita nombre." with no call. Clave SEP is optional, upper-cased and trimmed. Inscripción radio: Por ciclo (`cycle`, default) or Por periodo (`term`). | Button only when `canWriteConfig` | "Crear nivel" closes the drawer. "Guardar y crear otro" resets the fields and focuses `[role=dialog] input`. The new level is appended and selected, then the grades status is refreshed. Failure: "No se pudo crear el nivel. Revisa que no exista ya con ese nombre." and the drawer stays open. Buttons ignore clicks while pending. Esc or Cancelar closes. |
| Subir / Bajar (LevelOrder, "Orden de los niveles", only with more than 1 level) | `reorderLevels` PUT `/levels/order` `{levelIds}` (the whole order) | First level has no Subir, last has no Bajar (absent, not disabled) | `canWriteConfig` | Moves on screen at once. Moves are queued and sent one at a time. A refusal puts back the last confirmed order and shows the server's detail (400/409) or "No se pudo cambiar el orden de los niveles. Intenta de nuevo." Focus stays on the moved level's same arrow, or its other arrow at an edge. |
| Attendance radio "Asistencia de X" (Por día / Por clase) | `updateLevelAttendanceMode` PATCH `/levels/{id}` `{attendanceMode}` | Picking the current mode sends nothing. A level that does not say is read as daily. | **Not gated** | Toast "X: asistencia por día/clase" (2.6 s). Failure: "No se pudo cambiar la asistencia del nivel. Reintenta." |
| "Lleva calificación numérica" checkbox, "Califica con letras" (only when numeric is off), scale form (min, max, Mínimo aprobatorio, Guardar calificación) | `updateLevelGrading` PATCH `/levels/{id}` `{grading}` | Whole numbers only; 0 ≤ min < passing ≤ max ≤ 100. Errors show on blur and on submit, and focus goes to the first invalid field. Default 5–10, passing 6. Turning numbers on turns letters off. | **Not gated** | One queue for both toggles and the scale save. Toast "X: calificación guardada". `SCALE_TIGHTENING` puts the saved scale back and shows the server's detail. Other failures: "No se pudo guardar la calificación del nivel: …" and the switch goes back to what the server holds. |
| Eliminar nivel | Navigates to `levelDelete`, then `getLevelDeletionPreview` GET `/levels/{id}/deletion-preview` and `deleteLevel` POST `/levels/{id}/delete` `{confirmName,confirmToken,password}` | Three steps: Resumen, Confirmar nombre, Contraseña. Errors handled: `NAME_MISMATCH`, `REAUTH_FAILED` ("Contraseña incorrecta."), `PREVIEW_STALE` (starts over), 403, 404, `DELETION_IN_PROGRESS`. | Owner only (`levels:delete`) | Returns to config with the notice toast (8 s): "Nivel eliminado. Puedes restaurarlo desde la Papelera hasta el …" |
| Select a grade chip | Groups: `listGroups` GET `/groups?cycleId&gradeLevelId` | — | all | Shows `GradeGroups`. It draws nothing until the cycle-groups read is ready. Text is "Grupos de G: A · B" or "G aún no tiene grupos." |
| "Editar grupos de G" / "Crear grupos de G" link | none, `onGo("groups")` | — | all | Opens step 6 with the same grade selected (the selection is shared). Focus goes to the step's h2. |
| Nuevo grado (InlineCreateRow: Nombre del grado, Orden) | `createGradeLevel` POST `/grade-levels` `{levelId,name,ordinal}` | Name required and upper-cased ("Falta Nombre del grado." in the cell, focus there). Orden defaults to max+1 and must be ≥1: "El orden del grado tiene que ser un número mayor que cero." | **Not gated** | Enter saves. The row resets and focuses the first cell, ready for the next grade (save-and-new by design). The grade is appended but not selected; grades status refreshed. Failure: "No se pudo crear el grado. Revisa que no exista ya en este nivel con ese nombre o ese orden." and the typed values are kept. Esc cancels. Double Enter is guarded. |
| Rename grade (pencil, "Renombrar grado X") | `renameGradeLevel` PATCH `/grade-levels/{id}` `{name}` | Input is focused and upper-cased, max length 80. Empty: "Escribe el nombre del grado." An unchanged name closes the editor with no call. | `canWriteConfig` | Enter or Guardar saves ("Guardando…"). Esc or Cancelar closes. Errors 409/400 show the server's detail; otherwise "No se pudo renombrar el grado X. Intenta de nuevo." Focus goes back to the input on error. |
| Delete grade (trash, "Borrar grado X") | `deleteGradeLevel` DELETE `/grade-levels/{id}` | ConfirmDialog "¿Borrar el grado X?" with the description and "No se puede deshacer." | `canWriteConfig` | The grade is removed and deselected. Status region: "Grado X borrado". Focus goes to the next row's pencil, else the previous one, else the create control. 409 `GRADE_LEVEL_IN_USE` shows the server's detail inline. |

### Step 6 · Grupos

| Action | API / route | Validation and messages | Permission | Success / failure / focus |
|---|---|---|---|---|
| Level and grade chips (LevelGradeChips, groups "Nivel" and "Grado") | as in step 3 | Grades sorted by ordinal. Pill "sin grupos" / "1 grupo" / "N grupos" once the cycle groups are read. | all | Shared selection with step 3. |
| Nuevo grupo (Nombre del grupo, Cupo) | `createGroup` POST `/groups` `{cycleId,gradeLevelId,name,capacity}` | Name defaults to the next letter A, B… (capped at Z) and is upper-cased. Cupo defaults to 30 and must be ≥1: "El cupo del grupo tiene que ser un número mayor que cero." | **Not gated** | Enter saves, the row resets and re-proposes the next letter. Groups status refreshed. Failure: "No se pudo crear el grupo. Revisa que no exista ya con ese nombre." Help text explains what cupo decides. |
| Rename group | `renameGroup` PATCH `/groups/{id}` | Same as grade | `canWriteConfig` | 409 `GROUP_NAME_TAKEN` and 503 `GROUP_RELABEL_PENDING` show the server's detail. A refusal re-reads the list first. Saving the same name again after a refusal is still sent. |
| Delete group | `deleteGroup` DELETE `/groups/{id}` | Confirm "¿Borrar el grupo X?" | `canWriteConfig` | Status "Grupo X borrado", same focus rules as grades, and the grade's pill count follows. 409 `GROUP_IN_USE` lists what keeps it. |
| Pills | — | "A · 30 lugares" | — | Read-only without write. |

There is no in-place edit of cupo, no level rename and no grade reorder on either step.

**Step navigation:** "← {prev title}" (aria-label "Anterior: …") and "Siguiente: {next title} →". In step 3 Siguiente is a text link (the step has its own lime button). In step 6 Siguiente is the lime button. Opening a step moves focus to its h2 and announces it (aria-live), and the open step is kept in sessionStorage.

## 3. States

- **Loading / error (whole screen)**: "Cargando catálogo…" while loading. If boot fails (`listCycles` + `listLevels`), it shows `role=alert` "No se pudo cargar el catálogo." and no steps.
- **Step 3, no levels**: "Aún no hay niveles. Agrega el primero." No level is selected, so there is no grades pane and no settings.
- **Step 3, level without grades**: "Aún no hay grados en este nivel."
- **Step 3, cycle**: no cycle prerequisite. `GradeGroups` stays hidden while the groups read is loading or failed, or when there is no cycle.
- **Step 6, no cycle**: "Primero crea el ciclo escolar." plus "Ir al paso 2".
- **Step 6, no levels**: "Primero agrega un nivel." plus "Ir al paso 3".
- **Step 6, selected level has no grades**: "Primero agrega un grado." plus "Ir al paso 3".
- **Step 6, no grade picked**: "Elige un grado para ver sus grupos."
- **Step 6, grade without groups**: "Aún no hay grupos para este grado en el ciclo."
- **Step 6, cycle requirement**: groups need the selected cycle, whatever its status (draft, active or closed). The UI has no status check.
- **Step-list lines**:
  - Step 3: "Sin niveles", "Faltan grados en X (y N más)", or "N niveles · M grados".
  - Step 6: "Primero crea el ciclo escolar", "Primero agrega un nivel", "Primero agrega un grado", "Sin grupos en G de L…", or "N grupos".
  - "Revisando…" while a read is in flight; no line when it failed.
- **Read-only (no `config:write`)**:
  - Hidden: Nuevo nivel, Subir/Bajar, and the pencil and trash on grades and groups.
  - **Still shown (gateway 403 would end in the generic create error)**: the Nuevo grado and Nuevo grupo rows, the attendance radio and the grading controls.
  - The card is titled "Configuración" instead of "Puesta en marcha".
- **Delete permission**: without `levels:delete` there is no Eliminar nivel and no Papelera.

## 4. Test coverage

All tests are in `_screens/config/` unless a path says otherwise.

- **Level create**: `LevelsPanel.characterization.test.tsx` (empty, chip label, name required, Por periodo, refused create); `setup/CatalogSetup.actions.test.tsx` A38–A40 (Guardar y crear otro, Crear nivel); `Catalog.test.tsx` (no SEP key, creating several in a row, Esc, error in the field); `Catalog.characterization.test.tsx` (create selects the level); `setup/CatalogSetup.readOnly.test.tsx` (no Nuevo nivel).
- **Reorder**: `LevelOrder.test.tsx`, `useLevelReorder.test.ts`, `setup/CatalogSetup.corrections.test.tsx` (school order, rollback), `Catalog.corrections.test.tsx`.
- **Attendance**: `LevelsPanel.test.tsx` (toast, failure); `CatalogSetup.actions.test.tsx` A34–A36; `Catalog.test.tsx`.
- **Grading**: `GradingControl.test.tsx` (queue, letters, SCALE_TIGHTENING); `gradingDraft.test.ts`; `CatalogSetup.actions` A34–A36.
- **Delete level**: `LevelsPanel.test.tsx`, `CatalogSetup.actions` A37, `Catalog.deletion.test.tsx`, `levelDeletion/LevelDeleteScreen.test.tsx`, `LevelTrashPanel.test.tsx`, `setup/CatalogSetup.test.tsx` (notice).
- **Grades**: `GradeLevelsPanel.characterization.test.tsx` (ordinal default, ordinal ≥1, refusal); `CatalogSetup.actions` A41/A42; `CatalogSetup.corrections` (rename, delete, in use); `CorrectableName.test.tsx`; `CorrectableName.deletionFocus.test.tsx`; `SetupPresentation.test.tsx` (chip labels, Eliminar nivel styling).
- **Groups**: `GroupsPanel.characterization.test.tsx` (A/30 defaults, next letter, cupo ≥1, refusal, help text); `CatalogSetup.corrections` (rename taken, delete with count, in use); `useCatalogState.corrections.test.ts` (refused rename re-reads); `CatalogSetup.test.tsx` (re-reads only groups after create; Grupos chips and pills; prerequisite links; Editar/Crear grupos links); `LevelGradeChips.test.tsx`.
- **Navigation and status**: `setup/StepFrame.test.tsx`, `CatalogSetup.test.tsx` (focus to h2, Siguiente, lime Siguiente in Grupos), `setup/setupSteps.test.ts` (lines and prerequisites), `useCatalogState.singleLevel.test.ts`, `AppShell.configSetup.test.tsx` (flag).

### Behaviors with no test coverage

1. With `canWriteConfig=false`, Nuevo grado, Nuevo grupo, attendance and grading are still offered. No test asserts this either way.
2. LevelOrder's generic fallback message ("No se pudo cambiar el orden…") for a non-ConfigError failure.
3. In setup, Clave SEP being sent upper-cased and trimmed when filled in. Only the empty SEP case is tested.
4. In the setup view: Esc or Cancelar on the level drawer, and focus going to the first field after "Guardar y crear otro". These are covered in legacy `Catalog.test` only, as far as I could tell.
5. The "Primero agrega un grado." prerequisite as rendered in step 6. Only the step-list line is tested in `setupSteps.test`.
6. `GradeGroups` hidden while the groups read is loading or failed, or when there is no cycle.
7. The next group letter capping at "Z" after 25 groups.
8. A grade or group create error shown inside the setup view. Only tested at panel level.
9. Silent failures of `listGradeLevels` and `listGroups` when changing selection.
10. In setup, picking another level clears the selected grade (step 3 grades pane, step 6 GroupsPanel). Legacy only (`Catalog.characterization`).
11. Group create with a closed cycle selected. There is no UI rule for it.
12. A rename that leaves the name unchanged closing with no call. I didn't find a test for it.