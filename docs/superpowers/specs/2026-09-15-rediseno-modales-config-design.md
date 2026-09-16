# Rediseño del sistema de modales — Configuración primero

**Fecha:** 2026-09-15 · **Rama:** `feature/rediseno-modales` · **Estado:** diseño aprobado por el dueño

**Evidencia:** `.claude/docs/design/RESEARCH-modales-2026.md` (33 fuentes: Carbon, Polaris,
Material 3, GOV.UK, Cloudscape, Primer, NN/g, Baymard, W3C APG, WCAG 2.2). Copia global en
`~/.claude/reference/research-modals-forms-2026.md`; regla operativa global en
`~/.claude/rules/ui-forms-and-modals.md`.

---

## 1. El problema

La captura de Configuración se siente amontonada. No es percepción: son cuatro defectos, y
tres viven en el código.

1. **`column-count: 2` en el cuerpo del cajón grande** (`globals.css:308-318`). El corte
   entre columnas cae donde alcanza la altura, no donde termina una idea: al agregar un
   concepto de inscripción, "Cobranza" brinca de columna sola y la forma se reacomoda bajo
   las manos de quien captura. Baymard documenta que el layout multicolumna provoca campos
   obligatorios omitidos y campos innecesarios llenados; dos columnas "nunca se recomienda".
2. **El error aparece lejos del campo que lo causó.** En el tabulador, el mensaje de la
   sección Inscripción se dibuja al final del cuerpo, en la otra columna.
3. **Cinco secciones y catorce campos con el mismo peso visual.** Nada distingue lo
   obligatorio de lo opcional, y casi todo es opcional.
4. **Todo cajón mide 100 vh.** Un alta de tres campos flota arriba de un metro de blanco con
   el botón de guardar clavado hasta abajo.

Causa raíz común: **un feriado de 3 campos y un tabulador de 14 usan exactamente el mismo
envase.**

## 2. La decisión

El envase lo elige el **número de campos**, no la importancia del registro.

| Superficie | Campos | Envase | Hoy |
|---|---|---|---|
| Materia | 1 | Chip en el lugar | Cajón |
| Grupo | 2 (nombre, cupo) | Chip en el lugar | Cajón |
| Grado | 2 (nombre, orden→auto) | Chip en el lugar | Cajón |
| Actividad | 3 | Renglón en tabla | Cajón |
| Día cerrado | 4 (tipo→Feriado) | Renglón en tabla | Cajón |
| Periodo | 4 | Renglón en tabla | Cajón |
| Ciclo escolar | 3 | Cajón compacto | Cajón |
| Nivel educativo | 3 | Cajón compacto | Cajón |
| Tabulador de precios | 14 en 5 secciones | **Página** | Cajón `lg` |

**Excepción declarada:** *una decisión que necesita explicarse sube un envase.* Ciclo y Nivel
tienen 3 campos y aun así van a cajón, porque activar un ciclo pausa el anterior y elegir
"por periodo" cambia cómo se inscribe el alumno todo el año. Esos párrafos explicativos ya
existen en la UI actual, son correctos, y no caben en un renglón.

**"En el lugar" tiene dos formas.** Días cerrados, actividades y periodos ya se listan como
tablas → renglón editable al pie. Niveles, grados, materias y grupos se listan como *chips*
→ un chip vacío que se convierte en input al hacer clic. No se convierten chips en tablas.

**Un chip captura un solo valor: el nombre.** Donde el registro trae un segundo campo, ese
campo nace con un default y se edita después desde el propio renglón/ficha del registro:
Grado → `orden` = el siguiente disponible; Grupo → `cupo` = el del último grupo creado en
ese grado, o 25 si es el primero. Si algún día un alta de chip necesitara un tercer campo,
esa alta ya no es un chip: sube a cajón compacto.

## 3. El tabulador como página

Una sola página, un solo hilo. **Sin pasos.**

- Una columna. Secciones numeradas y apiladas: 1 Colegiatura · 2 Inscripción ·
  3 Cobranza · 4 Cuotas.
- Cobranza y Cuotas **nacen plegadas**, mostrando su estado en una línea
  ("recargo fijo · sin pronto pago", "ninguna"). Divulgación progresiva.
- **Riel derecho pegajoso con el año completo en vivo**: inscripción, colegiatura × N,
  cuotas, total, y la diferencia contra el ciclo anterior. Se recalcula al teclear.
- Barra inferior pegajosa: Cancelar · Guardar tabulador.
- Migaja `Configuración › Colegiaturas ›` y título `Precios de <Nivel> · <Ciclo>`.

