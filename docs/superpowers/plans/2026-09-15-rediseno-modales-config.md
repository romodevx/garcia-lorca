# Rediseño de modales (Configuración) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que el envase de un alta lo elija el número de campos —1-4 se capturan en el lugar donde vive la lista, 5-8 en cajón compacto, más de eso es página— arreglando de paso el contrato de foco que hoy no cumple ninguna de las 13 pantallas con cajón.

**Architecture:** Todo lo nuevo vive en `apps/web/app/_design/` y las pantallas lo consumen; se respeta la dirección de dependencias existente. La etapa 1 corrige la cáscara compartida (`DrawerShell`, `SlideOver`, `Toast`) y por eso las 13 pantallas heredan el arreglo sin tocarlas. La etapa 2 agrega los dos envases nuevos (`InlineCreateRow`, `InlineCreateChip`) y la cáscara de página (`CreatePage`). La etapa 3 convierte Configuración panel por panel. La etapa 4 saca el tabulador del cajón.

**Tech Stack:** Next.js (App Router) · React 19 · TypeScript estricto · Vitest + @testing-library/react + user-event · Playwright para E2E · estilos con los tokens de `globals.css` (sin librería de UI).

**Spec:** `docs/superpowers/specs/2026-09-15-rediseno-modales-config-design.md`
**Evidencia:** `.claude/docs/design/RESEARCH-modales-2026.md` · regla global `~/.claude/rules/ui-forms-and-modals.md`

**Comandos** (todos desde `apps/web/`):
- Una prueba: `pnpm vitest run app/_design/DrawerShell.test.tsx -t "nombre"`
- Todo: `pnpm test` · Tipos: `pnpm typecheck` · Lint: `pnpm lint`

---

## Estructura de archivos

| Archivo | Responsabilidad | Acción |
|---|---|---|
| `app/_design/DrawerShell.tsx` | Velo, panel, portal, Esc, **contrato de foco APG**, altura por contenido | Modificar |
| `app/_design/DrawerShell.test.tsx` | El contrato de teclado y foco, heredado por 13 pantallas | Crear |
| `app/_design/SlideOver.tsx` | Pie estándar + "Guardar y crear otro" | Modificar |
| `app/_design/Toast.tsx` | Aviso; gana acción para Deshacer | Modificar |
| `app/_design/UndoToast.tsx` | Borrado reversible con temporizador | Crear |
| `app/_design/UndoToast.test.tsx` | Restituye al deshacer; confirma al expirar | Crear |
| `app/_design/InlineCreateRow.tsx` | Renglón editable al pie de una tabla | Crear |
| `app/_design/InlineCreateRow.test.tsx` | Enter guarda y reabre; Esc cancela; error en la celda | Crear |
| `app/_design/InlineCreateChip.tsx` | Chip vacío que se vuelve input | Crear |
| `app/_design/InlineCreateChip.test.tsx` | Mismo contrato de teclado | Crear |
| `app/_design/CreatePage.tsx` | Cáscara de página de alta: migaja, título, riel, barra | Crear |
| `app/_design/CreatePage.test.tsx` | Riel pegajoso; barra siempre visible | Crear |
| `app/_design/form/useFormatError.ts` | Validación en `blur` sólo de datos mal escritos | Crear |
| `app/_design/form/useFormatError.test.ts` | No reclama vacío; sí reclama mal escrito | Crear |
| `app/_design/drawer-contract.test.ts` | Pruebas de código fuente; gana la regla de `column-count` | Modificar |
| `app/globals.css` | Muere el bloque `column-count`; nace `.inline-row`, `.create-page` | Modificar |
| `app/_screens/config/SubjectsPanel.tsx` | Materias → chip | Modificar |
| `app/_screens/config/GradeLevelsPanel.tsx` | Grados → chip | Modificar |
| `app/_screens/config/GroupsPanel.tsx` | Grupos → chip | Modificar |
| `app/_screens/config/TermsPanel.tsx` | Periodos → renglón | Modificar |
| `app/_screens/config/CalendarPanel.tsx` | Días cerrados y actividades → renglón; borrar con Deshacer | Modificar |
| `app/_screens/config/LevelsPanel.tsx` | Sigue en cajón; gana "Guardar y crear otro" | Modificar |
| `app/_screens/config/CycleBar.tsx` | Sigue en cajón | Modificar |
| `app/_screens/config/TuitionPanel.tsx` | Deja de abrir cajón; enlaza a la página | Modificar |
| `app/_screens/config/TuitionPage.tsx` | El tabulador como página de un solo hilo | Crear |
| `app/_screens/config/TuitionPage.test.tsx` | Copiar del ciclo anterior; total en vivo | Crear |
| `app/_screens/config/tuition-copy.ts` | Mapear el plan del ciclo anterior al nuevo | Crear |
| `app/_screens/config/tuition-copy.test.ts` | Montos intactos; fechas + 1 año; fuera de ciclo → vacía | Crear |

---

# ETAPA 1 — La cáscara (13 pantallas la heredan)

### Task 1: El cajón atrapa el foco y lo devuelve

**Files:**
- Modify: `app/_design/DrawerShell.tsx`
- Test: `app/_design/DrawerShell.test.tsx` (crear)

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_design/DrawerShell.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";

import { DrawerShell } from "./DrawerShell";

/**
 * El contrato de diálogo de WAI-ARIA APG, probado UNA vez sobre la cáscara y
 * heredado por las 13 pantallas que la usan. Las tres fallan en silencio: el
 * ratón sigue funcionando y la captura de pantalla se ve bien.
 */
function Host(): React.ReactElement {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>Nuevo nivel</button>
      <button>Otro botón de la página</button>
      {open && (
        <DrawerShell title="Nuevo nivel" icon="settings" onClose={() => setOpen(false)}>
          <input aria-label="Nombre" />
          <input aria-label="Clave" />
        </DrawerShell>
      )}
    </>
  );
}

