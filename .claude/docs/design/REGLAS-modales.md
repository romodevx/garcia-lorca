# Reglas de captura — García Lorca

> La regla general es **global** y se auto-carga en toda sesión:
> `~/.claude/rules/ui-forms-and-modals.md`. La evidencia (33 fuentes: Carbon, Polaris,
> Material 3, GOV.UK, Cloudscape, Primer, NN/g, Baymard, W3C APG, WCAG 2.2) está en
> `.claude/docs/design/RESEARCH-modales-2026.md`.
>
> Aquí sólo va lo específico de este repo. Diseño:
> `docs/superpowers/specs/2026-09-15-rediseno-modales-config-design.md`.

## 1. El envase lo elige el número de campos

| Campos | Envase | Componente |
|---|---|---|
| 1–4 | Renglón al pie de la lista (o forma en el lugar) | `_design/InlineCreateRow` |
| 5–8 | Cajón compacto | `_design/SlideOver` |
| > 8, o con pasos, o que necesita scroll | Página | `_design/CreatePage` |

**Nunca por importancia.** Un feriado de 3 campos y el tabulador de 14 usaban el mismo
cajón de 100 vh, y de ahí salía todo lo que el colegio llamó "amontonado".

**Dos excepciones, las dos declaradas:**

- **Una decisión que necesita explicarse sube un envase.** Ciclo escolar y Nivel educativo
  tienen 3 campos y van en cajón: activar un ciclo pausa el anterior, y elegir "por periodo"
  cambia cómo se inscribe el alumno todo el año. Esos párrafos no caben en un renglón.
- **Un default sustituye una pregunta sólo si el dato se puede corregir en algún lado.**
  El plan mandaba Grado y Grupo al chip con su segundo campo por default. No hay pantalla de
  edición de grados ni de grupos, así que eso habría eliminado la capacidad de intercalar un
  grado y la de abrir un salón más chico. Van en renglón con el valor **propuesto**, no
  desaparecido. Lo mismo con Feriado/Vacaciones.

El chip que se volvía campo (`InlineCreateChip`) se retiró en T13: su único usuario,
Materias, pasó a pedir nombre **y** grados (crea una por grado) y es una forma de dos
campos en el lugar (`_screens/config/SubjectsPanel.tsx`).

## 2. El contrato de teclado

Lo garantiza la cáscara, no cada pantalla. Está probado una vez en
`_design/DrawerShell.test.tsx` y lo heredan las trece pantallas con cajón.

- El foco entra al **primer campo** (no al botón de cerrar) al abrir.
- Tab y Shift+Tab **no salen** del cajón.
- Al cerrar, el foco **vuelve al control que lo abrió**.
- Esc cierra · Enter guarda.
- En renglón y chip: **Enter guarda y deja listo el siguiente**. Esc cancela.
- El pie pegajoso **no puede cubrir** el campo enfocado (WCAG 2.2 · 2.4.11).

## 3. Lo que no se hace

- **`column-count` en una forma.** Hay una prueba de código fuente que lo impide
  (`_design/drawer-contract.test.ts`). Si una forma ya no cabe en una columna, no necesita
  dos: necesita ser una página.
- **Apagar el botón primario** por una forma incompleta. Al enviar se dice qué falta y el
  foco va al primer campo con problema. Sólo se apaga mientras la petición va en camino.
- **Reclamar un campo vacío al salir de él.** Eso se reclama al guardar (`useRequired`).
  En `blur` sólo se revisa lo **mal escrito** (`useFormatError`): un fin anterior al inicio,
  un monto imposible. Se acusa al salir y se perdona por tecla.
- **Preguntar antes de un borrado reversible.** Se borra y se ofrece Deshacer 8 s
  (`_design/UndoToast`). El diálogo se reserva para lo irreversible.
- **Placeholder como etiqueta.**

## 4. Cómo se gana la velocidad aquí

Ninguno de estos es "apretar más el formulario":

1. **Copiar del ciclo anterior** en el tabulador (`tuition-copy.ts`). Montos tal cual,
   fechas recorridas un año exacto. El caso normal pasa de 14 campos en blanco a 3 clics.
2. **Enter encadena** en renglón y chip: doce feriados sin soltar el teclado.
3. **"Guardar y crear otro"** en el cajón (`SlideOver.onSubmitAndNew`), sólo donde el alta
   se hace en serie. En "Nuevo ciclo escolar" **no** se ofrece: se crea uno al año y
   ofrecerlo invita a un error caro.
4. **Defaults**: el orden que sigue, la letra que sigue, el cupo del último salón, Feriado.
5. **Deshacer** en vez de confirmar: agrega red sin costar un clic.

## 5. Trampas que ya costaron tiempo

- **`beforeEach(() => mock.mockResolvedValue(x))`** devuelve el mock, y lo que un `beforeEach`
  devuelve Vitest lo toma por su rutina de limpieza y lo **ejecuta**. Cada prueba deja una
  llamada fantasma sin argumentos. Cuerpo con llaves, siempre.
- **El foco no se pide en el manejador del clic** que crea el campo: ese campo todavía no
  existe. Va en un `useEffect` sobre el estado de apertura.
- **Una prueba de foco que no mete el foco al cajón pasa sola.** "No se escapa" y "lo
  devuelve" son ciertas por inercia si el foco nunca entró. Hay que meterlo al **último**
  campo, que es el borde por donde se fuga.
- **`e2e/setup-from-zero.spec.ts` exige base vacía**: su nivel se llama `"Primaria"` sin
  marca de corrida, así que con `E2E_SKIP_RESET=1` choca consigo mismo en la segunda
  corrida. No es una regresión de la interfaz.
