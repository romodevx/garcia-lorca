# ADR 0039: Calificaciones con letras para Preescolar

- Status: Accepted
- Date: 2026-10-20
- Context: Preescolar se calificaba con letras en el colegio, pero el sistema la dejaba sin calificaciones. `earlyYearsLevelNames` apagaba la captura numérica, y nada la reemplazaba. No se podía capturar ni cerrar el periodo, y nunca se emitía su boleta. Toda la cadena trataba la calificación como número: el CHECK de `period_grades.value`, `decodeValues` y los `json.Number` del gateway.

## Decisión (del dueño, 20-oct)

1. **Letras:** A = Excelente, B = Bien, C = Suficiente, D = Requiere apoyo. Son las mismas para todo el colegio.
2. **Por materia y periodo**, igual que las numéricas.
3. **Salen en la boleta y en la app de padres.**
4. **Maternal sigue sin calificaciones.** Solo Preescolar (y Kínder) usa letras.
5. **Sin feature flag nuevo:** amplía `module_grades`.

## Cómo

- **config:**
  - `GradingScale.letters` sigue el patrón `With…` (`WithLetters`). Nunca va junto con `enabled`; el dominio y un CHECK lo exigen.
  - Migración 0024: `levels.grading_letters` y el backfill de los niveles preescolar y kínder.
  - Proto: `GradingScale.letters = 6`.
- **grades:**
  - `domain.Letter` (A–D) con `Meaning()`. `PeriodGrade` lleva un número **o** una letra. `Text()` da "9" o "B", y así la letra viaja en el mismo campo de texto del proto (`value`).
  - Un nivel con letras pasa `ParseLetter`. Un número ahí se rechaza con `GRADE_NOT_LETTER`; una letra en un nivel numérico, con `GRADE_NOT_NUMBER`.
  - `LevelRef/LevelCatalog.Graded()` habilita para letras la hoja, la vista de familia, la lista de periodos, el conteo y el cierre.
  - La boleta con letras (`ReportCardHeader.Letters`) guarda `PeriodLetters` y ningún promedio. La boleta numérica ignora cualquier letra, para que nunca cuente como 0.
  - Migración 0011:
    - `period_grades.letter`, con un CHECK que exige exactamente uno: número o letra.
    - `report_cards.grading_letters` es **nullable**: la papelera (ADR 0021) restaura con `jsonb_populate_record`, y ahí una llave ausente vuelve como NULL. NULL se lee como «números».
  - `CountScaleViolations`:
    - al pasar a números, cuenta las letras ya capturadas;
    - al pasar a letras, cuenta los números ya capturados.
- **gateway:**
  - Campos **nuevos** junto a los numéricos, sin romper a quien solo lee números:
    - `letter` en la celda y en la fila de la hoja;
    - `gradingLetters` en la hoja, la familia y las boletas;
    - `letters` en las materias de la familia y en las líneas de la boleta.
  - El PUT acepta una cadena con una letra A–D. Cualquier otra cadena sigue siendo `GRADE_NOT_NUMBER`.
  - El PDF imprime las letras, la leyenda de su significado y ninguna columna de promedio.

## Consecuencias

- Las letras no se promedian en ninguna parte. La hoja muestra cuántos alumnos tienen cada letra en lugar del promedio (web).
- Para cambiar un nivel de números a letras, o al revés, con calificaciones ya capturadas en el ciclo activo, la respuesta es 409 `SCALE_TIGHTENING`. Es la misma regla que cuando se aprieta una escala.
- No hay observaciones escritas por alumno ni letras distintas por nivel. Si el colegio las pide, se agregan aparte.