describe("el contrato de foco del cajón", () => {
  it("pone el foco en el primer campo al abrir", async () => {
    const user = userEvent.setup();
    render(<Host />);

    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));

    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("no deja que Tab se escape a la página de atrás", async () => {
    const user = userEvent.setup();
    render(<Host />);
    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));

    // Nombre → Clave → Cerrar → vuelve a Nombre. Nunca "Otro botón".
    await user.tab();
    await user.tab();
    await user.tab();

    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("devuelve el foco al botón que lo abrió", async () => {
    const user = userEvent.setup();
    render(<Host />);
    const trigger = screen.getByRole("button", { name: "Nuevo nivel" });
    await user.click(trigger);

    await user.keyboard("{Escape}");

    expect(trigger).toHaveFocus();
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/DrawerShell.test.tsx`
Expected: FAIL — los tres. El foco se queda en el `<button>` disparador al abrir, Tab sale a "Otro botón de la página", y al cerrar el foco cae en `<body>`.

- [ ] **Step 3: Implementar**

En `app/_design/DrawerShell.tsx`, agregar `useRef` a los imports de react y, dentro del componente, después del `useEffect` de Escape:

```tsx
  // El panel, para buscar en él lo que puede recibir foco.
  const panelRef = useRef<HTMLElement | null>(null);

  /**
   * El contrato de diálogo de APG: el foco entra, se queda dentro, y vuelve a
   * donde estaba. Las tres partes fallan en silencio —el ratón nunca se entera—
   * así que viven aquí, una sola vez, y no en cada pantalla.
   */
  useEffect(() => {
    const returnTo = document.activeElement as HTMLElement | null;

    const focusables = (): HTMLElement[] => {
      const root = panelRef.current;
      if (root === null) return [];
      return Array.from(
        root.querySelectorAll<HTMLElement>(
          'input:not([type="hidden"]), select, textarea, button, a[href], [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !el.hasAttribute("disabled") && el.offsetParent !== null);
    };

    // El primer CAMPO, no el primer elemento enfocable: abrir sobre el botón de
    // cerrar le da a quien captura un control destructivo bajo el pulgar.
    const first = focusables();
    const firstField = first.find((el) => el.tagName !== "BUTTON") ?? first[0];
    firstField?.focus();

    const onTab = (e: KeyboardEvent): void => {
      if (e.key !== "Tab") return;
      const list = focusables();
      if (list.length === 0) return;
      const edge = e.shiftKey ? list[0] : list[list.length - 1];
      if (document.activeElement === edge) {
        e.preventDefault();
        (e.shiftKey ? list[list.length - 1] : list[0]).focus();
      }
    };
    window.addEventListener("keydown", onTab);

    return () => {
      window.removeEventListener("keydown", onTab);
      // Devolver el foco a quien abrió. Sin esto, al cerrar con Esc el foco cae
      // en <body> y la siguiente tecla no va a ningún lado.
      returnTo?.focus();
    };
  }, []);
```

Y enlazar la referencia: en el `<form>` y en el `<div>` del panel, agregar `ref={panelRef as never}` junto a `className={className}`.

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/DrawerShell.test.tsx`
Expected: PASS, 3 pruebas.

- [ ] **Step 5: Verificar que no rompí las 13 pantallas**

Run: `pnpm test`
Expected: PASS. Si `SlideOver.test.tsx` o `CollectDrawer.test.tsx` fallan por foco inicial, es hallazgo real: la prueba asumía que nada tenía foco. Ajustar la prueba, no el contrato.

- [ ] **Step 6: Commit**

```bash
git add app/_design/DrawerShell.tsx app/_design/DrawerShell.test.tsx
git commit -m "fix(a11y): el cajón atrapa el foco, lo pone en el primer campo y lo devuelve"
```

---

### Task 2: El cajón mide lo que mide su forma

**Files:**
- Modify: `app/_design/DrawerShell.tsx`
- Modify: `app/globals.css`
- Test: `app/_design/DrawerShell.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

Agregar a `app/_design/DrawerShell.test.tsx`:

```tsx
describe("la altura del cajón", () => {
  it("se ajusta a su contenido en vez de ocupar la pantalla", () => {
    render(
      <DrawerShell title="Nuevo día cerrado" icon="calendar" onClose={() => {}}>
        <input aria-label="Nombre" />
      </DrawerShell>,
    );

    const panel = screen.getByRole("dialog");
    // El panel ya no se estira: la altura la pone el contenido, con techo.
    expect(panel).toHaveStyle({ height: "auto", maxHeight: "85vh" });
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/DrawerShell.test.tsx -t "altura"`
Expected: FAIL — `height` es `100%`.

- [ ] **Step 3: Implementar**

En `app/_design/DrawerShell.tsx`, reemplazar en `panelStyle`:

```tsx
  const panelStyle: CSSProperties = {
    // La altura es la de la forma, no la de la pantalla. Un alta de tres campos
    // flotando arriba de un metro de blanco, con Guardar clavado hasta abajo,
    // obliga al ojo a recorrer el monitor entero para nada.
    height: "auto",
    maxHeight: "85vh",
    marginTop: "auto",
    marginBottom: "auto",
    background: "var(--surface)",
    border: "1px solid var(--border)",
    borderRadius: "var(--r-xl) 0 0 var(--r-xl)",
    boxShadow: "var(--shadow-pop)",
    display: "flex",
    flexDirection: "column",
    animation: "slideIn .28s var(--ease) both",
  };
```

Y en el velo, cambiar `justifyContent: "flex-end"` por `justifyContent: "flex-end", alignItems: "center"` para que el panel quede centrado a lo alto.

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/DrawerShell.test.tsx`
Expected: PASS.

- [ ] **Step 5: Verificar el contrato de fuente**

Run: `pnpm test`
Expected: PASS. Nota: `drawer-contract.test.ts` busca `justifyContent: "flex-end"` en `_screens/`, no en `_design/`, así que este cambio no lo dispara.

- [ ] **Step 6: Commit**

```bash
git add app/_design/DrawerShell.tsx app/_design/DrawerShell.test.tsx
git commit -m "fix(ui): el cajón mide lo que mide su forma, no la pantalla"
```

---

### Task 3: El pie nunca tapa el campo enfocado

**Files:**
- Modify: `app/_design/DrawerShell.tsx`
- Test: `app/_design/DrawerShell.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

```tsx
describe("WCAG 2.2 · 2.4.11 — el pie no cubre el campo enfocado", () => {
  it("desplaza el cuerpo cuando el campo enfocado queda bajo el pie", async () => {
    const user = userEvent.setup();
    const scrollIntoView = vi.fn();
    // jsdom no implementa scrollIntoView; se espía que el cajón lo pida.
    Element.prototype.scrollIntoView = scrollIntoView;

    render(
      <DrawerShell title="Alta" icon="settings" onClose={() => {}} footer={<button>Guardar</button>}>
        <input aria-label="Primero" />
        <input aria-label="Último" />
      </DrawerShell>,
    );

    await user.click(screen.getByLabelText("Último"));

    expect(scrollIntoView).toHaveBeenCalledWith({ block: "nearest", behavior: "smooth" });
  });
});
```

(Agregar `vi` al import de vitest en este archivo.)

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/DrawerShell.test.tsx -t "2.4.11"`
Expected: FAIL — `scrollIntoView` nunca se llama.

- [ ] **Step 3: Implementar**

En `DrawerShell`, dentro del `useEffect` del contrato de foco, antes del `return`:

```tsx
    /*
     * WCAG 2.2 · 2.4.11: el pie es pegajoso y puede quedar encima del campo que
     * acaba de recibir el foco. `nearest` no mueve nada si ya se ve — sólo
     * corrige cuando hace falta.
     */
    const onFocusIn = (e: FocusEvent): void => {
      const target = e.target as HTMLElement | null;
      if (target === null || panelRef.current?.contains(target) !== true) return;
      target.scrollIntoView({ block: "nearest", behavior: "smooth" });
    };
    window.addEventListener("focusin", onFocusIn);
```

Y en la limpieza: `window.removeEventListener("focusin", onFocusIn);`

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/DrawerShell.test.tsx`
Expected: PASS, 5 pruebas.

- [ ] **Step 5: Commit**

```bash
git add app/_design/DrawerShell.tsx app/_design/DrawerShell.test.tsx
git commit -m "fix(a11y): el pie pegajoso deja de poder cubrir el campo enfocado (WCAG 2.4.11)"
```

---

### Task 4: "Guardar y crear otro"

**Files:**
- Modify: `app/_design/SlideOver.tsx`
- Test: `app/_design/SlideOver.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

Agregar a `app/_design/SlideOver.test.tsx`:

```tsx
describe("guardar y crear otro", () => {
  it("no ofrece el secundario cuando la pantalla no lo pide", () => {
    open();
    expect(screen.queryByRole("button", { name: "Guardar y crear otro" })).toBeNull();
  });

  it("guarda sin cerrar y avisa que viene otro", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    open({ onSubmit, onSubmitAndNew: () => onSubmit("again") });

    await user.click(screen.getByRole("button", { name: "Guardar y crear otro" }));

    expect(onSubmit).toHaveBeenCalledWith("again");
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/SlideOver.test.tsx -t "guardar y crear otro"`
Expected: FAIL — `onSubmitAndNew` no existe en las props (error de tipos) y no hay tal botón.

- [ ] **Step 3: Implementar**

En `app/_design/SlideOver.tsx`, agregar a `SlideOverProps`:

```tsx
  /**
   * Cuando se da, aparece "Guardar y crear otro": guarda, NO cierra, conserva el
   * contexto (ciclo, nivel), limpia el resto y devuelve el foco al primer campo.
   * Es el patrón Save & New: capturar doce feriados no debe costar doce viajes.
   */
  onSubmitAndNew?: () => void;
```

Y en el pie, entre el espaciador y el primario:

```tsx
          {onSubmitAndNew !== undefined && (
            <Btn
              kind="ghost"
              type="button"
              onClick={() => { if (!pending) onSubmitAndNew(); }}
              style={{ height: 52, fontSize: 15 }}
            >
              Guardar y crear otro
            </Btn>
          )}
```

Agregar `onSubmitAndNew` a la desestructuración de props.

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/SlideOver.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_design/SlideOver.tsx app/_design/SlideOver.test.tsx
git commit -m "feat(ui): 'Guardar y crear otro' en el cajón de alta"
```

---

### Task 5: Borrar con Deshacer en vez de preguntar

**Files:**
- Create: `app/_design/UndoToast.tsx`
- Test: `app/_design/UndoToast.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_design/UndoToast.test.tsx`:

```tsx
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { UndoToast } from "./UndoToast";

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => vi.useRealTimers());

describe("UndoToast", () => {
  it("deshace sin confirmar nada", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const onUndo = vi.fn();
    const onCommit = vi.fn();
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={onUndo} onCommit={onCommit} />);

    await user.click(screen.getByRole("button", { name: "Deshacer" }));

    expect(onUndo).toHaveBeenCalledTimes(1);
    expect(onCommit).not.toHaveBeenCalled();
  });

  it("confirma el borrado cuando pasan los 8 segundos", () => {
    const onUndo = vi.fn();
    const onCommit = vi.fn();
    render(<UndoToast message="Se quitó INDEPENDENCIA" onUndo={onUndo} onCommit={onCommit} />);

    act(() => { vi.advanceTimersByTime(8000); });

    expect(onCommit).toHaveBeenCalledTimes(1);
    expect(onUndo).not.toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/UndoToast.test.tsx`
Expected: FAIL — "Failed to resolve import ./UndoToast".

- [ ] **Step 3: Implementar**

Crear `app/_design/UndoToast.tsx`:

```tsx
"use client";

import { useEffect, type ReactElement } from "react";

import { Icon } from "./Icon";

/**
 * Borrar sin preguntar, y dar ocho segundos para arrepentirse.
 *
 * Un diálogo de confirmación en cada borrado entrena a la gente a descartarlos
 * por reflejo — incluidos los que sí importan (NN/g). El toast no cuesta un clic
 * de más y sí da una red que hoy no existe: "Quitar" borra en seco.
 *
 * El diálogo se reserva para lo irreversible. Esto no lo es.
 */
const GRACE_MS = 8000;

export interface UndoToastProps {
  message: string;
  /** Restituir. El registro nunca llegó a borrarse del servidor. */
  onUndo: () => void;
  /** Se acabó el plazo: ahora sí, borrar de verdad. */
  onCommit: () => void;
}

export function UndoToast({ message, onUndo, onCommit }: UndoToastProps): ReactElement {
  useEffect(() => {
    const id = setTimeout(onCommit, GRACE_MS);
    return () => clearTimeout(id);
  }, [onCommit]);

  return (
    <div
      role="status"
      style={{
        position: "fixed",
        bottom: 28,
        left: "50%",
        transform: "translateX(-50%)",
        zIndex: 80,
        display: "flex",
        alignItems: "center",
        gap: 14,
        padding: "14px 16px 14px 22px",
        borderRadius: "var(--r-pill)",
        background: "var(--text)",
        color: "var(--surface)",
        fontWeight: 600,
        fontSize: 15,
        boxShadow: "var(--shadow-pop)",
        animation: "pop .25s var(--ease) both",
      }}
    >
      <Icon name="check" size={20} stroke={2.6} />
      {message}
      <button
        type="button"
        onClick={onUndo}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 6,
          height: 36,
          padding: "0 16px",
          borderRadius: "var(--r-pill)",
          background: "var(--lime)",
          color: "#16311A",
          fontWeight: 700,
          fontSize: 14,
          cursor: "pointer",
          border: "none",
        }}
      >
        Deshacer
      </button>
    </div>
  );
}
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/UndoToast.test.tsx`
Expected: PASS, 2 pruebas.

- [ ] **Step 5: Commit**

```bash
git add app/_design/UndoToast.tsx app/_design/UndoToast.test.tsx
git commit -m "feat(ui): borrar con Deshacer en lugar de un diálogo que se descarta por reflejo"
```

---

### Task 6: Validar en blur lo mal escrito, nunca lo vacío

**Files:**
- Create: `app/_design/form/useFormatError.ts`
- Test: `app/_design/form/useFormatError.test.ts`
- Modify: `app/_design/form/index.ts`

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_design/form/useFormatError.test.ts`:

```ts
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useFormatError } from "./useFormatError";

const rules = {
  endsOn: (value: string, all: Record<string, string>): string | undefined =>
    value !== "" && all.startsFrom !== "" && value < all.startsFrom
      ? "La fecha de fin es antes que la de inicio."
      : undefined,
};

describe("useFormatError", () => {
  // La decisión que useRequired ya tomó y que aquí NO se toca: un campo vacío
  // no se reclama al salir de él. Perseguir con rojos a quien apenas va
  // llegando al siguiente campo es exactamente lo que se quiso evitar.
  it("no dice nada de un campo vacío", () => {
    const { result } = renderHook(() => useFormatError(rules));

    act(() => { result.current.onBlur("endsOn", "", { startsFrom: "2026-09-01" }); });

    expect(result.current.errors.endsOn).toBeUndefined();
  });

  it("reclama el dato mal escrito al salir del campo", () => {
    const { result } = renderHook(() => useFormatError(rules));

    act(() => { result.current.onBlur("endsOn", "2026-08-01", { startsFrom: "2026-09-01" }); });

    expect(result.current.errors.endsOn).toBe("La fecha de fin es antes que la de inicio.");
  });

  it("borra el error por tecla en cuanto el valor vuelve a ser válido", () => {
    const { result } = renderHook(() => useFormatError(rules));
    act(() => { result.current.onBlur("endsOn", "2026-08-01", { startsFrom: "2026-09-01" }); });

    act(() => { result.current.onChange("endsOn", "2026-10-01", { startsFrom: "2026-09-01" }); });

    expect(result.current.errors.endsOn).toBeUndefined();
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/form/useFormatError.test.ts`
Expected: FAIL — "Failed to resolve import ./useFormatError".

- [ ] **Step 3: Implementar**

Crear `app/_design/form/useFormatError.ts`:

```ts
"use client";

import { useCallback, useState } from "react";

/**
 * Lo que está MAL ESCRITO, revisado al salir del campo.
 *
 * Deliberadamente NO revisa lo que falta: de eso se encarga `useRequired`, al
 * guardar, y esa decisión está bien tomada — reclamar un campo vacío al salir
 * de él persigue con rojos a quien apenas va llegando al siguiente.
 *
 * Lo que sí gana con revisarse en `blur` es el dato imposible: una fecha de fin
 * anterior a la de inicio, un monto negativo. Ahí el hallazgo de Baymard aplica
 * completo (−42% de tiempo de captura, −22% de errores), porque el dato ya está
 * escrito y la persona todavía está parada en el campo. Y en cuanto vuelve a ser
 * válido, el rojo se va por tecla: nunca se valida MIENTRAS se teclea, sólo se
 * perdona mientras se teclea.
 */
export type FormatRule = (value: string, all: Readonly<Record<string, string>>) => string | undefined;

export interface FormatErrorState {
  readonly errors: Readonly<Record<string, string>>;
  /** Al salir del campo: revisa y marca. */
  onBlur: (key: string, value: string, all: Readonly<Record<string, string>>) => void;
  /** Al teclear: sólo perdona, nunca acusa. */
  onChange: (key: string, value: string, all: Readonly<Record<string, string>>) => void;
}

export function useFormatError(rules: Readonly<Record<string, FormatRule>>): FormatErrorState {
  const [errors, setErrors] = useState<Record<string, string>>({});

  const evaluate = useCallback(
    (key: string, value: string, all: Readonly<Record<string, string>>, accusing: boolean): void => {
      const rule = rules[key];
      if (rule === undefined) return;
      // Un campo vacío nunca es "mal escrito": eso lo dice useRequired al guardar.
      const found = value.trim() === "" ? undefined : rule(value, all);
      setErrors((prev) => {
        if (found === undefined) {
          if (prev[key] === undefined) return prev;
          const rest = { ...prev };
          delete rest[key];
          return rest;
        }
        if (!accusing) return prev;
        if (prev[key] === found) return prev;
        return { ...prev, [key]: found };
      });
    },
    [rules],
  );

  return {
    errors,
    onBlur: useCallback(
      (key, value, all) => evaluate(key, value, all, true),
      [evaluate],
    ),
    onChange: useCallback(
      (key, value, all) => evaluate(key, value, all, false),
      [evaluate],
    ),
  };
}
```

Agregar a `app/_design/form/index.ts`:

```ts
export { useFormatError, type FormatErrorState, type FormatRule } from "./useFormatError";
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/form/useFormatError.test.ts`
Expected: PASS, 3 pruebas.

- [ ] **Step 5: Verificar toda la capa de captura**

Run: `pnpm test && pnpm typecheck`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app/_design/form/useFormatError.ts app/_design/form/useFormatError.test.ts app/_design/form/index.ts
git commit -m "feat(form): validar en blur lo mal escrito; lo vacío sigue reclamándose al guardar"
```

---

# ETAPA 2 — Los envases nuevos

### Task 7: InlineCreateRow

**Files:**
- Create: `app/_design/InlineCreateRow.tsx`
- Test: `app/_design/InlineCreateRow.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_design/InlineCreateRow.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { InlineCreateRow, type InlineColumn } from "./InlineCreateRow";

const COLUMNS: readonly InlineColumn[] = [
  { key: "label", label: "Nombre", kind: "text", uppercase: true, required: true },
  { key: "startsOn", label: "Del día", kind: "date", required: true },
  { key: "endsOn", label: "Al día", kind: "date" },
];

function setup(onCreate = vi.fn().mockResolvedValue(undefined)) {
  render(<InlineCreateRow columns={COLUMNS} onCreate={onCreate} addLabel="Agregar día cerrado" />);
  return onCreate;
}

describe("InlineCreateRow", () => {
  it("empieza cerrado, como un renglón que invita", () => {
    setup();
    expect(screen.getByRole("button", { name: "Agregar día cerrado" })).toBeInTheDocument();
    expect(screen.queryByLabelText("Nombre")).toBeNull();
  });

  it("al abrir pone el foco en la primera celda", async () => {
    const user = userEvent.setup();
    setup();

    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  // El corazón del asunto: doce feriados sin soltar el teclado.
  it("Enter guarda y deja listo el siguiente renglón", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    await user.type(screen.getByLabelText("Nombre"), "INDEPENDENCIA");
    await user.type(screen.getByLabelText("Del día"), "2026-09-16");
    await user.keyboard("{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ label: "INDEPENDENCIA", startsOn: "2026-09-16", endsOn: "" });
    expect(screen.getByLabelText("Nombre")).toHaveValue("");
    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("Esc cancela sin guardar", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "ALGO");

    await user.keyboard("{Escape}");

    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("Nombre")).toBeNull();
  });

  // El error sale EN LA CELDA. El defecto que se está arreglando es exactamente
  // que el mensaje aparecía a metros del campo que lo causó.
  it("marca la celda que falta, sin mandar un mensaje a otro lado", async () => {
    const user = userEvent.setup();
    const onCreate = setup();
    await user.click(screen.getByRole("button", { name: "Agregar día cerrado" }));

    await user.type(screen.getByLabelText("Nombre"), "INDEPENDENCIA");
    await user.keyboard("{Enter}");

    expect(onCreate).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Del día")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByLabelText("Del día")).toHaveFocus();
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/InlineCreateRow.test.tsx`
Expected: FAIL — "Failed to resolve import ./InlineCreateRow".

- [ ] **Step 3: Implementar**

Crear `app/_design/InlineCreateRow.tsx`:

```tsx
"use client";

import { useRef, useState, type CSSProperties, type KeyboardEvent, type ReactElement } from "react";

import { Icon } from "./Icon";

/**
 * Dar de alta sin irse a ningún lado.
 *
 * Un registro de uno a cuatro campos no merece un cajón: el viaje —abrir, mirar
 * a otro lado, guardar, volver, buscar dónde quedó— cuesta más que la captura.
 * Aquí el alta ocurre al pie de la lista que ya se está viendo, y Enter guarda y
 * reabre, para que doce feriados sean doce Enter.
 *
 * Carbon lo dice para el caso repetitivo: "si un usuario necesita realizar una
 * tarea repetidamente, considere hacer la tarea completable en la página
 * principal".
 */
export interface InlineColumn {
  key: string;
  label: string;
  kind: "text" | "date" | "number";
  /** Catálogo se captura en mayúsculas, como el resto del sistema. */
  uppercase?: boolean;
  required?: boolean;
  placeholder?: string;
  /** Ancho de la celda; por omisión reparte el espacio. */
  width?: number;
}

export interface InlineCreateRowProps {
  columns: readonly InlineColumn[];
  onCreate: (values: Record<string, string>) => Promise<void>;
  /** Lo que dice el botón cerrado. "Agregar día cerrado" gana a "Agregar". */
  addLabel: string;
}

const empty = (columns: readonly InlineColumn[]): Record<string, string> =>
  Object.fromEntries(columns.map((c) => [c.key, ""]));

const cellInput = (invalid: boolean): CSSProperties => ({
  height: 40,
  width: "100%",
  minWidth: 0,
  padding: "0 12px",
  borderRadius: "var(--r-md)",
  border: `1px solid ${invalid ? "var(--red)" : "var(--border)"}`,
  boxShadow: invalid ? "0 0 0 3px color-mix(in oklab, var(--red) 22%, transparent)" : "none",
  background: "var(--surface)",
  color: "var(--text)",
  fontFamily: "var(--font-ui)",
  fontSize: 15,
  outline: "none",
});

export function InlineCreateRow({ columns, onCreate, addLabel }: InlineCreateRowProps): ReactElement {
  const [open, setOpen] = useState(false);
  const [values, setValues] = useState<Record<string, string>>(() => empty(columns));
  const [invalid, setInvalid] = useState<string | undefined>(undefined);
  const [saving, setSaving] = useState(false);
  const firstRef = useRef<HTMLInputElement | null>(null);

  const focusFirst = (): void => {
    // El foco después del repintado: el input todavía no existe cuando se pide.
    requestAnimationFrame(() => firstRef.current?.focus());
  };

  const cancel = (): void => {
    setOpen(false);
    setValues(empty(columns));
    setInvalid(undefined);
  };

  const save = async (): Promise<void> => {
    const missing = columns.find((c) => c.required === true && values[c.key].trim() === "");
    if (missing !== undefined) {
      // El rojo va EN LA CELDA y el foco se va con él. No hay mensaje suelto en
      // otra parte de la pantalla preguntándose a quién se refiere.
      setInvalid(missing.key);
      document.querySelector<HTMLInputElement>(`[data-inline-key="${missing.key}"]`)?.focus();
      return;
    }
    setSaving(true);
    try {
      await onCreate({ ...values });
      setValues(empty(columns));
      setInvalid(undefined);
      focusFirst();
    } finally {
      setSaving(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLDivElement>): void => {
    if (e.key === "Enter") {
      e.preventDefault();
      void save();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  };

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => { setOpen(true); focusFirst(); }}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 9,
          width: "100%",
          padding: "12px 16px",
          borderTop: "1px solid var(--border)",
          background: "transparent",
          color: "var(--text-muted)",
          fontSize: 14.5,
          fontWeight: 600,
          cursor: "pointer",
        }}
      >
        <Icon name="plus" size={18} />
        {addLabel}
      </button>
    );
  }

  return (
    <div
      onKeyDown={onKeyDown}
      style={{
        display: "flex",
        alignItems: "center",
        gap: 10,
        padding: "10px 16px",
        borderTop: "1px solid var(--border)",
        background: "color-mix(in oklab, var(--lime) 8%, var(--surface))",
      }}
    >
      {columns.map((c, i) => (
        <div key={c.key} style={{ flex: c.width === undefined ? 1 : `0 0 ${c.width}px`, minWidth: 0 }}>
          <input
            ref={i === 0 ? firstRef : undefined}
            data-inline-key={c.key}
            aria-label={c.label}
            aria-invalid={invalid === c.key ? "true" : undefined}
            type={c.kind === "text" ? "text" : c.kind}
            placeholder={c.placeholder}
            value={values[c.key]}
            onChange={(e) => {
              const next = c.uppercase === true ? e.target.value.toUpperCase() : e.target.value;
              setValues((prev) => ({ ...prev, [c.key]: next }));
              if (invalid === c.key && next.trim() !== "") setInvalid(undefined);
            }}
            style={cellInput(invalid === c.key)}
          />
        </div>
      ))}
      <span style={{ fontSize: 12, color: "var(--text-faint)", whiteSpace: "nowrap" }}>
        {saving ? "Guardando…" : "Enter guarda"}
      </span>
      <button
        type="button"
        onClick={cancel}
        aria-label="Cancelar"
        style={{ background: "transparent", border: "none", color: "var(--text-muted)", cursor: "pointer" }}
      >
        <Icon name="close" size={18} />
      </button>
    </div>
  );
}
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/InlineCreateRow.test.tsx`
Expected: PASS, 5 pruebas.

- [ ] **Step 5: Commit**

```bash
git add app/_design/InlineCreateRow.tsx app/_design/InlineCreateRow.test.tsx
git commit -m "feat(ui): renglón de alta en la propia tabla; Enter guarda y reabre"
```

---

### Task 8: InlineCreateChip

**Files:**
- Create: `app/_design/InlineCreateChip.tsx`
- Test: `app/_design/InlineCreateChip.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_design/InlineCreateChip.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { InlineCreateChip } from "./InlineCreateChip";

describe("InlineCreateChip", () => {
  it("empieza como un chip que invita", () => {
    render(<InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={vi.fn()} />);
    expect(screen.getByRole("button", { name: "Nueva materia" })).toBeInTheDocument();
  });

  it("se convierte en campo con el foco puesto", async () => {
    const user = userEvent.setup();
    render(<InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={vi.fn()} />);

    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    expect(screen.getByLabelText("Nombre de la materia")).toHaveFocus();
  });

  it("Enter guarda en mayúsculas y deja listo el siguiente", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    render(<InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={onCreate} />);
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.type(screen.getByLabelText("Nombre de la materia"), "matemáticas{Enter}");

    expect(onCreate).toHaveBeenCalledWith("MATEMÁTICAS");
    expect(screen.getByLabelText("Nombre de la materia")).toHaveValue("");
    expect(screen.getByLabelText("Nombre de la materia")).toHaveFocus();
  });

  it("Enter en vacío no guarda nada", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn();
    render(<InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={onCreate} />);
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.keyboard("{Enter}");

    expect(onCreate).not.toHaveBeenCalled();
  });

  it("Esc lo vuelve a cerrar", async () => {
    const user = userEvent.setup();
    render(<InlineCreateChip label="Nueva materia" fieldLabel="Nombre de la materia" onCreate={vi.fn()} />);
    await user.click(screen.getByRole("button", { name: "Nueva materia" }));

    await user.keyboard("{Escape}");

    expect(screen.getByRole("button", { name: "Nueva materia" })).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/InlineCreateChip.test.tsx`
Expected: FAIL — "Failed to resolve import ./InlineCreateChip".

- [ ] **Step 3: Implementar**

Crear `app/_design/InlineCreateChip.tsx`:

```tsx
"use client";

import { useRef, useState, type KeyboardEvent, type ReactElement } from "react";

import { Icon } from "./Icon";

/**
 * El alta de un solo dato, ahí donde vive la lista.
 *
 * Niveles, grados, materias y grupos se listan como chips, no como tablas. El
 * alta correcta ahí no es un renglón: es un chip vacío que se vuelve campo. Sin
 * cajón, sin viaje, y sin convertir en tabla una fila de chips que funciona.
 *
 * Un chip captura UN valor. Donde el registro trae un segundo campo, ese campo
 * nace con default y se edita después. Si llegara a necesitar un tercero, el
 * alta deja de ser chip y sube a cajón compacto.
 */
export interface InlineCreateChipProps {
  /** Lo que dice el chip cerrado: "Nueva materia". */
  label: string;
  /** El nombre del campo, para quien no ve el chip. */
  fieldLabel: string;
  placeholder?: string;
  onCreate: (name: string) => Promise<void> | void;
}

export function InlineCreateChip({
  label,
  fieldLabel,
  placeholder,
  onCreate,
}: InlineCreateChipProps): ReactElement {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const ref = useRef<HTMLInputElement | null>(null);

  const save = async (): Promise<void> => {
    const name = value.trim();
    // Enter en vacío no es un error que haya que gritar: simplemente no hay nada
    // que guardar todavía.
    if (name === "") return;
    setSaving(true);
    try {
      await onCreate(name);
      setValue("");
      ref.current?.focus();
    } finally {
      setSaving(false);
    }
  };

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>): void => {
    if (e.key === "Enter") {
      e.preventDefault();
      void save();
    }
    if (e.key === "Escape") {
      e.preventDefault();
      setValue("");
      setOpen(false);
    }
  };

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => { setOpen(true); requestAnimationFrame(() => ref.current?.focus()); }}
        style={{
          display: "inline-flex",
          alignItems: "center",
          gap: 7,
          height: 40,
          padding: "0 16px",
          borderRadius: "var(--r-pill)",
          border: "1.5px dashed var(--border-strong)",
          background: "transparent",
          color: "var(--text-muted)",
          fontSize: 14,
          fontWeight: 600,
          cursor: "pointer",
        }}
      >
        <Icon name="plus" size={17} />
        {label}
      </button>
    );
  }

  return (
    <input
      ref={ref}
      aria-label={fieldLabel}
      value={value}
      placeholder={placeholder}
      disabled={saving}
      onChange={(e) => setValue(e.target.value.toUpperCase())}
      onKeyDown={onKeyDown}
      onBlur={() => { if (value.trim() === "") setOpen(false); }}
      style={{
        height: 40,
        padding: "0 16px",
        borderRadius: "var(--r-pill)",
        border: "1.5px solid var(--forest)",
        boxShadow: "0 0 0 3px color-mix(in oklab, var(--forest) 15%, transparent)",
        background: "var(--surface)",
        color: "var(--text)",
        fontFamily: "var(--font-ui)",
        fontSize: 14,
        fontWeight: 600,
        outline: "none",
        minWidth: 190,
      }}
    />
  );
}
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/InlineCreateChip.test.tsx`
Expected: PASS, 5 pruebas.

- [ ] **Step 5: Commit**

```bash
git add app/_design/InlineCreateChip.tsx app/_design/InlineCreateChip.test.tsx
git commit -m "feat(ui): chip de alta donde la lista son chips, no tablas"
```

---

### Task 9: El contrato de fuente prohíbe el flujo en columnas

**Files:**
- Modify: `app/_design/drawer-contract.test.ts`
- Modify: `app/globals.css`

- [ ] **Step 1: Escribir la prueba que falla**

Agregar a `app/_design/drawer-contract.test.ts`:

```ts
/**
 * El flujo tipo periódico en una forma.
 *
 * Baymard lo midió: el layout multicolumna hace que se omitan campos
 * obligatorios y se llenen campos que no venían al caso, porque las columnas
 * "jalan la atención en múltiples direcciones". En el cajón grande, además, el
 * corte caía donde alcanzara la altura: agregar un concepto hacía brincar de
 * columna a una sección entera mientras alguien capturaba.
 *
 * Esta prueba mira el CSS porque lo que defiende es que no vuelva.
 */
describe("las formas", () => {
  it("no fluyen en columnas", () => {
    const css = readFileSync("app/globals.css", "utf8");
    expect(css).not.toMatch(/column-count/);
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/drawer-contract.test.ts`
Expected: FAIL — `column-count` sigue en `globals.css:311`.

- [ ] **Step 3: Implementar**

En `app/globals.css`, borrar el bloque completo `@media (min-width: 1280px) { .drawer-lg .drawer-body… }` (líneas ~305-318, incluido su comentario) y dejar en su lugar:

```css
/* Aquí vivía un `column-count: 2` para el cajón grande. Se fue, y con él el
   defecto: el corte entre columnas caía donde alcanzara la altura, así que
   agregar un concepto de inscripción hacía brincar de columna a "Cobranza"
   sola, reacomodando la forma bajo las manos de quien capturaba. Baymard lo
   midió: multicolumna = campos obligatorios omitidos y campos innecesarios
   llenados. Una columna, siempre. La forma que ya no cabe en una columna no
   necesita dos: necesita ser una página. */
```

**No tocar `.drawer-lg { width: … }`**: lo sigue usando `CollectDrawer`, que monta cuerpo desnudo y al que esta regla nunca le aplicó.

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/drawer-contract.test.ts && pnpm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/globals.css app/_design/drawer-contract.test.ts
git commit -m "fix(ui): muere el flujo en dos columnas del cajón grande, y una prueba impide que vuelva"
```

---

# ETAPA 3 — Configuración, panel por panel

> Cada tarea de esta etapa sigue el mismo ciclo: ajustar la prueba del panel al
> nuevo envase (falla), cambiar el panel (pasa), commit. Las pruebas existentes
> de cada panel buscan el cajón por su título; hay que cambiarlas para buscar el
> chip o el renglón.

### Task 10: Materias → chip

**Files:**
- Modify: `app/_screens/config/SubjectsPanel.tsx`
- Test: `app/_screens/config/Catalog.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

En `app/_screens/config/Catalog.test.tsx`, reemplazar el caso que abre el cajón de materia por:

```tsx
  it("da de alta una materia sin salir de la lista", async () => {
    const user = userEvent.setup();
    renderCatalog();

    await user.click(await screen.findByRole("button", { name: "Nueva materia" }));
    await user.type(screen.getByLabelText("Nombre de la materia"), "matemáticas{Enter}");

    // No hubo cajón en ningún momento.
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(await screen.findByText("MATEMÁTICAS")).toBeInTheDocument();
  });
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx -t "materia"`
Expected: FAIL — hoy se abre un `dialog`.

- [ ] **Step 3: Implementar**

En `app/_screens/config/SubjectsPanel.tsx`: borrar el import de `SlideOver`, el estado `creating`/`name`/`pending`, y el bloque `{creating && <SlideOver …>}`. En su lugar, junto a la lista de chips:

```tsx
      <InlineCreateChip
        label="Nueva materia"
        fieldLabel="Nombre de la materia"
        placeholder="MATEMÁTICAS"
        onCreate={async (name) => { await onCreate({ name }); }}
      />
```

Importar `InlineCreateChip` desde `../../_design/InlineCreateChip`. El manejo de error se conserva con `ErrorAlert` como hoy.

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/SubjectsPanel.tsx app/_screens/config/Catalog.test.tsx
git commit -m "feat(config): la materia se da de alta en su propia lista"
```

---

### Task 11: Grados → chip (con `orden` por default)

**Files:**
- Modify: `app/_screens/config/GradeLevelsPanel.tsx`
- Test: `app/_screens/config/Catalog.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

```tsx
  it("da de alta un grado y le pone el siguiente orden sin preguntarlo", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    renderGradeLevels({ gradeLevels: [{ id: "g1", name: "1° PRIMARIA", ordinal: 1 }], onCreate });

    await user.click(screen.getByRole("button", { name: "Nuevo grado" }));
    await user.type(screen.getByLabelText("Nombre del grado"), "2° primaria{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ name: "2° PRIMARIA", ordinal: 2 });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx -t "grado"`
Expected: FAIL — hoy abre cajón y pide `orden` a mano.

- [ ] **Step 3: Implementar**

En `GradeLevelsPanel.tsx`, quitar el `SlideOver` y sus estados; agregar:

```tsx
  /**
   * El orden no se pregunta: es el siguiente. Preguntarlo obliga a quien captura
   * a leer la lista, contar, y escribir un número que el sistema ya sabe.
   */
  const nextOrdinal = gradeLevels.reduce((max, g) => Math.max(max, g.ordinal), 0) + 1;
```

y en el JSX, junto a los chips:

```tsx
      <InlineCreateChip
        label="Nuevo grado"
        fieldLabel="Nombre del grado"
        placeholder="1° PRIMARIA"
        onCreate={async (name) => { await onCreate({ name, ordinal: nextOrdinal }); }}
      />
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/GradeLevelsPanel.tsx app/_screens/config/Catalog.test.tsx
git commit -m "feat(config): el grado se da de alta en su lista y el orden lo pone el sistema"
```

---

### Task 12: Grupos → chip (con `cupo` por default)

**Files:**
- Modify: `app/_screens/config/GroupsPanel.tsx`
- Test: `app/_screens/config/Catalog.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

```tsx
  it("hereda el cupo del último grupo del grado", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    renderGroups({ groups: [{ id: "x", name: "A", capacity: 30 }], onCreate });

    await user.click(screen.getByRole("button", { name: "Nuevo grupo" }));
    await user.type(screen.getByLabelText("Nombre del grupo"), "b{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ name: "B", capacity: 30 });
  });

  it("usa 25 cuando es el primer grupo del grado", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    renderGroups({ groups: [], onCreate });

    await user.click(screen.getByRole("button", { name: "Nuevo grupo" }));
    await user.type(screen.getByLabelText("Nombre del grupo"), "a{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ name: "A", capacity: 25 });
  });
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx -t "cupo"`
Expected: FAIL.

- [ ] **Step 3: Implementar**

En `GroupsPanel.tsx`, quitar el `SlideOver`; agregar:

```tsx
/** El cupo típico de un salón cuando el grado todavía no tiene ninguno. */
const DEFAULT_CAPACITY = 25;
```

y dentro del componente:

```tsx
  // Hereda del último grupo creado: un grado suele tener salones del mismo
  // tamaño, y el cupo se corrige en el propio renglón si este no lo es.
  const capacity = groups.length === 0 ? DEFAULT_CAPACITY : groups[groups.length - 1].capacity;
```

```tsx
      <InlineCreateChip
        label="Nuevo grupo"
        fieldLabel="Nombre del grupo"
        placeholder="A"
        onCreate={async (name) => { await onCreate({ name, capacity }); }}
      />
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/GroupsPanel.tsx app/_screens/config/Catalog.test.tsx
git commit -m "feat(config): el grupo se da de alta en su lista y hereda el cupo del grado"
```

---

### Task 13: Periodos → renglón

**Files:**
- Modify: `app/_screens/config/TermsPanel.tsx`
- Test: `app/_screens/config/Catalog.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

```tsx
  it("da de alta un periodo en un renglón, con el orden puesto", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    renderTerms({ terms: [{ id: "t1", name: "T1", ordinal: 1, startsOn: "2026-09-01", endsOn: "2026-12-15" }], onCreate });

    await user.click(screen.getByRole("button", { name: "Agregar periodo" }));
    await user.type(screen.getByLabelText("Nombre"), "t2");
    await user.type(screen.getByLabelText("Inicio"), "2027-01-07");
    await user.type(screen.getByLabelText("Fin"), "2027-03-31");
    await user.keyboard("{Enter}");

    expect(onCreate).toHaveBeenCalledWith({ name: "T2", ordinal: 2, startsOn: "2027-01-07", endsOn: "2027-03-31" });
    expect(screen.queryByRole("dialog")).toBeNull();
  });
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx -t "periodo"`
Expected: FAIL.

- [ ] **Step 3: Implementar**

En `TermsPanel.tsx`, quitar el `SlideOver` y envolver la lista de periodos en una tabla con `InlineCreateRow` al pie:

```tsx
      <InlineCreateRow
        addLabel="Agregar periodo"
        columns={[
          { key: "name", label: "Nombre", kind: "text", uppercase: true, required: true, placeholder: "T1", width: 140 },
          { key: "startsOn", label: "Inicio", kind: "date", required: true, width: 170 },
          { key: "endsOn", label: "Fin", kind: "date", required: true, width: 170 },
        ]}
        onCreate={async (v) => {
          await onCreate({ name: v.name, ordinal: nextOrdinal, startsOn: v.startsOn, endsOn: v.endsOn });
        }}
      />
```

con `const nextOrdinal = terms.reduce((max, t) => Math.max(max, t.ordinal), 0) + 1;`

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/TermsPanel.tsx app/_screens/config/Catalog.test.tsx
git commit -m "feat(config): el periodo se captura en un renglón de su propia tabla"
```

---

### Task 14: Días cerrados y actividades → renglón

**Files:**
- Modify: `app/_screens/config/CalendarPanel.tsx`
- Test: `app/_screens/config/CalendarPanel.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

En `CalendarPanel.test.tsx`, reemplazar los casos que abren cajón por:

```tsx
  it("agrega doce feriados sin abrir un solo cajón", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "Agregar día cerrado" }));
    await user.type(screen.getByLabelText("Nombre"), "INDEPENDENCIA");
    await user.type(screen.getByLabelText("Del día"), "2026-09-16");
    await user.keyboard("{Enter}");

    expect(screen.queryByRole("dialog")).toBeNull();
    // El renglón queda listo para el siguiente, con el foco puesto.
    expect(screen.getByLabelText("Nombre")).toHaveFocus();
  });

  it("agrega una actividad en su propio renglón", async () => {
    const user = userEvent.setup();
    renderPanel();

    await user.click(await screen.findByRole("button", { name: "Agregar actividad" }));
    await user.type(screen.getByLabelText("Actividad"), "FESTIVAL DE PRIMAVERA");
    await user.type(screen.getByLabelText("Fecha"), "2027-03-21");
    await user.keyboard("{Enter}");

    expect(screen.queryByRole("dialog")).toBeNull();
  });
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/CalendarPanel.test.tsx`
Expected: FAIL.

- [ ] **Step 3: Implementar**

En `CalendarPanel.tsx`: quitar el import de `SlideOver`, el estado `open`, y los dos bloques `{open === "day"}` / `{open === "event"}`. El tipo (`Feriado`/`Vacaciones`) deja de ser un campo del alta y pasa a ser un selector del renglón con default `holiday` — se cambia después desde el renglón del día. Al pie de la tabla de días cerrados:

```tsx
        <InlineCreateRow
          addLabel="Agregar día cerrado"
          columns={[
            { key: "label", label: "Nombre", kind: "text", uppercase: true, required: true, placeholder: "INDEPENDENCIA" },
            { key: "startsOn", label: "Del día", kind: "date", required: true, width: 170 },
            { key: "endsOn", label: "Al día", kind: "date", width: 170 },
          ]}
          onCreate={async (v) => {
            await createNonWorkingPeriod(API_URL, {
              cycleId: cycle.id,
              label: v.label.trim(),
              // El default es el caso común: un feriado. Vacaciones se marca
              // después en el renglón, que es donde se ve junto a sus fechas.
              kind: "holiday",
              startsOn: v.startsOn,
              ...(v.endsOn !== "" ? { endsOn: v.endsOn } : {}),
            });
            reload();
          }}
        />
```

y al pie de la de actividades:

```tsx
        <InlineCreateRow
          addLabel="Agregar actividad"
          columns={[
            { key: "title", label: "Actividad", kind: "text", uppercase: true, required: true, placeholder: "FESTIVAL DE PRIMAVERA" },
            { key: "startsOn", label: "Fecha", kind: "date", required: true, width: 170 },
            { key: "endsOn", label: "Hasta", kind: "date", width: 170 },
          ]}
          onCreate={async (v) => {
            await createSchoolEvent(API_URL, {
              cycleId: cycle.id,
              title: v.title.trim(),
              startsOn: v.startsOn,
              ...(v.endsOn !== "" ? { endsOn: v.endsOn } : {}),
            });
            reload();
          }}
        />
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/CalendarPanel.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/CalendarPanel.tsx app/_screens/config/CalendarPanel.test.tsx
git commit -m "feat(config): días cerrados y actividades se capturan en su propia tabla"
```

---

### Task 15: Quitar un día cerrado da ocho segundos para arrepentirse

**Files:**
- Modify: `app/_screens/config/CalendarPanel.tsx`
- Test: `app/_screens/config/CalendarPanel.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

```tsx
  it("quitar un día ofrece deshacer, y deshacer lo devuelve", async () => {
    const user = userEvent.setup();
    renderPanel({ periods: [{ id: "p1", label: "INDEPENDENCIA", kind: "holiday", startsOn: "2026-09-16", endsOn: "2026-09-16" }] });

    await user.click(await screen.findByRole("button", { name: "Quitar INDEPENDENCIA" }));
    expect(screen.queryByText("INDEPENDENCIA")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Deshacer" }));

    expect(await screen.findByText("INDEPENDENCIA")).toBeInTheDocument();
    expect(deleteNonWorkingPeriod).not.toHaveBeenCalled();
  });
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/CalendarPanel.test.tsx -t "deshacer"`
Expected: FAIL — hoy `remove` llama al borrado de inmediato.

- [ ] **Step 3: Implementar**

En `CalendarPanel.tsx`, agregar estado y reemplazar `remove`:

```tsx
  /**
   * Lo que está en el limbo: se quitó de la lista pero todavía no del servidor.
   * Ocho segundos para arrepentirse. Hoy "Quitar" borra en seco, sin red.
   */
  const [pendingRemoval, setPendingRemoval] = useState<NonWorkingPeriod | undefined>(undefined);

  const remove = (period: NonWorkingPeriod): void => {
    setPeriods((prev) => prev.filter((p) => p.id !== period.id));
    setPendingRemoval(period);
  };
```

y en el JSX, al final:

```tsx
      {pendingRemoval !== undefined && (
        <UndoToast
          message={`Se quitó ${pendingRemoval.label}`}
          onUndo={() => {
            setPeriods((prev) => [...prev, pendingRemoval].sort((a, b) => a.startsOn.localeCompare(b.startsOn)));
            setPendingRemoval(undefined);
          }}
          onCommit={() => {
            void deleteNonWorkingPeriod(API_URL, pendingRemoval.id)
              .then(reload)
              .catch(() => setError("No se pudo quitar ese día."));
            setPendingRemoval(undefined);
          }}
        />
      )}
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/CalendarPanel.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/CalendarPanel.tsx app/_screens/config/CalendarPanel.test.tsx
git commit -m "feat(config): quitar un día cerrado da ocho segundos para arrepentirse"
```

---

### Task 16: Ciclo y Nivel conservan su cajón y ganan "Guardar y crear otro"

**Files:**
- Modify: `app/_screens/config/LevelsPanel.tsx`
- Modify: `app/_screens/config/CycleBar.tsx`
- Test: `app/_screens/config/Catalog.test.tsx`, `app/_screens/config/CycleBar.test.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

```tsx
  it("crea un nivel y deja el cajón listo para el siguiente", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    renderLevels({ onCreate });

    await user.click(screen.getByRole("button", { name: "Nuevo nivel" }));
    await user.type(screen.getByLabelText("Nombre del nivel"), "PRIMARIA");
    await user.click(screen.getByRole("button", { name: "Guardar y crear otro" }));

    expect(onCreate).toHaveBeenCalled();
    // Sigue abierto, vacío y con el foco puesto.
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("Nombre del nivel")).toHaveValue("");
    expect(screen.getByLabelText("Nombre del nivel")).toHaveFocus();
  });
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/Catalog.test.tsx -t "listo para el siguiente"`
Expected: FAIL — no existe ese botón.

- [ ] **Step 3: Implementar**

En `LevelsPanel.tsx`, extraer el guardado a una función que reciba si cierra, y pasar `onSubmitAndNew`:

```tsx
  const submit = async (andNew: boolean): Promise<void> => {
    if (name.trim() === "" || sepCode.trim() === "") {
      setError("Un nivel necesita nombre y clave SEP.");
      return;
    }
    setPending(true);
    setError(undefined);
    try {
      await onCreate({
        name: name.trim(),
        sepCode: sepCode.trim(),
        enrollmentGranularity: granularity as EnrollmentGranularity,
      });
      setName("");
      setSepCode("");
      setGranularity("cycle");
      if (andNew) {
        // Queda abierto para el siguiente: una escuela captura maternal, kínder
        // y primaria de corrido, no uno cada quince días.
        document.querySelector<HTMLInputElement>('[aria-label="Nombre del nivel"]')?.focus();
      } else {
        setCreating(false);
      }
    } catch {
      setError("No se pudo crear el nivel. Revisa que no exista ya con ese nombre.");
    } finally {
      setPending(false);
    }
  };
```

y en el `SlideOver`: `onSubmit={() => void submit(false)}` más `onSubmitAndNew={() => void submit(true)}`.

`CycleBar.tsx` NO lleva "Guardar y crear otro": un ciclo escolar se crea uno por año, y ofrecer "y crear otro" ahí invita a un error caro. Sólo se verifica que su cajón sigue funcionando.

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/LevelsPanel.tsx app/_screens/config/Catalog.test.tsx
git commit -m "feat(config): el nivel se puede crear en serie sin cerrar el cajón"
```

---

# ETAPA 4 — El tabulador deja de ser un modal

### Task 17: Copiar el plan del ciclo anterior

**Files:**
- Create: `app/_screens/config/tuition-copy.ts`
- Test: `app/_screens/config/tuition-copy.test.ts`

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_screens/config/tuition-copy.test.ts`:

```ts
import { describe, expect, it } from "vitest";

import { copyPlanToCycle } from "./tuition-copy";

const CYCLE = { id: "c2", name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-07-31" };

const PREVIOUS = {
  id: "p1",
  cycleId: "c1",
  levelId: "l1",
  monthlyCents: 280000,
  months: 10,
  firstMonth: 9,
  dueDay: 10,
  enrollmentDueOn: "2026-06-30",
  concepts: [
    { name: "INSCRIPCIÓN", tiers: [{ amountCents: 450000, startsOn: "2025-08-31", endsOn: undefined }] },
  ],
};

describe("copyPlanToCycle", () => {
  it("deja los montos intactos: subirlos es decisión de quien captura", () => {
    const copy = copyPlanToCycle(PREVIOUS, CYCLE);

    expect(copy.monthlyCents).toBe(280000);
    expect(copy.concepts[0].tiers[0].amountCents).toBe(450000);
  });

  it("recorre las fechas exactamente un año", () => {
    const copy = copyPlanToCycle(PREVIOUS, CYCLE);

    expect(copy.concepts[0].tiers[0].startsOn).toBe("2026-08-31");
    expect(copy.enrollmentDueOn).toBe("2027-06-30");
  });

  // Mejor un hueco visible que una fecha silenciosamente equivocada en algo que
  // cobra dinero.
  it("deja vacía la fecha que se sale del ciclo destino", () => {
    const outside = { ...PREVIOUS, enrollmentDueOn: "2026-07-15" };

    // 2026-07-15 + 1 año = 2027-07-15, dentro. Se usa una que sí se sale:
    const wayOut = { ...PREVIOUS, enrollmentDueOn: "2026-08-15" };
    expect(copyPlanToCycle(wayOut, CYCLE).enrollmentDueOn).toBe("");
    expect(copyPlanToCycle(outside, CYCLE).enrollmentDueOn).toBe("2027-07-15");
  });

  it("apunta al ciclo nuevo, no al viejo", () => {
    expect(copyPlanToCycle(PREVIOUS, CYCLE).cycleId).toBe("c2");
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/tuition-copy.test.ts`
Expected: FAIL — "Failed to resolve import ./tuition-copy".

- [ ] **Step 3: Implementar**

Crear `app/_screens/config/tuition-copy.ts`:

```ts
import type { Cycle, TuitionPlan } from "../../finance/api";

/**
 * El tabulador del año pasado, listo para el que empieza.
 *
 * El caso real del colegio no es capturar catorce campos en blanco: es "los
 * precios del año pasado, con el monto más alto". Ese camino no existía, y por
 * eso la captura del tabulador se sentía como empezar de cero cada agosto.
 *
 * Los MONTOS se copian tal cual, a propósito: subirlos es la decisión que quien
 * captura vino a tomar, y adivinarla sería cobrarle a las familias un número que
 * nadie escribió. Las FECHAS sí se recorren, porque una vigencia del ciclo
 * pasado aplicada al nuevo no es un dato conservador: es un dato equivocado.
 */
export function copyPlanToCycle(previous: TuitionPlan, target: Cycle): TuitionPlan {
  const shift = (iso: string | undefined): string => shiftOneYear(iso, target);
  return {
    ...previous,
    cycleId: target.id,
    enrollmentDueOn: shift(previous.enrollmentDueOn),
    concepts: previous.concepts.map((c) => ({
      ...c,
      tiers: c.tiers.map((t) => ({
        ...t,
        startsOn: shift(t.startsOn),
        endsOn: t.endsOn === undefined ? undefined : shift(t.endsOn),
      })),
    })),
  };
}

/**
 * La misma fecha, un año después. Si el resultado cae fuera del ciclo destino se
 * devuelve vacía: un hueco se ve y se llena, una fecha equivocada cobra mal
 * durante meses sin que nadie lo note.
 */
function shiftOneYear(iso: string | undefined, target: Cycle): string {
  if (iso === undefined || iso === "") return "";
  const [year, rest] = [iso.slice(0, 4), iso.slice(4)];
  const moved = `${String(Number(year) + 1)}${rest}`;
  if (moved < target.startsOn || moved > target.endsOn) return "";
  return moved;
}
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/tuition-copy.test.ts`
Expected: PASS, 4 pruebas.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/tuition-copy.ts app/_screens/config/tuition-copy.test.ts
git commit -m "feat(config): copiar el tabulador del ciclo anterior con las fechas recorridas un año"
```

---

### Task 18: CreatePage — la cáscara de página de alta

**Files:**
- Create: `app/_design/CreatePage.tsx`
- Test: `app/_design/CreatePage.test.tsx`
- Modify: `app/globals.css`

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_design/CreatePage.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { CreatePage } from "./CreatePage";

describe("CreatePage", () => {
  it("se nombra con migaja y título, para saber dónde se está", () => {
    render(
      <CreatePage
        breadcrumb={["Configuración", "Colegiaturas"]}
        title="Precios de Kínder · 2026-2027"
        subtitle="Inscripción, colegiatura, cobranza y cuotas"
        aside={<p>Año completo</p>}
        footer={<button>Guardar tabulador</button>}
      >
        <input aria-label="Mensual" />
      </CreatePage>,
    );

    expect(screen.getByRole("heading", { name: "Precios de Kínder · 2026-2027" })).toBeInTheDocument();
    expect(screen.getByText("Configuración")).toBeInTheDocument();
  });

  it("el resumen y el guardar viven fuera del cuerpo que se desplaza", () => {
    render(
      <CreatePage breadcrumb={["Configuración"]} title="Alta" aside={<p>Resumen</p>} footer={<button>Guardar</button>}>
        <input aria-label="Campo" />
      </CreatePage>,
    );

    expect(screen.getByTestId("create-page-aside")).toBeInTheDocument();
    expect(screen.getByTestId("create-page-footer")).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_design/CreatePage.test.tsx`
Expected: FAIL — "Failed to resolve import ./CreatePage".

- [ ] **Step 3: Implementar**

Crear `app/_design/CreatePage.tsx`:

```tsx
"use client";

import type { ReactElement, ReactNode } from "react";

/**
 * La página de un alta que ya no cabe en un cajón.
 *
 * Carbon lo dice sin rodeos: si la altura del modal grande no alcanza,
 * probablemente se necesita una página completa. El tabulador de colegiaturas
 * son catorce campos en cinco secciones; llevaba años capturándose por un cajón
 * que, para que cupiera, fluía en dos columnas tipo periódico —y el corte caía
 * donde alcanzara la altura, reacomodándose mientras alguien capturaba.
 *
 * Una columna, el cuerpo es lo único que se desplaza, y dos cosas que NUNCA se
 * van de la vista: el resumen de lo que se está armando y el botón de guardar.
 */
export interface CreatePageProps {
  breadcrumb: readonly string[];
  title: string;
  subtitle?: string;
  /** El resumen en vivo. Pegado, porque es lo que se revisa antes de guardar. */
  aside?: ReactNode;
  footer: ReactNode;
  children: ReactNode;
}

export function CreatePage({
  breadcrumb,
  title,
  subtitle,
  aside,
  footer,
  children,
}: CreatePageProps): ReactElement {
  return (
    <div className="create-page">
      <header style={{ padding: "22px 26px 18px" }}>
        <p style={{ fontSize: 13, color: "var(--text-muted)", marginBottom: 4 }}>
          {breadcrumb.join(" › ")} ›
        </p>
        <h1 style={{ fontSize: 26, fontWeight: 800, color: "var(--text)", lineHeight: 1.15 }}>{title}</h1>
        {subtitle !== undefined && (
          <p style={{ fontSize: 14.5, color: "var(--text-muted)", marginTop: 4 }}>{subtitle}</p>
        )}
      </header>

      <div className="create-page-grid">
        {/* Una columna. El ancho lo acota globals.css, donde una @media puede
            opinar — un estilo en línea aquí le ganaría, y este repo ya pagó esa
            lección dos veces. */}
        <div className="create-page-body">{children}</div>
        {aside !== undefined && (
          <aside data-testid="create-page-aside" className="create-page-aside">
            {aside}
          </aside>
        )}
      </div>

      <div data-testid="create-page-footer" className="create-page-footer">
        {footer}
      </div>
    </div>
  );
}
```

Agregar a `app/globals.css`:

```css
/* ---------------- PÁGINA DE ALTA ----------------
   Para la forma que ya no cabe en un cajón. Una columna de lectura acotada, el
   resumen pegado a un lado y el guardar pegado abajo: las dos cosas que no
   pueden irse de la vista mientras se captura. */
.create-page { display: flex; flex-direction: column; min-height: 100%; }
.create-page-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 22px;
  padding: 0 26px 26px;
  align-items: start;
}
.create-page-body { display: flex; flex-direction: column; gap: 16px; max-width: 680px; }
.create-page-aside { position: sticky; top: 22px; }
.create-page-footer {
  position: sticky;
  bottom: 0;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px 26px;
  border-top: 1px solid var(--border);
  background: var(--surface);
  z-index: 5;
}
@media (min-width: 1100px) {
  .create-page-grid { grid-template-columns: minmax(0, 1fr) 280px; }
}
```

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_design/CreatePage.test.tsx`
Expected: PASS, 2 pruebas.

- [ ] **Step 5: Commit**

```bash
git add app/_design/CreatePage.tsx app/_design/CreatePage.test.tsx app/globals.css
git commit -m "feat(ui): cáscara de página para el alta que ya no cabe en un cajón"
```

---

### Task 19: El tabulador se muda a la página

**Files:**
- Create: `app/_screens/config/TuitionPage.tsx`
- Test: `app/_screens/config/TuitionPage.test.tsx`
- Modify: `app/_screens/config/TuitionPanel.tsx`

- [ ] **Step 1: Escribir la prueba que falla**

Crear `app/_screens/config/TuitionPage.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { TuitionPage } from "./TuitionPage";

const CYCLE = { id: "c2", name: "2026-2027", startsOn: "2026-08-01", endsOn: "2027-07-31", status: "draft" as const };
const LEVEL = { id: "l1", name: "KÍNDER", sepCode: "KIN", enrollmentGranularity: "cycle" as const };

function renderPage(props = {}) {
  return render(
    <TuitionPage cycle={CYCLE} level={LEVEL} plan={undefined} previousPlan={undefined} onSave={vi.fn()} onClose={vi.fn()} {...props} />,
  );
}

describe("TuitionPage", () => {
  it("es una página, no un cajón", () => {
    renderPage();
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("heading", { name: /Precios de KÍNDER/ })).toBeInTheDocument();
  });

  it("muestra el año completo y lo recalcula al teclear", async () => {
    const user = userEvent.setup();
    renderPage();

    await user.clear(screen.getByLabelText("Colegiatura mensual"));
    await user.type(screen.getByLabelText("Colegiatura mensual"), "2800.00");

    // 2800 × 10 mensualidades.
    expect(await screen.findByText("$28,000.00")).toBeInTheDocument();
  });

  it("Cobranza y Cuotas nacen plegadas, con su estado en una línea", () => {
    renderPage();
    expect(screen.getByRole("button", { name: /Cobranza/ })).toHaveAttribute("aria-expanded", "false");
    expect(screen.getByRole("button", { name: /Cuotas/ })).toHaveAttribute("aria-expanded", "false");
  });

  it("ofrece copiar el ciclo anterior sólo cuando hay de dónde", async () => {
    const user = userEvent.setup();
    const previousPlan = {
      id: "p1", cycleId: "c1", levelId: "l1", monthlyCents: 280000, months: 10,
      firstMonth: 9, dueDay: 10, enrollmentDueOn: "2026-06-30", concepts: [],
    };
    renderPage({ previousPlan });

    await user.click(screen.getByRole("button", { name: /Copiar de/ }));

    expect(screen.getByLabelText("Colegiatura mensual")).toHaveValue("2800.00");
  });

  it("no ofrece copiar cuando no hay ciclo anterior", () => {
    renderPage();
    expect(screen.queryByRole("button", { name: /Copiar de/ })).toBeNull();
  });
});
```

- [ ] **Step 2: Correr y verificar que falla**

Run: `pnpm vitest run app/_screens/config/TuitionPage.test.tsx`
Expected: FAIL — "Failed to resolve import ./TuitionPage".

- [ ] **Step 3: Implementar**

Crear `app/_screens/config/TuitionPage.tsx` moviendo el contenido de `PlanForm` (hoy en `TuitionPanel.tsx:423-640`) a una página. Estructura:

- Envolver en `<CreatePage breadcrumb={["Configuración", "Colegiaturas"]} title={\`Precios de ${level.name} · ${cycle.name}\`} subtitle="Inscripción, colegiatura, cobranza y cuotas" aside={<YearSummary … />} footer={<><Btn kind="ghost" onClick={onClose}>Cancelar</Btn><div style={{flex:1}} /><Btn icon="check" onClick={save}>Guardar tabulador</Btn></>}>`.
- Dentro, cuatro `<FormSection n={1..4}>` en este orden: Colegiatura · Inscripción · Cobranza · Cuotas.
- `FormSection` recibe `collapsible` y `summary`; Cobranza y Cuotas la usan con `defaultOpen={false}`, y su encabezado es un `<button aria-expanded>`.
- Sobre las secciones, si `previousPlan !== undefined`, la tarjeta de copiar que llama `copyPlanToCycle(previousPlan, cycle)` y siembra el estado del formulario.
- `YearSummary` calcula con las funciones YA EXISTENTES que se quedan en su archivo actual y se importan: `inscriptionTotals`, `monthlyTotals`, `centsToPesos`, `pesosToCents`, `billedMonths`, `feeShares`, `ladderProblem`. **No se reimplementan.**

En `TuitionPanel.tsx`: borrar el `SlideOver` y el componente `PlanForm`; el botón de cada nivel deja de abrir cajón y llama `onOpenPlan(levelId)`, que el `Catalog` resuelve mostrando `<TuitionPage>` en lugar del catálogo.

- [ ] **Step 4: Correr y verificar que pasa**

Run: `pnpm vitest run app/_screens/config/ && pnpm typecheck`
Expected: PASS. `TuitionPanel.test.tsx` fallará donde busque el `dialog`; ajustar esos casos para buscar el encabezado de la página — el cálculo que prueban no cambia.

- [ ] **Step 5: Commit**

```bash
git add app/_screens/config/TuitionPage.tsx app/_screens/config/TuitionPage.test.tsx app/_screens/config/TuitionPanel.tsx app/_screens/config/TuitionPanel.test.tsx
git commit -m "feat(config): el tabulador deja el cajón y se vuelve una página de un solo hilo"
```

---

### Task 20: Cerrar — E2E, tipos, lint y las reglas del proyecto

**Files:**
- Modify: `app/e2e/drawer.spec.ts` y los specs de configuración
- Create: `.claude/docs/design/REGLAS-modales.md`

- [ ] **Step 1: Correr la suite completa**

Run: `pnpm test && pnpm typecheck && pnpm lint`
Expected: PASS las tres.

- [ ] **Step 2: Correr E2E y ajustar lo que cambió de envase**

Run: `E2E_SKIP_RESET=1 pnpm e2e --project=chromium`
Expected: fallan los specs que abren cajón para crear en Configuración. Ajustarlos al chip/renglón. **Usar `E2E_SKIP_RESET=1`**: sin eso `globalSetup` vacía la base sembrada.

- [ ] **Step 3: Escribir las reglas del proyecto**

Crear `.claude/docs/design/REGLAS-modales.md` con: la tabla de envase por número de campos, la excepción declarada ("una decisión que necesita explicarse sube un envase"), el contrato de teclado, y un puntero a `~/.claude/rules/ui-forms-and-modals.md` y a la investigación. Sin repetir la evidencia: se referencia.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "docs(design): las reglas de modales del proyecto, apuntando a la regla global"
```

---

## Auto-revisión del plan

**Cobertura de la spec** — §1 diagnóstico → Tasks 2, 3, 9; §2 tabla de envases → Tasks 10-16; §3 tabulador página → Tasks 17-19; §4 mecanismos de velocidad → Task 17 (copiar), 7/8 (Enter reabre), 4 (guardar y crear otro), 11/12/14 (defaults), 5/15 (deshacer); §5 arquitectura → todas; §6 pruebas → cada task abre con la suya, más Task 9 (fuente) y Task 20 (E2E); §7 orden → el de las etapas. Sin huecos.

**Consistencia de tipos** — `InlineColumn.key/label/kind` se usa igual en Tasks 7, 13, 14. `onCreate` devuelve `Promise<void>` en `InlineCreateRow` y `Promise<void> | void` en `InlineCreateChip` (el chip acepta síncrono porque varios paneles ya tienen el handler síncrono). `copyPlanToCycle(previous, target)` se llama con ese orden en Tasks 17 y 19. `UndoToast` expone `message/onUndo/onCommit` en Tasks 5 y 15.

**Riesgo conocido** — Task 19 mueve ~220 líneas de `PlanForm`. Se hace **sin reescribir** las funciones de cálculo (`inscriptionTotals`, `monthlyTotals`, `billedMonths`, `feeShares`, `ladderProblem`, `pesosToCents`, `centsToPesos`), que ya tienen pruebas en `TuitionPanel.test.tsx` y se importan tal cual. Si esas pruebas siguen verdes, el dinero no cambió.
