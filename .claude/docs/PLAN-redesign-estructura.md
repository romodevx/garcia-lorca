# PLAN — Rediseño «Estructura de la escuela» (niveles, grados y grupos)

- **Origen:** `/redesign` del 20-oct. El dueño dice que crear niveles, grados y grupos «es muy confuso» y eligió la **variante A, Árbol de la escuela** (lienzo: https://claude.ai/artifact/2mTbSBYT42bQDosCQt3Ybi).
- **Regla dura:** no cambia ninguna funcionalidad, endpoint, contrato, dato, permiso ni regla de negocio. El contrato es `.gstack/redesign/grupos/behavior.md`.
- **Estado:** revisado por diseño y por ingeniería (§8). Pendiente de la aprobación del dueño.

## 1. Qué cambia para el usuario

| Hoy | Con el rediseño |
|---|---|
| Niveles y grados en el paso 3; grupos en el paso 6, después de Periodos y Materias | Un solo paso 3, **«Estructura de la escuela»**: niveles, grados y grupos juntos. La puesta en marcha pasa de 8 a **7 pasos** |
| Botones de nivel, clic en un grado, clic otra vez para ver sus grupos | **Todo a la vista**: una tarjeta por nivel, una fila por grado y sus grupos en la fila («A · 30», «B · 30») |
| Cada nivel aparece dos veces (botón y lista «Orden de los niveles») | Cada nivel aparece una vez: Subir y Bajar van en el encabezado de su tarjeta |
| Asistencia, calificación y «Eliminar nivel» alargan el panel | Van en **«Ajustes del nivel»**, un panel lateral (`SlideOver`) que se abre desde la tarjeta |
| Grados como botones con lápiz y bote intercalados | Cada fila es un grado: su nombre con el **lápiz y el bote visibles** (como hoy), sus grupos y «+ Grupo» |
| «Editar grupos» salta al paso 6 | Ya no hace falta: el grupo se crea en la fila de su grado |
| Para saber qué falta: 19 clics | Una fila sin grupos se marca **«Sin grupos»** (texto, no solo color) y el encabezado del paso dice qué falta |

## 2. Mapa del comportamiento (cada acción de `behavior.md` → dónde queda)

| Acción de hoy | Dónde queda | Llamada (sin cambio) |
|---|---|---|
| Nuevo nivel (cajón; «Guardar y crear otro») | Botón «+ Nuevo nivel» al pie del árbol, mismo cajón y mismos campos | `createLevel` |
| Subir / Bajar nivel | Encabezado de cada tarjeta, misma cola y mismo foco | `reorderLevels` |
| Asistencia (por día / por clase) | «Ajustes del nivel», el mismo `AttendanceModeControl` | `updateLevelAttendanceMode` |
| Calificación (números, letras, escala) | «Ajustes del nivel», el mismo `GradingControl` | `updateLevelGrading` |
| Eliminar nivel (solo dueño) | «Ajustes del nivel», el mismo `DeleteLevelButton` y la misma pantalla de 3 pasos | `getLevelDeletionPreview`, `deleteLevel` |
| Nuevo grado (fila en línea, se guarda y deja lista la siguiente) | «+ Grado» al pie de cada tarjeta abre el mismo `InlineCreateRow`, con el orden por defecto en el máximo + 1 | `createGradeLevel` |
| Renombrar o borrar grado | El nombre del grado en su fila, con el mismo `CorrectableName`, foco y avisos | `renameGradeLevel`, `deleteGradeLevel` |
| Nuevo grupo (letra siguiente propuesta, cupo 30) | «+ Grupo» en la fila del grado abre el mismo `InlineCreateRow` ahí mismo | `createGroup` |
| Renombrar o borrar grupo | La pastilla del grupo en su fila, con el mismo `CorrectableName` | `renameGroup`, `deleteGroup` |
| «Editar grupos de G» (lleva al paso 6) | Se retira: el paso 6 ya no existe con el flag | — |
| Requisitos del paso 6 (sin ciclo, sin niveles, sin grados) | Dentro del árbol: sin ciclo, «+ Grupo» se reemplaza por «Primero crea el ciclo escolar» y el enlace al paso 2 | — |
| Anterior / Siguiente, foco en el h2, paso abierto recordado | Igual: `StepFrame` | — |
| Solo lectura (sin `config:write`) | Se ocultan los mismos controles que hoy. Lo que hoy sigue visible, sigue visible (ver §5) | — |

**Las lecturas no cambian.** `useSetupStatus` ya lee todos los grados y todos los grupos del ciclo; el árbol se pinta con esos datos y no agrega llamadas nuevas. Las escrituras usan los handlers de `useSetupActions`, que ya refrescan el estado.

## 3. Archivos

- **Nuevos** (en `apps/web/app/_screens/config/setup/steps/structure/`):
  - `StepStructure.tsx`: el paso.
  - `LevelCard.tsx`: encabezado, Subir/Bajar, «Ajustes» y grados.
  - `GradeRow.tsx`: el nombre del grado, los grupos y «+ Grupo».
  - `LevelSettingsDrawer.tsx`: `SlideOver` con `LevelSettings` tal cual.
  - Más sus tests.
- **Se reusan sin tocar:** `CorrectableName`, `InlineCreateRow`, `SlideOver`, `LevelSettings` (`AttendanceModeControl`, `GradingControl`, `DeleteLevelButton`), el cajón de nuevo nivel de `LevelsPanel`, `useLevelReorder`, `StepFrame`, `Prerequisite`.
- **Se tocan:**
  - `setupSteps.ts`: con el flag, la lista sin `groups`, el paso `levels` titulado «Estructura de la escuela», números del 1 al 7, y la línea de estado que junta grados y grupos.
  - `SetupPanes.tsx`: monta `StepStructure` con el flag.
  - `useCatalogState` / `useSetupActions`: los handlers reciben el `levelId` o el `gradeLevelId` directo, **sin cambiar la selección**, que comparten los pasos 4 y 5. Un guard evita agregar a la lista del nivel seleccionado un registro que es de otro (§8, M1).
- **Sin cambios:** la vista vieja `Catalog.tsx`, que se usa con `module_config_setup` apagado, y los pasos 3 y 6 actuales, que quedan intactos con el flag apagado.

## 4. Flag

- `module_school_structure`, **apagado por defecto**. Encendido muestra el árbol; apagado muestra los pasos 3 y 6 de hoy.
- Se registra en `FLAGS.md`. Es de presentación (web), detrás de `module_config_setup`.

## 5. Tests (TDD: primero el test que falla)

1. **Caracterización antes de rediseñar**, en verde con la pantalla actual, para los huecos de `behavior.md` que sobreviven:
   - #1: sin `config:write` siguen visibles «Nuevo grado», «Nuevo grupo», asistencia y calificación.
   - #3: la Clave SEP se manda en mayúsculas y sin espacios.
   - #4: Esc o Cancelar cierran el cajón, y el foco va al primer campo después de «Guardar y crear otro».
   - #7: la letra del grupo no pasa de Z.
   - #8: el error de crear un grado o un grupo se ve dentro de la puesta en marcha.
   - #12: renombrar sin cambiar el nombre no llama a nada.
2. **Nuevos, uno por pieza:**
   - El árbol pinta niveles, grados y grupos a partir de las lecturas de estado.
   - Una fila sin grupos dice «Sin grupos».
   - «+ Grupo» crea en el grado de esa fila, con la misma llamada y valores por defecto.
   - «+ Grado» crea en el nivel de esa tarjeta.
   - Subir y Bajar en la tarjeta.
   - Los ajustes abren el panel lateral con los mismos controles.
   - Sin ciclo, se muestra el requisito.
   - Solo lectura.
   - Con el flag: 7 pasos, sin «Grupos», y el título nuevo.
3. **Toda la suite actual en verde con el flag apagado y con el flag encendido**, salvo los tests del paso 6, que solo aplican con el flag apagado.
4. `/qa` en el navegador visible con el flag encendido: recorrer cada fila de `behavior.md` y tomar capturas en `after/`.

## 6. Propuestas aparte (cambian comportamiento; no entran aquí)

- **P1 · Plantilla (variante C):** crear grados y grupos A/B de varios niveles con un clic. Necesita plan, tests y revisión propios.
- **P2 · Solo lectura coherente:** ocultar «Nuevo grado», «Nuevo grupo», asistencia y calificación a quien no tiene `config:write`. Hoy se muestran y el servidor los rechaza.
- **P3 · Editar el cupo** de un grupo ya creado, y **renombrar un nivel**. Hoy no se puede ninguna de las dos.

## 7. Estimación

Un día de trabajo, más que lo que decía el borrador por lo que encontró la revisión de ingeniería (§8). Un PR detrás del flag. Al final, capturas de antes y después.

## 8. Ajustes de las revisiones (20-oct)

### Ingeniería (tech-lead)
- **B1. La lista de pasos se lee en 7 lugares.** `STEPS` queda intacto (flag apagado). Se agregan `STRUCTURE_STEPS` y `stepsFor(view)`, que llegan por un `SetupStepsContext` a `StepFrame`, `SetupStepBar`, `useOpenStep`, `Prerequisite`, `deriveSteps` y los fixtures. Una prueba de dirección de dependencias evita que alguien vuelva a importar la constante.
- **B2. Un paso «groups» guardado en `sessionStorage`** se traduce a «levels» cuando el flag está encendido. Lo mismo en `Prerequisite` y en `onGo`. Lleva su test.
- **H1. Parche optimista en `useSetupStatus`** al crear, renombrar o borrar, seguido del refresco. Así «Guardar y crear otro» propone B y luego C sin esperar el GET, y un renglón borrado desaparece en el acto (M2).
- **H2. Si una lectura falla, el árbol conserva lo último bueno** y muestra «Reintentar». «Sin grupos» solo aparece cuando la lectura de grupos está lista.
- **H3. Renombrar un grupo refresca el árbol aunque el servidor lo rechace** (`GROUP_RELABEL_PENDING`).
- **H4. El flag se registra en el gateway:** `flags.go` y sus tests, la unión `FlagKey`, FLAGS.md y el `docker-compose`. No lleva migración. Se lee una sola vez en `AppShell`.
- **M3. Estado del paso fusionado.** Va pendiente si lo está cualquiera de las dos partes, en el orden «Sin niveles», «Faltan grados…», «Primero crea el ciclo» y «Sin grupos…». `ready` sigue exigiendo grupos en cada grado. El encabezado dirá «falta Estructura de la escuela»: es un **cambio de texto**.
- **L2. La lógica de crear se extrae** a `GradeCreateRow` y `GroupCreateRow`, que usan los paneles viejos y el árbol. No se duplica.

### Diseño
- **Visibles y de 44 px:** el lápiz y el bote de cada grado y de cada grupo. Los nombres accesibles dan contexto («Agregar grupo a 2° de PRIMARIA», «Ajustes de PRIMARIA»).
- **Estructura:** un h3 por nivel, la lista de grados y, en cada grado, la lista «Grupos de 2°».
- **Textos de hoy:** las pastillas dicen «A · 30 lugares» y el texto de ayuda del cupo se queda.
- **Foco:** al borrar, el foco pasa al siguiente, luego al anterior, y si no queda ninguno al «+» de esa tarjeta o fila. Al crear, vuelve al primer campo de la misma fila.
- **Vacíos:** una escuela sin niveles muestra «+ Nuevo nivel» como botón principal. Un nivel sin grados dice «Aún no hay grados en este nivel».
- **«+ Nuevo nivel»** va también en el encabezado del paso, no solo al pie.
- **Resumen en cada tarjeta:** «6 grados · 12 grupos · 1 sin grupos · Asistencia por día · Califica 5–10».
- **Lo que falta** se vuelve un enlace a la primera fila «Sin grupos».
- **Una sola fila en línea abierta a la vez.** Al Subir o Bajar se anuncia «PRIMARIA ahora es 3 de 5».
- **Botón principal:** un solo botón lima, «Siguiente». Los «+» son secundarios.
- **Celular (360 px):** la fila del grado en dos líneas, los campos uno debajo de otro, el panel lateral a pantalla completa y sin desplazamiento horizontal.
- **Textos con número de paso:** se revisan los que dicen «paso 3» o «paso 6», porque la numeración cambia.

### Tests que se suman (§5)
- Crear en un nivel o grado no seleccionado no cambia la selección.
- Dos Enter seguidos proponen B y luego C.
- Si la lectura de grupos falla, no aparece «Sin grupos».
- Un paso «groups» guardado abre el paso 3.
- Anterior y Siguiente funcionan en los 7 pasos.
- Un renombre rechazado refresca el árbol.
- El gateway reporta el flag apagado.
- Los grados se muestran en orden.
- Hay foco tras crear y tras borrar.
- Hay capturas en el `/qa` a 360 px.