Se descartaron los pasos: cobran navegación justo en el caso normal del dueño ("copiar y
subir el monto") y separan el total del año de los números que lo producen. Los pasos sirven
cuando cada pregunta es independiente; aquí las cuatro secciones alimentan un mismo
resultado.

**Funcionalidad conservada íntegra:** los 14 campos, los conceptos de inscripción con
tramos de vigencia, el cálculo de totales, la validación de escalera (`ladderProblem`), las
cuotas con selección de meses y la corrida mensual. Sólo cambia el envase y el orden.

## 4. Los cinco mecanismos de velocidad

Ninguno es "apretar más el formulario". La regla de los 3 clics está desmentida (NN/g); la
velocidad se compra con defaults, contexto y menos decisiones.

1. **Copiar del ciclo anterior** (tabulador). Verificado: no requiere backend nuevo —
   `listTuitionPlans(baseURL, cycleId)` ya recibe el ciclo. Los **montos se copian tal
   cual**; las **fechas se recorren exactamente un año** — misma fecha, año + 1 — lo que
   aplica a las vigencias de cada tramo y a la fecha límite de inscripción. Si la fecha
   resultante queda fuera del ciclo destino, se copia vacía y el campo se marca para que la
   directora la capture: mejor un hueco visible que una fecha silenciosamente equivocada.
   Caso normal: Copiar → Mensual → escribir → Guardar = **3 clics**.
2. **Enter guarda y reabre** en renglón y chip. Doce feriados sin soltar el teclado.
3. **"Guardar y crear otro"** en los dos cajones restantes: conserva el contexto
   (ciclo/nivel), limpia el resto, devuelve el foco al primer campo.
4. **Defaults**: tipo = Feriado · orden = el siguiente · fechas acotadas al ciclo activo ·
   nivel = el que venía seleccionado.
5. **Borrar con Deshacer** (toast ≥ 8 s) en vez de preguntar. Hoy "Quitar" borra en seco sin
   red; el toast agrega seguridad **sin costar un clic**.

**No-objetivo declarado:** "copiar y subir un %". Aplicar un porcentaje sobre conceptos con
tramos obliga a decidir redondeo sobre dinero real; merece su propia decisión y sus propias
pruebas. Fuera de este trabajo.

## 5. Arquitectura

Todo vive en `apps/web/app/_design/`; las pantallas consumen. Se respeta la dirección de
dependencias existente y los tokens de `DESIGN-SYSTEM.md` — **cero colores, tipografías o
sombras nuevas**.

**Nuevo**

- `InlineCreateRow` — renglón editable al pie de una tabla. Recibe descriptores de columna,
  `onCreate`, y el contrato de teclado: Enter guarda y reabre, Esc cancela, el error se
  dibuja **en la celda**.
- `InlineCreateChip` — chip vacío que se vuelve input; mismo contrato de teclado.
- `CreatePage` — cáscara de página de alta: migaja, título, cuerpo de una columna, riel
  derecho pegajoso y barra inferior pegajosa.

**Se corrige lo existente (13 pantallas lo heredan)**

- `DrawerShell`: contrato APG completo — trampa de foco, foco inicial en el primer campo,
  **retorno de foco al elemento que lo abrió** (hoy no existe ninguno de los tres); y el pie
  pegajoso deja de poder cubrir el campo enfocado (WCAG 2.2 · 2.4.11).
- `DrawerShell`: la altura es la del contenido, con techo de 85 vh y scroll sólo en el
  cuerpo. Muere el metro de blanco.
- `SlideOver`: gana el secundario "Guardar y crear otro".
- `Toast`: gana acción, para el borrado con Deshacer.
- `useRequired`: **se conserva su decisión actual** — un campo obligatorio **vacío** se
  reclama al guardar, nunca al salir del campo. El código ya lo razonó y tiene razón:
  *"reclamar campo por campo al salir de cada uno persigue con rojos a quien apenas va
  llegando al siguiente"*. Lo que sí se agrega es validación en `blur` **sólo para datos
  mal escritos** en un campo que la persona ya tocó (fecha imposible, monto negativo, fin
  antes del inicio) — que es de lo que habla el hallazgo de Baymard. El error se limpia por
  tecla en cuanto el valor vuelve a ser válido. Nunca se valida mientras se teclea.

**Se borra**

- El bloque `column-count: 2` de `globals.css`. Verificado quién depende de él: sólo
  `TuitionPanel`, que se va a página. `CollectDrawer` también es `lg` pero usa cuerpo
  desnudo (`drawer-body-bare`), así que la regla nunca le aplicó. **El ancho `.drawer-lg` se
  conserva** — lo necesita `CollectDrawer`.

## 6. Pruebas (TDD estricto: test que falla primero, sin excepción)

- **Contrato de fuente**, extendiendo `drawer-contract.test.ts`: nadie dibuja su propio
  diálogo, y `column-count` no puede reaparecer en CSS de formularios.
- **Contrato de teclado del cajón**, una sola vez sobre la cáscara, heredado por todas las
  pantallas: foco inicial dentro, ciclo de Tab atrapado, **retorno de foco al disparador**,
  Esc cierra (pregunta si hay cambios), Enter envía, Cmd/Ctrl+Enter envía desde cualquier
  campo, y el pie no cubre el campo enfocado.
- **Nunca se deshabilita el primario**: enviar una forma incompleta produce mensaje y mueve
  el foco al primer campo con error.
- `InlineCreateRow` / `InlineCreateChip`: Enter guarda y reabre; Esc cancela sin guardar; el
  error sale en la celda.
- Tabulador: copiar mapea todos los campos, **recorre las fechas un año** y deja los montos
  intactos; el total del riel se recalcula al teclear; `ladderProblem` sigue vigente.
- Borrado: el toast de Deshacer restituye el registro.
- E2E Playwright: se actualizan los specs de Configuración a los nuevos envases.

## 7. Orden de entrega

1. La cáscara (`DrawerShell` foco + altura, `SlideOver`, `Toast`, `useRequired`). Primero
   porque 13 pantallas lo heredan gratis.
2. `InlineCreateRow` + `InlineCreateChip` con sus pruebas.
3. Configuración, panel por panel, del más chico al más grande.
4. El tabulador a página.
5. Las reglas del proyecto en `.claude/docs/design/`, apuntando a la regla global.

## 8. Alcance y decisiones abiertas

- **Sin flag nuevo.** Esto es refactor de superficies existentes, no un módulo nuevo; viaja
  bajo el flag de `config` que ya existe. El dueño puede pedir un flag propio si quiere
  retroceso en vivo durante una demo.
- **Las otras 10 pantallas no se convierten en esta etapa.** Heredan foco y altura del cajón
  sin cambios propios; pasarlas a renglón en línea es una etapa posterior, con la regla ya
  escrita.
- **Sin modo oscuro**, sin translucidez ni blur detrás de formularios: superficie sólida,
  borde definido, sombra discreta.
