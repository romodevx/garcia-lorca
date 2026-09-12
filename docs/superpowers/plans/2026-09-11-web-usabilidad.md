# Web Usabilidad — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hacer `apps/web` usable en cualquier monitor de 1024px a 2560px, con el botón "atrás" del navegador funcionando dentro del sitio, y con una sola capa de captura que garantice calendario en fechas, mayúsculas en nombres, minúsculas en correo, pesos en montos y campos requeridos marcados.

**Architecture:** Tres bloques independientes sobre `apps/web`. (A) La URL pasa a ser el estado de navegación vía un catch-all de Next App Router y un módulo puro `routes.ts`. (B) Nace `app/_design/form/` como única capa de inputs y los ~50 inputs en línea migran a ella. (C) Los anchos salen del JSX a tokens CSS, y `SlideOver` se unifica y se renderiza por `createPortal` a `document.body`, borrando sus 8 clones.

**Tech Stack:** Next.js 16.2.9 (App Router), React 19.2.7, TypeScript 6, Vitest 4.1.8 + Testing Library, Playwright 1.62.

**Spec:** `.claude/docs/PLAN-web-usabilidad.md`

**Rama:** `feature/responsive-navegacion-formularios` (ya creada, de `dev`)

---

## Comandos de verificación

Todos desde `apps/web`:

| Qué | Comando |
|---|---|
| Un test | `pnpm vitest run app/ruta/archivo.test.ts` |
| Toda la suite unitaria | `pnpm test` |
| Tipos | `pnpm typecheck` |
| Lint | `pnpm lint` |
| e2e (requiere stack local) | `pnpm e2e` |
| Build de producción | `pnpm build && pnpm check:no-sourcemaps` |

---

## Estructura de archivos

**Nacen:**

| Archivo | Responsabilidad |
|---|---|
| `app/routing/routes.ts` | Mapa puro `View ↔ pathname`, en ambos sentidos. Sin React. |
| `app/routing/routes.test.ts` | Su suite. |
| `app/[[...slug]]/page.tsx` | Catch-all: toda pantalla del shell responde en su URL. |
| `app/_lib/money.ts` | El `finance/money.ts` de hoy, movido intacto. Único formateador. |
| `app/_lib/money.test.ts` | Su suite, movida intacta. |
| `app/_design/form/useRequired.ts` | Hook puro: qué falta, salta al primero, limpia al escribir. |
| `app/_design/form/useRequired.test.ts` | Su suite. |
| `app/_design/form/Field.tsx` | Envoltura común: etiqueta, `*`, borde rojo, mensaje, ARIA. |
| `app/_design/form/fields.tsx` | `TextField` `EmailField` `NumberField` `TextArea` `SelectField`. |
| `app/_design/form/DateField.tsx` | `type="date"` — calendario nativo. |
| `app/_design/form/MoneyField.tsx` | Prefijo `$`, formatea al salir, entrega centavos. |
| `app/_design/form/index.ts` | Superficie pública del paquete. |
| `app/_design/form/*.test.tsx` | Una suite por primitiva. |
| `app/_design/Page.tsx` | `<Page>` y `<Page variant="reading">`. Mata 12 copias de `maxWidth`. |
| `e2e/navigation.spec.ts` | Atrás, adelante, F5, link directo, link directo sin sesión. |
| `e2e/drawer.spec.ts` | El velo cubre el viewport completo; el cajón toca el borde. |
| `e2e/wide.spec.ts` | 2560×1440: sin scroll lateral y el ancho se aprovecha. |
| `e2e/tablet.spec.ts` | 1024×768: sin scroll lateral en cada módulo. |

**Mueren:**

| Archivo | Por qué |
|---|---|
| `app/page.tsx` | Lo reemplaza el catch-all (Next prohíbe tener ambos). |
| `app/(auth)/login/page.tsx` | Stub muerto de FASE 0; `/login` sirve "Stub — la autenticación llega con el módulo IAM". |
| `app/_screens/finance/money.ts` + `.test.ts` | Se mueven a `app/_lib/`. |

**Cambian a fondo:** `app/AppShell.tsx` · `app/_design/SlideOver.tsx` · `app/globals.css` · y los 23 archivos con inputs en línea.

---

## BLOQUE A — Navegación por URL

### Task 1: El mapa de rutas (módulo puro)

**Files:**
- Create: `apps/web/app/routing/routes.ts`
- Test: `apps/web/app/routing/routes.test.ts`

- [ ] **Step 1: Escribir el test que falla**

```ts
import { describe, expect, it } from "vitest";
import { pathToView, viewToPath, type View } from "./routes";

describe("el mapa de rutas", () => {
  const pares: ReadonlyArray<readonly [string, View]> = [
    ["/", { screen: "dashboard" }],
    ["/entrar", { screen: "login" }],
    ["/cambiar-contrasena", { screen: "changePassword" }],
    ["/finanzas", { screen: "finance" }],
    ["/pagos", { screen: "payments" }],
    ["/alumnos", { screen: "students" }],
    ["/calificaciones", { screen: "grades" }],
    ["/asistencia", { screen: "attendance" }],
    ["/docentes", { screen: "teachers" }],
    ["/uniformes", { screen: "uniforms" }],
    ["/avisos", { screen: "announcements" }],
    ["/configuracion", { screen: "config" }],
    ["/administracion", { screen: "admin" }],
    ["/modulo/reportes", { screen: "module", moduleId: "reportes" }],
  ];

  it.each(pares)("%s ida y vuelta", (path, view) => {
    expect(pathToView(path)).toEqual(view);
    expect(viewToPath(view)).toBe(path);
  });

  it("una ruta desconocida aterriza en el tablero, no en blanco", () => {
    expect(pathToView("/no-existe")).toEqual({ screen: "dashboard" });
  });

  it("tolera la diagonal final", () => {
    expect(pathToView("/finanzas/")).toEqual({ screen: "finance" });
  });

  it("un id de módulo vacío no finge ser un módulo", () => {
    expect(pathToView("/modulo/")).toEqual({ screen: "dashboard" });
  });
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/routing/routes.test.ts`
Expected: FAIL — `Failed to resolve import "./routes"`.

- [ ] **Step 3: Implementar**

`View` se **mueve** aquí desde `AppShell.tsx:34-49` (deja de vivir allá; `AppShell` lo importa).

```ts
/**
 * El mapa entre la barra de direcciones y la pantalla que se ve.
 *
 * Vive aparte de React a propósito: es la única pieza de navegación que puede
 * probarse sin montar nada, y es la que define qué URLs existen. Antes de esto
 * la app entera respondía en "/" y el botón "atrás" del navegador sacaba del
 * sitio, porque nunca había una segunda entrada en el historial.
 */
export type View =
  | { screen: "login" }
  | { screen: "changePassword" }
  | { screen: "dashboard" }
  | { screen: "finance" }
  | { screen: "announcements" }
  | { screen: "payments" }
  | { screen: "config" }
  | { screen: "students" }
  | { screen: "grades" }
  | { screen: "attendance" }
  | { screen: "teachers" }
  | { screen: "uniforms" }
  | { screen: "admin" }
  | { screen: "module"; moduleId: string };

/** El prefijo de un módulo que aún no tiene pantalla propia. */
const MODULE_PREFIX = "/modulo/";

/**
 * Las pantallas de una sola pieza. Los módulos sin backend van por
 * MODULE_PREFIX, que lleva un id variable y no cabe en una tabla.
 */
const BY_SCREEN = {
  login: "/entrar",
  changePassword: "/cambiar-contrasena",
  dashboard: "/",
  finance: "/finanzas",
  announcements: "/avisos",
  payments: "/pagos",
  config: "/configuracion",
  students: "/alumnos",
  grades: "/calificaciones",
  attendance: "/asistencia",
  teachers: "/docentes",
  uniforms: "/uniformes",
  admin: "/administracion",
} as const satisfies Record<Exclude<View["screen"], "module">, string>;

const BY_PATH: ReadonlyMap<string, View> = new Map(
  Object.entries(BY_SCREEN).map(([screen, path]) => [path, { screen } as View]),
);

/** La dirección que le corresponde a una pantalla. */
export function viewToPath(view: View): string {
  if (view.screen === "module") {
    return `${MODULE_PREFIX}${view.moduleId}`;
  }
  return BY_SCREEN[view.screen];
}

/**
 * La pantalla que le corresponde a una dirección. Una dirección que no
 * reconocemos aterriza en el tablero: una pantalla en blanco por un dedazo en
 * la URL es peor que llegar a casa.
 */
export function pathToView(pathname: string): View {
  const clean = pathname.length > 1 && pathname.endsWith("/")
    ? pathname.slice(0, -1)
    : pathname;

  const known = BY_PATH.get(clean);
  if (known !== undefined) {
    return known;
  }

  if (clean.startsWith(MODULE_PREFIX)) {
    const moduleId = clean.slice(MODULE_PREFIX.length);
    if (moduleId.length > 0 && !moduleId.includes("/")) {
      return { screen: "module", moduleId };
    }
  }

  return { screen: "dashboard" };
}
```

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/routing/routes.test.ts`
Expected: PASS, 17 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/routing/
git commit -m "feat(web): el mapa entre la URL y la pantalla"
```

---

### Task 2: El catch-all, y los dos archivos que mueren

**Files:**
- Create: `apps/web/app/[[...slug]]/page.tsx`
- Delete: `apps/web/app/page.tsx`, `apps/web/app/(auth)/login/page.tsx`
- Modify: `apps/web/app/page.test.tsx` → `apps/web/app/[[...slug]]/page.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

Mover `app/page.test.tsx` a `app/[[...slug]]/page.test.tsx`, ajustar el import a `./page`, y añadir:

```tsx
it("sirve la misma cáscara en cualquier ruta del shell", () => {
  // El catch-all no mira el slug: AppShell lee la URL por su cuenta. Lo que
  // esta prueba defiende es que la ruta EXISTE — antes de esto /finanzas era
  // un 404 y F5 devolvía al tablero.
  render(<HomePage />);
  expect(screen.getByRole("main", { name: /entrar|inicio/i })).toBeTruthy();
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run "app/[[...slug]]/page.test.tsx"`
Expected: FAIL — el archivo `./page` no existe todavía.

- [ ] **Step 3: Implementar**

```bash
git mv apps/web/app/page.tsx "apps/web/app/[[...slug]]/page.tsx"
git rm -r "apps/web/app/(auth)"
```

Y reescribir el comentario de `[[...slug]]/page.tsx`:

```tsx
import type { ReactNode } from "react";
import { AppShell } from "../AppShell";

/**
 * Punto de entrada de la web, en CUALQUIER ruta del shell.
 *
 * Es un catch-all opcional a propósito: `/`, `/finanzas`, `/alumnos` y las
 * demás son la misma aplicación, y quien decide qué pintar es <AppShell> al
 * leer la dirección (app/routing/routes.ts). Antes vivía todo en "/" con la
 * pantalla en useState, y por eso el botón "atrás" del navegador sacaba del
 * sitio y F5 devolvía al tablero.
 *
 * Las rutas más específicas que ya existen — /activar, /healthz, /api/* —
 * ganan sobre este catch-all; así resuelve Next y por eso no hay conflicto.
 *
 * La experiencia del padre/tutor es la app móvil y NO vive aquí.
 */
export default function HomePage(): ReactNode {
  return <AppShell />;
}
```

- [ ] **Step 4: Verificar que las rutas vecinas siguen vivas**

Run: `pnpm build`
Expected: PASS. En la tabla de rutas deben aparecer `/activar`, `/healthz`, `/api/[...path]` **y** `/[[...slug]]`. Si `/activar` desapareció, el catch-all lo está tapando y hay que detenerse aquí.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web/app
git commit -m "feat(web): toda pantalla responde en su propia URL

Borra el stub muerto de FASE 0 en (auth)/login, que servía
'Stub — la autenticación llega con el módulo IAM' a quien tecleara /login."
```

---

### Task 3: `AppShell` deja de recordar la pantalla

**Files:**
- Modify: `apps/web/app/AppShell.tsx`
- Test: `apps/web/app/AppShell.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

```tsx
it("al abrir un módulo cambia la dirección, no un useState", async () => {
  const push = vi.fn();
  vi.mocked(useRouter).mockReturnValue({ push, replace: vi.fn() } as never);
  vi.mocked(usePathname).mockReturnValue("/");

  render(<AppShell />);
  await screen.findByRole("heading", { name: /hola/i });
  await userEvent.click(screen.getByRole("button", { name: /finanzas/i }));

  expect(push).toHaveBeenCalledWith("/finanzas");
});

it("pinta la pantalla que dice la dirección, no la última visitada", async () => {
  vi.mocked(usePathname).mockReturnValue("/finanzas");
  render(<AppShell />);
  expect(await screen.findByRole("heading", { name: /adeudos/i })).toBeTruthy();
});
```

Con `vi.mock("next/navigation", () => ({ useRouter: vi.fn(), usePathname: vi.fn() }))` arriba.

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/AppShell.test.tsx`
Expected: FAIL — `push` nunca se llama; la pantalla sigue saliendo de `useState`.

- [ ] **Step 3: Implementar**

En `AppShell.tsx`:
1. Borrar el bloque `type View = …` (líneas 34-49); importarlo de `./routing/routes`.
2. Sustituir `const [view, setView] = useState<View>({ screen: "login" })` por:

```tsx
const router = useRouter();
const pathname = usePathname();
const view = pathToView(pathname);
// `replace` cuando la pantalla no es un destino al que valga la pena regresar
// (el login tras cerrar sesión, el cambio forzado de contraseña): dejarlas en
// el historial haría que "atrás" reviviera una pantalla ya resuelta.
const go = useCallback((next: View): void => { router.push(viewToPath(next)); }, [router]);
const goReplacing = useCallback((next: View): void => { router.replace(viewToPath(next)); }, [router]);
```

3. Cada `setView({...})` pasa a `go({...})`, **salvo** estos tres, que usan `goReplacing`:
   - el `setView({ screen: "login" })` de `logout()`
   - el `setView({ screen: "login" })` del `onSessionLost`
   - el `setView({ screen: "changePassword" })` de `handleLogin`
4. En el `useEffect` de restauración, los dos `setView({ screen: "dashboard" })` se
   eliminan: si la sesión revive, la pantalla ya la decide la URL. Sólo se conserva el
   caso "no hay sesión restaurable" → `goReplacing({ screen: "login" })`.
5. Los `if (view.screen === "login")` y `"changePassword"` de arriba no cambian.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/AppShell.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/AppShell.tsx apps/web/app/AppShell.test.tsx
git commit -m "feat(web): la URL manda sobre la pantalla, no useState"
```

---

### Task 4: Un link directo sin sesión aterriza donde lo pidieron

**Files:**
- Modify: `apps/web/app/AppShell.tsx`
- Test: `apps/web/app/AppShell.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

```tsx
it("tras entrar, devuelve al módulo que se pidió por link, no al tablero", async () => {
  const replace = vi.fn();
  vi.mocked(useRouter).mockReturnValue({ push: vi.fn(), replace } as never);
  vi.mocked(usePathname).mockReturnValue("/finanzas");
  server.use(rest.get("*/me", (_, res, ctx) => res(ctx.status(401))));

  render(<AppShell />);
  await screen.findByLabelText(/correo/i);
  expect(replace).toHaveBeenCalledWith("/entrar");

  await signIn(screen);
  expect(replace).toHaveBeenLastCalledWith("/finanzas");
});

it("un destino pendiente nunca sale de la lista de rutas conocidas", async () => {
  // El destino se guarda como View ya resuelta, no como texto crudo, así que
  // no hay forma de que un link externo empuje al usuario fuera del sitio.
  expect(viewToPath(pathToView("//evil.example.com"))).toBe("/");
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/AppShell.test.tsx -t "link"`
Expected: FAIL — tras entrar aterriza en `/`.

- [ ] **Step 3: Implementar**

```tsx
// A dónde iba quien tocó un link directo sin sesión viva. Se guarda como View
// YA RESUELTA, no como el texto de la URL: así el destino sólo puede ser una de
// las rutas que routes.ts reconoce, y un link de fuera no puede empujar a nadie
// a otro sitio (redirección abierta). Vive en memoria del cliente — nunca en la
// barra de direcciones, que es donde un ?next= sería copiable y compartible.
const pending = useRef<View | undefined>(undefined);
```

En el `useEffect` de restauración, en la rama "no hay sesión":

```tsx
const asked = pathToView(window.location.pathname);
if (asked.screen !== "login" && asked.screen !== "dashboard") {
  pending.current = asked;
}
goReplacing({ screen: "login" });
```

Y al final de `handleLogin`, sustituir `setView({ screen: "dashboard" })` por:

```tsx
const target = pending.current ?? { screen: "dashboard" as const };
pending.current = undefined;
goReplacing(target);
```

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/AppShell.test.tsx`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/AppShell.tsx apps/web/app/AppShell.test.tsx
git commit -m "feat(web): un link directo sobrevive al login"
```

---

### Task 5: e2e de navegación

**Files:**
- Create: `apps/web/e2e/navigation.spec.ts`

- [ ] **Step 1: Escribir la prueba**

```ts
import { expect, test } from "@playwright/test";
import { openModule, signIn } from "./helpers";

/**
 * El botón "atrás" del navegador. Vale la pena decir por qué existe esta suite:
 * hasta septiembre de 2026 toda la web vivía en "/" con la pantalla en un
 * useState, así que "atrás" no tenía a dónde ir y sacaba del sitio. Nada de
 * esto se puede defender en jsdom — no hay historial que recorrer.
 */
test.describe("la navegación del navegador", () => {
  test("atrás y adelante se mueven dentro del sitio", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Finanzas");
    await expect(page).toHaveURL(/\/finanzas$/);

    await page.goBack();
    await expect(page).toHaveURL(/\/$/);
    await expect(page.getByRole("heading", { name: /hola/i })).toBeVisible();

    await page.goForward();
    await expect(page).toHaveURL(/\/finanzas$/);
  });

  test("recargar conserva el lugar", async ({ page }) => {
    await signIn(page);
    await openModule(page, "Finanzas");
    await page.reload();
    await expect(page).toHaveURL(/\/finanzas$/);
    await expect(page.getByRole("heading", { name: /adeudos/i })).toBeVisible();
  });

  test("un link directo abre el módulo", async ({ page }) => {
    await signIn(page);
    await page.goto("/alumnos");
    await expect(page.getByRole("heading", { name: /alumnos/i })).toBeVisible();
  });

  test("las rutas vecinas siguen siendo suyas", async ({ page }) => {
    const health = await page.request.get("/healthz");
    expect(health.ok()).toBe(true);
    await page.goto("/activar");
    await expect(page.getByText(/stub/i)).toHaveCount(0);
  });
});
```

- [ ] **Step 2: Correr contra el stack local**

Run: `pnpm e2e -- e2e/navigation.spec.ts`
Expected: PASS, 4 tests.

- [ ] **Step 3: Commit**

```bash
git add apps/web/e2e/navigation.spec.ts
git commit -m "test(web): atrás, adelante, F5 y link directo"
```

---

## BLOQUE B — La capa de formulario

### Task 6: Un solo formateador de dinero

**Files:**
- Move: `app/_screens/finance/money.ts` → `app/_lib/money.ts` (y su `.test.ts`)
- Modify: `app/_screens/Dashboard.tsx:14`, `app/_screens/uniforms/parts.tsx:17`, y todo importador de `./money`

- [ ] **Step 1: Escribir el test que falla**

Añadir a `app/_lib/money.test.ts`:

```ts
it("el tablero ya no redondea a pesos enteros", () => {
  // Dashboard.tsx tenía su propio money() con Math.round(cents/100): $1,850.50
  // se mostraba como $1,851. Un peso de diferencia en un saldo es un reclamo
  // en ventanilla.
  expect(formatCents(185050)).toBe("$1,850.50");
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_lib/money.test.ts`
Expected: FAIL — el archivo no existe aún.

- [ ] **Step 3: Implementar**

```bash
mkdir -p apps/web/app/_lib
git mv apps/web/app/_screens/finance/money.ts apps/web/app/_lib/money.ts
git mv apps/web/app/_screens/finance/money.test.ts apps/web/app/_lib/money.test.ts
```

Borrar `const money = …` de `Dashboard.tsx:14` y de `uniforms/parts.tsx:17`; ambos importan `formatCents` de `../_lib/money`. Reapuntar los imports `./money` de finanzas a `../../_lib/money`.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run && pnpm typecheck`
Expected: PASS. Cero referencias a `_screens/finance/money`: `grep -rn "finance/money" apps/web/app` debe salir vacío.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web/app
git commit -m "refactor(web): un solo formateador de dinero

El tablero redondeaba a pesos enteros con su propio money(): \$1,850.50
se mostraba \$1,851."
```

---

### Task 7: `useRequired` — qué falta y a dónde saltar

**Files:**
- Create: `apps/web/app/_design/form/useRequired.ts`
- Test: `apps/web/app/_design/form/useRequired.test.ts`

- [ ] **Step 1: Escribir el test que falla**

```ts
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useRequired } from "./useRequired";

describe("useRequired", () => {
  it("no reclama nada antes de guardar", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    expect(result.current.errors).toEqual({});
  });

  it("al guardar marca TODOS los faltantes a la vez", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ nombre: "", fecha: "", nota: "" }); });
    expect(Object.keys(result.current.errors).sort()).toEqual(["fecha", "nombre"]);
  });

  it("no reclama un campo opcional vacío", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    act(() => { result.current.check({ nombre: "Ana", nota: "" }); });
    expect(result.current.errors).toEqual({});
  });

  it("el espacio en blanco no llena un campo", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    act(() => { result.current.check({ nombre: "   " }); });
    expect(result.current.errors.nombre).toBeDefined();
  });

  it("check devuelve si se puede guardar", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    let ok = false;
    act(() => { ok = result.current.check({ nombre: "" }); });
    expect(ok).toBe(false);
    act(() => { ok = result.current.check({ nombre: "Ana" }); });
    expect(ok).toBe(true);
  });

  it("escribir en un campo limpia SU rojo y deja los demás", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ nombre: "", fecha: "" }); });
    act(() => { result.current.clear("nombre"); });
    expect(result.current.errors.nombre).toBeUndefined();
    expect(result.current.errors.fecha).toBeDefined();
  });

  it("nombra el primer faltante en el orden declarado, no el del objeto", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ fecha: "", nombre: "" }); });
    expect(result.current.firstMissing).toBe("nombre");
  });

  it("cuenta cuántos faltan, para el resumen de arriba", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ nombre: "", fecha: "" }); });
    expect(result.current.missingCount).toBe(2);
  });
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/form/useRequired.test.ts`
Expected: FAIL — no existe `./useRequired`.

- [ ] **Step 3: Implementar**

```ts
"use client";

import { useCallback, useState } from "react";

/** El mensaje que se pinta bajo un campo obligatorio vacío. */
const FALTA = "Falta este dato";

export interface RequiredState {
  /** Los campos marcados en rojo ahora mismo, por clave. */
  readonly errors: Readonly<Record<string, string>>;
  /** El primero que falta EN EL ORDEN DECLARADO — a donde salta la vista. */
  readonly firstMissing: string | undefined;
  /** Cuántos faltan, para el resumen de arriba. */
  readonly missingCount: number;
  /** Revisa y marca. Devuelve true si se puede guardar. */
  check: (values: Readonly<Record<string, string>>) => boolean;
  /** Limpia el rojo de un campo — se llama desde su onChange. */
  clear: (key: string) => void;
}

/**
 * Qué falta para poder guardar.
 *
 * El momento importa: NO reclama mientras se captura, sólo al guardar, y
 * entonces marca todos los faltantes a la vez. Reclamar campo por campo al
 * salir de cada uno persigue con rojos a quien apenas va llegando al siguiente.
 *
 * `firstMissing` respeta el orden en que se DECLARARON los obligatorios, no el
 * orden de las llaves del objeto: la vista debe saltar al primero que el ojo
 * encuentra bajando por la forma, y el orden de un objeto no es el de la
 * pantalla.
 */
export function useRequired(required: readonly string[]): RequiredState {
  const [errors, setErrors] = useState<Record<string, string>>({});

  const check = useCallback(
    (values: Readonly<Record<string, string>>): boolean => {
      const found: Record<string, string> = {};
      for (const key of required) {
        if ((values[key] ?? "").trim() === "") {
          found[key] = FALTA;
        }
      }
      setErrors(found);
      return Object.keys(found).length === 0;
    },
    [required],
  );

  const clear = useCallback((key: string): void => {
    setErrors((prev) => {
      if (prev[key] === undefined) return prev;
      const { [key]: _dropped, ...rest } = prev;
      return rest;
    });
  }, []);

  return {
    errors,
    firstMissing: required.find((key) => errors[key] !== undefined),
    missingCount: Object.keys(errors).length,
    check,
    clear,
  };
}
```

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/_design/form/useRequired.test.ts`
Expected: PASS, 8 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/_design/form/useRequired.ts apps/web/app/_design/form/useRequired.test.ts
git commit -m "feat(web): useRequired — marca todos los faltantes al guardar"
```

---

### Task 8: `Field` — la envoltura con `*`, rojo y ARIA

**Files:**
- Create: `apps/web/app/_design/form/Field.tsx`
- Test: `apps/web/app/_design/form/Field.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

```tsx
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Field } from "./Field";

describe("Field", () => {
  const child = (id: string, describedBy?: string) => (
    <input id={id} aria-describedby={describedBy} />
  );

  it("pinta el asterisco cuando es obligatorio", () => {
    render(<Field label="Nombre" required>{child}</Field>);
    expect(screen.getByText("*")).toBeTruthy();
  });

  it("no lo pinta cuando es opcional", () => {
    render(<Field label="Nota">{child}</Field>);
    expect(screen.queryByText("*")).toBeNull();
  });

  it("el asterisco no se lee dos veces", () => {
    // aria-required ya dice que es obligatorio; el * es sólo para el ojo.
    render(<Field label="Nombre" required>{child}</Field>);
    expect(screen.getByText("*")).toHaveAttribute("aria-hidden", "true");
  });

  it("con error, liga el mensaje al input por aria-describedby", () => {
    render(<Field label="Fecha" required error="Falta este dato">{child}</Field>);
    const input = screen.getByLabelText(/fecha/i);
    const msg = screen.getByText("Falta este dato");
    expect(input.getAttribute("aria-describedby")).toBe(msg.id);
  });

  it("sin error no hay mensaje colgando", () => {
    render(<Field label="Fecha">{child}</Field>);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/form/Field.test.tsx`
Expected: FAIL — no existe `./Field`.

- [ ] **Step 3: Implementar**

`Field` recibe sus hijos como **función** `(id, describedBy) => ReactElement` para poder
ligar `id` y `aria-describedby` sin clonar elementos.

```tsx
"use client";

import { useId, type CSSProperties, type ReactElement } from "react";
import { Icon } from "../Icon";

/**
 * La envoltura de todo campo del sistema: etiqueta, el asterisco de
 * obligatorio, el borde rojo y el mensaje ligado por ARIA.
 *
 * Existe para que "requerido" y "en rojo" signifiquen lo mismo en las 23
 * pantallas. Antes cada una dibujaba su propio input en línea y sólo dos
 * lugares en toda la web decían `required`.
 */
export interface FieldProps {
  label: string;
  required?: boolean;
  error?: string;
  hint?: string;
  children: (id: string, describedBy: string | undefined) => ReactElement;
}

const labelStyle: CSSProperties = {
  display: "block",
  fontSize: 13,
  fontWeight: 600,
  color: "var(--text-muted)",
  marginBottom: 6,
};

export function Field({ label, required, error, hint, children }: FieldProps): ReactElement {
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const describedBy = error !== undefined ? errorId : hint !== undefined ? hintId : undefined;

  return (
    <div style={{ display: "block", minWidth: 0 }}>
      <label htmlFor={id} style={labelStyle}>
        {label}
        {required === true && (
          <span aria-hidden="true" style={{ color: "var(--red)", marginLeft: 3 }}>*</span>
        )}
      </label>
      {children(id, describedBy)}
      {hint !== undefined && error === undefined && (
        <p id={hintId} style={{ fontSize: 12.5, color: "var(--text-faint)", marginTop: 5 }}>{hint}</p>
      )}
      {error !== undefined && (
        <p
          id={errorId}
          role="alert"
          style={{ display: "flex", alignItems: "center", gap: 5, fontSize: 12.5, fontWeight: 600, color: "var(--bad-fg)", marginTop: 5 }}
        >
          <Icon name="warning" size={14} stroke={2.4} />
          {error}
        </p>
      )}
    </div>
  );
}

/**
 * El aspecto de un input. El rojo NO es sólo el borde: lleva halo, porque un
 * borde de 1px en una forma de veinte campos no se encuentra de un vistazo, y
 * encontrarlo rápido es justamente lo que se pidió.
 */
export function inputStyle(invalid: boolean): CSSProperties {
  return {
    height: 44,
    padding: "0 14px",
    borderRadius: "var(--r-md)",
    border: `1px solid ${invalid ? "var(--red)" : "var(--border)"}`,
    boxShadow: invalid ? "0 0 0 3px color-mix(in oklab, var(--red) 22%, transparent)" : "none",
    background: "var(--surface)",
    color: "var(--text)",
    fontFamily: "var(--font-ui)",
    fontSize: 15,
    outline: "none",
    width: "100%",
    minWidth: 0,
  };
}
```

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/_design/form/Field.test.tsx`
Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/_design/form/Field.tsx apps/web/app/_design/form/Field.test.tsx
git commit -m "feat(web): Field — asterisco, borde rojo con halo y ARIA"
```

---

### Task 9: Texto, correo, número, área y selector

**Files:**
- Create: `apps/web/app/_design/form/fields.tsx`
- Test: `apps/web/app/_design/form/fields.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

```tsx
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { EmailField, TextArea, TextField } from "./fields";

describe("TextField", () => {
  it("con uppercase, lo capturado sube a mayúsculas", async () => {
    const onChange = vi.fn();
    render(<TextField label="Alumno" value="" onChange={onChange} uppercase />);
    await userEvent.type(screen.getByLabelText(/alumno/i), "a");
    expect(onChange).toHaveBeenCalledWith("A");
  });

  it("respeta los acentos al subir", async () => {
    const onChange = vi.fn();
    render(<TextField label="Alumno" value="" onChange={onChange} uppercase />);
    await userEvent.type(screen.getByLabelText(/alumno/i), "á");
    expect(onChange).toHaveBeenCalledWith("Á");
  });

  it("sin uppercase no toca lo que se escribe", async () => {
    const onChange = vi.fn();
    render(<TextField label="Nota" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText(/nota/i), "a");
    expect(onChange).toHaveBeenCalledWith("a");
  });

  it("transforma al capturar, no al pintar", async () => {
    // Guardar tal cual y sólo PINTAR en mayúsculas dejaría "Ana Ruiz" y
    // "ana ruiz" idénticos en pantalla y distintos en la base.
    const onChange = vi.fn();
    render(<TextField label="Alumno" value="" onChange={onChange} uppercase />);
    await userEvent.type(screen.getByLabelText(/alumno/i), "ana");
    expect(onChange).toHaveBeenLastCalledWith("A");
    expect(screen.getByLabelText(/alumno/i)).not.toHaveStyle({ textTransform: "uppercase" });
  });

  it("al escribir avisa que se limpie su rojo", async () => {
    const onClearError = vi.fn();
    render(<TextField label="Alumno" value="" onChange={vi.fn()} error="Falta este dato" onClearError={onClearError} />);
    await userEvent.type(screen.getByLabelText(/alumno/i), "x");
    expect(onClearError).toHaveBeenCalled();
  });

  it("con error se marca inválido para el lector de pantalla", () => {
    render(<TextField label="Alumno" value="" onChange={vi.fn()} error="Falta este dato" />);
    expect(screen.getByLabelText(/alumno/i)).toHaveAttribute("aria-invalid", "true");
  });
});

describe("EmailField", () => {
  it("SIEMPRE baja a minúsculas", async () => {
    const onChange = vi.fn();
    render(<EmailField label="Correo" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText(/correo/i), "A");
    expect(onChange).toHaveBeenCalledWith("a");
  });

  it("no deja que el teclado del sistema ponga mayúscula inicial", () => {
    render(<EmailField label="Correo" value="" onChange={vi.fn()} />);
    const input = screen.getByLabelText(/correo/i);
    expect(input).toHaveAttribute("autocapitalize", "none");
    expect(input).toHaveAttribute("spellcheck", "false");
    expect(input).toHaveAttribute("type", "email");
  });
});

describe("TextArea", () => {
  it("NO sube a mayúsculas — un aviso a los papás no se grita", async () => {
    const onChange = vi.fn();
    render(<TextArea label="Mensaje" value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText(/mensaje/i), "h");
    expect(onChange).toHaveBeenCalledWith("h");
  });
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/form/fields.test.tsx`
Expected: FAIL — no existe `./fields`.

- [ ] **Step 3: Implementar**

Firma común a todas: `{ label, value, onChange, required?, error?, onClearError?, placeholder?, hint?, disabled? }`.
`TextField` añade `uppercase?: boolean`. `SelectField` añade `options`. `NumberField` añade `min?`/`max?`/`step?`.
Cada una envuelve en `<Field>` y usa `inputStyle(error !== undefined)`.
El `onChange` interno hace: transformar → `onChange(transformado)` → `onClearError?.()`.
`TextArea` usa `<textarea rows={5}>` con `height: "auto"` y `padding: "12px 14px"` sobre `inputStyle`.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/_design/form/fields.test.tsx`
Expected: PASS, 9 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/_design/form/fields.tsx apps/web/app/_design/form/fields.test.tsx
git commit -m "feat(web): texto en mayúsculas, correo en minúsculas, aviso sin gritar"
```

---

### Task 10: `DateField` — calendario, no un `aaaa-mm-dd` tecleado

**Files:**
- Create: `apps/web/app/_design/form/DateField.tsx`
- Test: `apps/web/app/_design/form/DateField.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

```tsx
it("es un campo de fecha de verdad, con su calendario", () => {
  render(<DateField label="Fecha de nacimiento" value="" onChange={vi.fn()} />);
  expect(screen.getByLabelText(/fecha de nacimiento/i)).toHaveAttribute("type", "date");
});

it("entrega la fecha como aaaa-mm-dd, que es lo que el API espera", async () => {
  const onChange = vi.fn();
  render(<DateField label="Fecha" value="" onChange={onChange} />);
  await userEvent.type(screen.getByLabelText(/fecha/i), "2026-09-20");
  expect(onChange).toHaveBeenLastCalledWith("2026-09-20");
});

it("acepta un rango cuando se le da", () => {
  render(<DateField label="Fecha" value="" onChange={vi.fn()} min="2026-01-01" max="2026-12-31" />);
  const input = screen.getByLabelText(/fecha/i);
  expect(input).toHaveAttribute("min", "2026-01-01");
  expect(input).toHaveAttribute("max", "2026-12-31");
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/form/DateField.test.tsx`
Expected: FAIL — no existe `./DateField`.

- [ ] **Step 3: Implementar**

`<input type="date">` dentro de `<Field>`. El navegador lo pinta `dd/mm/aaaa` en es-MX y
abre su calendario; el `value` sigue siendo ISO `aaaa-mm-dd`, que es lo que el API recibe
hoy — no hay conversión y no hay zona horaria de por medio. Props extra: `min`, `max`.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/_design/form/DateField.test.tsx`
Expected: PASS, 3 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/_design/form/DateField.tsx apps/web/app/_design/form/DateField.test.tsx
git commit -m "feat(web): toda fecha se captura con calendario"
```

---

### Task 11: `MoneyField` — pesos a la vista, centavos por dentro

**Files:**
- Create: `apps/web/app/_design/form/MoneyField.tsx`
- Test: `apps/web/app/_design/form/MoneyField.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

```tsx
it("muestra el signo de pesos sin meterlo en lo que se teclea", () => {
  render(<MoneyField label="Monto" value="" onChange={vi.fn()} />);
  expect(screen.getByText("$")).toBeTruthy();
  expect(screen.getByLabelText(/monto/i)).toHaveValue("");
});

it("al salir del campo, formatea con separador de miles", async () => {
  const onChange = vi.fn();
  render(<MoneyField label="Monto" value="1850" onChange={onChange} />);
  await userEvent.click(screen.getByLabelText(/monto/i));
  await userEvent.tab();
  expect(onChange).toHaveBeenLastCalledWith("1,850.00");
});

it("no formatea lo que no es un monto — lo deja para que se corrija", async () => {
  const onChange = vi.fn();
  render(<MoneyField label="Monto" value="abc" onChange={onChange} />);
  await userEvent.click(screen.getByLabelText(/monto/i));
  await userEvent.tab();
  expect(onChange).not.toHaveBeenCalled();
});

it("la coma decimal no se lee como separador de miles", async () => {
  // pesosToCents es estricto justo por esto: "1850,50" leído a la ligera se
  // volvía 185050 pesos — un cobro 100 veces mayor.
  const onChange = vi.fn();
  render(<MoneyField label="Monto" value="1850,50" onChange={onChange} />);
  await userEvent.click(screen.getByLabelText(/monto/i));
  await userEvent.tab();
  expect(onChange).not.toHaveBeenCalled();
});

it("acepta decimales sin tocarlos", async () => {
  const onChange = vi.fn();
  render(<MoneyField label="Monto" value="1850.5" onChange={onChange} />);
  await userEvent.click(screen.getByLabelText(/monto/i));
  await userEvent.tab();
  expect(onChange).toHaveBeenLastCalledWith("1,850.50");
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/form/MoneyField.test.tsx`
Expected: FAIL — no existe `./MoneyField`.

- [ ] **Step 3: Implementar**

`<Field>` + un contenedor `position: relative` con un `<span aria-hidden>$</span>` a la
izquierda y `paddingLeft: 30` en el input. `inputMode="decimal"`.
En `onBlur`: `pesosToCents(value)` de `app/_lib/money.ts`; si es `NaN`, **no se toca el
valor** (que el usuario vea lo que escribió y lo corrija); si no, se reemite
`centsToPesosInput(cents)` con separador de miles. **Nunca** se emite un float al API: el
consumidor llama `pesosToCents` al guardar, igual que hoy.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/_design/form/MoneyField.test.tsx`
Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/_design/form/MoneyField.tsx apps/web/app/_design/form/MoneyField.test.tsx
git commit -m "feat(web): los montos se capturan y se ven en pesos"
```

---

### Task 12: La superficie pública, y config migra primero

**Files:**
- Create: `apps/web/app/_design/form/index.ts`
- Modify: `app/_screens/config/parts.tsx` (sus primitivas mueren), `CycleBar.tsx`, `CalendarPanel.tsx`, `TermsPanel.tsx`, `TuitionPanel.tsx`, `LevelsPanel.tsx`, `GradeLevelsPanel.tsx`, `SubjectsPanel.tsx`, `GroupsPanel.tsx`

- [ ] **Step 1: Escribir los tests que fallan**

En `app/_screens/config/CalendarPanel.test.tsx`:

```tsx
it("la fecha de un día cerrado se elige en calendario, no se teclea", () => {
  render(<CalendarPanel cycle={cycle} />);
  expect(screen.getByLabelText(/^fecha$/i)).toHaveAttribute("type", "date");
});

it("el nombre del día cerrado es obligatorio y se marca al guardar", async () => {
  render(<CalendarPanel cycle={cycle} />);
  await userEvent.click(screen.getByRole("button", { name: /nuevo día cerrado/i }));
  await userEvent.click(screen.getByRole("button", { name: /guardar/i }));
  expect(screen.getByLabelText(/nombre/i)).toHaveAttribute("aria-invalid", "true");
});
```

En `TuitionPanel.test.tsx`:

```tsx
it("la fecha límite de inscripción se elige en calendario", () => {
  render(<TuitionPanel cycle={cycle} levels={levels} />);
  expect(screen.getByLabelText(/fecha límite de inscripción/i)).toHaveAttribute("type", "date");
});

it("los montos del tabulador se capturan en pesos", () => {
  render(<TuitionPanel cycle={cycle} levels={levels} />);
  expect(screen.getAllByText("$").length).toBeGreaterThan(0);
});
```

- [ ] **Step 2: Correr y verlos fallar**

Run: `pnpm vitest run app/_screens/config`
Expected: FAIL — hoy son `TextField` de texto plano.

- [ ] **Step 3: Implementar**

`index.ts` reexporta `Field`, `inputStyle`, `useRequired`, `TextField`, `EmailField`,
`NumberField`, `TextArea`, `SelectField`, `DateField`, `MoneyField`.

En `config/parts.tsx` **se borran** `TextField`, `NumberField`, `DateField`, `SelectField`
e `inputStyle`; queda sólo `Section`. Los 8 paneles importan de `../../_design/form`.

Cambios de sustancia (no sólo de import):
- `CalendarPanel.tsx:244` y su gemelo de actividades: `TextField` → `DateField`.
- `TuitionPanel.tsx:578` *"Fecha límite de inscripción"*: `TextField` → `DateField`;
  **borrar** la validación de formato de `TuitionPanel.tsx:507` y su mensaje *"se escribe
  aaaa-mm-dd"* — con calendario ese error ya no puede ocurrir.
- `TuitionPanel`: los `TextField` de montos (`Colegiatura mensual`, `Recargo por día`,
  `Pronto pago (monto)`, y los de tramos) → `MoneyField`.
- `CycleBar.tsx`, `LevelsPanel`, `GradeLevelsPanel`, `SubjectsPanel`, `GroupsPanel`,
  `TermsPanel`: los campos de nombre llevan `uppercase` y `required`; cada formulario usa
  `useRequired` y marca al guardar.
- Las rejillas `1fr 1fr` de `CycleBar.tsx:138`, `GroupsPanel.tsx:88`, `TermsPanel.tsx:90`
  → `repeat(auto-fit, minmax(240px, 1fr))`.

- [ ] **Step 4: Correr y verlos pasar**

Run: `pnpm vitest run app/_screens/config && pnpm typecheck`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web/app
git commit -m "feat(web): configuración captura por la capa común

Las fechas dejan de teclearse como texto; muere la validación
'se escribe aaaa-mm-dd', que con calendario ya no puede ocurrir."
```

---

### Task 13: Alumnos, finanzas y ventanilla migran

**Files:**
- Modify: `students/StudentsScreen.tsx`, `students/ContactsPanel.tsx`, `students/RosterPanel.tsx`, `students/StudentDirectoryPanel.tsx`, `finance/FinanceScreen.tsx`, `finance/CollectDrawer.tsx`, `finance/ConceptsPanel.tsx`, `finance/ChargeActions.tsx`, `finance/GeneratePanel.tsx`, `finance/PaymentPanel.tsx`, `finance/AccountPanel.tsx`, `payments/PaymentsScreen.tsx`

- [ ] **Step 1: Escribir los tests que fallan**

En `StudentsScreen.test.tsx`:

```tsx
it("el alta de alumno marca en rojo lo que falta al guardar", async () => {
  render(<StudentsScreen onBack={vi.fn()} />);
  await userEvent.click(await screen.findByRole("button", { name: /nuevo alumno/i }));
  await userEvent.click(screen.getByRole("button", { name: /guardar/i }));
  expect(screen.getByLabelText(/nombre/i)).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByRole("alert", { name: /faltan/i })).toBeTruthy();
});

it("el nombre del alumno se captura en mayúsculas", async () => {
  render(<StudentsScreen onBack={vi.fn()} />);
  await userEvent.click(await screen.findByRole("button", { name: /nuevo alumno/i }));
  const nombre = screen.getByLabelText(/nombre/i);
  await userEvent.type(nombre, "ana");
  expect(nombre).toHaveValue("ANA");
});

it("la fecha de nacimiento se elige en calendario", async () => {
  render(<StudentsScreen onBack={vi.fn()} />);
  await userEvent.click(await screen.findByRole("button", { name: /nuevo alumno/i }));
  expect(screen.getByLabelText(/nacimiento/i)).toHaveAttribute("type", "date");
});
```

En `CollectDrawer.test.tsx`:

```tsx
it("el monto recibido se captura en pesos", () => {
  render(<CollectDrawer student={student} onClose={vi.fn()} onCollected={vi.fn()} />);
  expect(screen.getByText("$")).toBeTruthy();
});
```

- [ ] **Step 2: Correr y verlos fallar**

Run: `pnpm vitest run app/_screens/students app/_screens/finance app/_screens/payments`
Expected: FAIL.

- [ ] **Step 3: Implementar**

Sustituir cada `<input style={field}>` por la primitiva que corresponda.
Reglas fijas: nombres de persona y referencias → `TextField uppercase`; correos →
`EmailField`; fechas → `DateField`; montos → `MoneyField`.
Cada formulario con obligatorios usa `useRequired` y, al enviar, si `check()` devuelve
`false`, hace `document.getElementById(firstMissing)?.scrollIntoView({ block: "center" })`
y no llama al API.
Borrar los `maxWidth` internos de `AccountPanel.tsx:53`, `GeneratePanel.tsx:58`,
`PaymentPanel.tsx:97` y `StudentDirectoryPanel.tsx:54` (H6 del spec).
Las rejillas `1fr 1fr` de `StudentsScreen.tsx:271/275`, `RosterPanel.tsx:101` y
`EnrollmentRow.tsx:74` → `repeat(auto-fit, minmax(240px, 1fr))`.

- [ ] **Step 4: Correr y verlos pasar**

Run: `pnpm vitest run app/_screens && pnpm typecheck`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web/app
git commit -m "feat(web): alumnos, finanzas y ventanilla capturan por la capa común"
```

---

### Task 14: Administración, docentes, uniformes, avisos, calificaciones y asistencia

**Files:**
- Modify: `admin/AdminScreen.tsx`, `admin/UsersPanel.tsx`, `admin/RolesPanel.tsx`, `admin/TeacherLinkPanel.tsx`, `admin/parts.tsx`, `teachers/TeachersPanel.tsx`, `teachers/AssignmentsPanel.tsx`, `uniforms/CatalogPanel.tsx`, `uniforms/InventoryPanel.tsx`, `uniforms/CounterSalePanel.tsx`, `announcements/AnnouncementsScreen.tsx`, `grades/GradeRow.tsx`, `grades/DirectorCapture.tsx`, `attendance/DirectorAttendance.tsx`

- [ ] **Step 1: Escribir los tests que fallan**

En `AdminScreen.test.tsx`:

```tsx
it("el correo del nuevo usuario baja a minúsculas mientras se teclea", async () => {
  render(<AdminScreen onBack={vi.fn()} />);
  await userEvent.click(await screen.findByRole("button", { name: /nuevo usuario/i }));
  const correo = screen.getByLabelText(/correo/i);
  await userEvent.type(correo, "Ana.Ruiz@CEGLORCA.COM");
  expect(correo).toHaveValue("ana.ruiz@ceglorca.com");
});
```

En `AnnouncementsScreen.test.tsx`:

```tsx
it("el cuerpo del aviso NO se grita", async () => {
  render(<AnnouncementsScreen onBack={vi.fn()} />);
  const cuerpo = screen.getByLabelText(/mensaje/i);
  await userEvent.type(cuerpo, "Mañana no hay clases");
  expect(cuerpo).toHaveValue("Mañana no hay clases");
});
```

En `DirectorAttendance.test.tsx`:

```tsx
it("la fecha de la lista sigue siendo un calendario tras migrar", () => {
  render(<DirectorAttendance permissions={["attendance:write"]} />);
  expect(screen.getByLabelText(/fecha/i)).toHaveAttribute("type", "date");
});
```

- [ ] **Step 2: Correr y verlos fallar**

Run: `pnpm vitest run app/_screens/admin app/_screens/announcements app/_screens/attendance`
Expected: FAIL.

- [ ] **Step 3: Implementar**

Mismas reglas de la Task 13. Notas específicas:
- `AdminScreen`: el correo del alta y de la invitación → `EmailField`. Los nombres →
  `TextField uppercase`. Las contraseñas **no** se tocan.
- `AnnouncementsScreen`: el título → `TextField uppercase`; el cuerpo → `TextArea`
  **sin** `uppercase` (D3 del spec).
- `uniforms/CatalogPanel.tsx:270` (`placeholder="650.00"`) → `MoneyField`.
- `grades/GradeRow.tsx` y `DirectorCapture.tsx`: son calificaciones, **no** montos →
  `NumberField` con `min={0} max={10} step={0.1}`. No llevan `$`.
- `uniforms/InventoryPanel.tsx:315/394`: son cantidades → `NumberField`.

- [ ] **Step 4: Correr y verlos pasar**

Run: `pnpm test && pnpm typecheck && pnpm lint`
Expected: PASS. `grep -rn "<input" apps/web/app/_screens --include='*.tsx' | grep -v test`
sólo debe devolver `type="checkbox"` y los buscadores.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web/app
git commit -m "feat(web): el resto de los módulos captura por la capa común"
```

---

## BLOQUE C — Responsive y el cajón

### Task 15: `SlideOver` por portal, con tamaño

**Files:**
- Modify: `apps/web/app/_design/SlideOver.tsx`
- Test: `apps/web/app/_design/SlideOver.test.tsx`

- [ ] **Step 1: Escribir el test que falla**

```tsx
it("se monta en el body, fuera de cualquier ancestro que pueda atraparlo", () => {
  // El defecto que esto previene: en Catalog.tsx el cajón nacía dentro de un
  // <div className="fade-up">, cuya animación de transform crea un bloque
  // contenedor. `inset: 0` dejaba de significar "el viewport" y pasaba a
  // significar "esa columna de 1320px": el velo no cubría la barra superior y
  // se podía cerrar sesión con el cajón "modal" abierto.
  const { container } = render(
    <div style={{ transform: "translateY(0)" }}>
      <SlideOver title="X" subtitle="y" icon="settings" onClose={vi.fn()} onSubmit={vi.fn()} submitLabel="Guardar">
        <p>contenido</p>
      </SlideOver>
    </div>,
  );
  expect(container.querySelector("[role=dialog]")).toBeNull();
  expect(document.body.querySelector("[role=dialog]")).not.toBeNull();
});

it("se anuncia como modal", () => {
  render(<SlideOver title="X" subtitle="y" icon="settings" onClose={vi.fn()} onSubmit={vi.fn()} submitLabel="Guardar"><p>c</p></SlideOver>);
  expect(screen.getByRole("dialog")).toHaveAttribute("aria-modal", "true");
});

it("Esc lo cierra", async () => {
  const onClose = vi.fn();
  render(<SlideOver title="X" subtitle="y" icon="settings" onClose={onClose} onSubmit={vi.fn()} submitLabel="Guardar"><p>c</p></SlideOver>);
  await userEvent.keyboard("{Escape}");
  expect(onClose).toHaveBeenCalled();
});

it("una forma grande pide un cajón grande", () => {
  render(<SlideOver size="lg" title="X" subtitle="y" icon="settings" onClose={vi.fn()} onSubmit={vi.fn()} submitLabel="Guardar"><p>c</p></SlideOver>);
  expect(screen.getByRole("dialog").className).toContain("drawer-lg");
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/SlideOver.test.tsx`
Expected: FAIL — hoy se renderiza en su sitio del árbol y no tiene `aria-modal` ni `size`.

- [ ] **Step 3: Implementar**

- Envolver el retorno en `createPortal(…, document.body)`, guardado tras un `useEffect`
  de montaje para que SSR no lo intente.
- `size?: "sm" | "md" | "lg"` (predeterminado `md`) → `className={"drawer drawer-" + size}`.
  El ancho deja el JSX y vive en `globals.css` (Task 17).
- `role="dialog"` + `aria-modal="true"` + `aria-label={title}`.
- `useEffect` que escucha `keydown` y cierra con `Escape`.
- Mientras esté abierto, `document.body.style.overflow = "hidden"`, restaurado al cerrar.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/_design/SlideOver.test.tsx`
Expected: PASS, 4 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/app/_design/SlideOver.tsx apps/web/app/_design/SlideOver.test.tsx
git commit -m "fix(web): el cajón se monta en el body y vuelve a ser modal

En Configuración nacía dentro de un ancestro con transform animado, que
crea bloque contenedor: el velo no cubría la barra superior y se podía
cerrar sesión con el cajón abierto."
```

---

### Task 16: Mueren los ocho clones del cajón

**Files:**
- Modify: `admin/AdminScreen.tsx:60-90`, `students/StudentsScreen.tsx:256/336`, `finance/CollectDrawer.tsx:187`, `finance/FinanceScreen.tsx:103/407`, `finance/StudentPicker.tsx:8`, `uniforms/InventoryPanel.tsx:442`

- [ ] **Step 1: Escribir el test que falla**

```tsx
// apps/web/app/_design/SlideOver.contract.test.tsx
it("nadie dibuja su propio cajón", async () => {
  // Nueve cajones con cinco anchos distintos (520/520/520/460/520/480/520/460/560)
  // era la razón por la que el ancho no se podía arreglar en un solo lugar.
  const { globSync } = await import("node:fs");
  const offenders = globSync("app/_screens/**/*.tsx", { cwd: process.cwd() })
    .filter((f) => !f.includes(".test."))
    .filter((f) => readFileSync(f, "utf8").includes('justifyContent: "flex-end"')
                && readFileSync(f, "utf8").includes('position: "fixed"'));
  expect(offenders).toEqual([]);
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/SlideOver.contract.test.tsx`
Expected: FAIL — lista los 8 archivos.

- [ ] **Step 3: Implementar**

Cada clon se sustituye por `<SlideOver>` con el `size` que le toca:
- `sm`: `students/StudentsScreen.tsx:336` (Ficha del alumno), `uniforms/InventoryPanel.tsx:442`
- `md`: `admin/AdminScreen.tsx:60`, `students/StudentsScreen.tsx:256` (Nuevo alumno),
  `finance/FinanceScreen.tsx:103` (Registrar pago), `finance/FinanceScreen.tsx:407` (Corte de caja)
- `lg`: `finance/CollectDrawer.tsx:187` (Cobrar), y el tabulador de `TuitionPanel`
- `finance/StudentPicker.tsx` es un modal centrado, no un cajón: se deja como está pero
  también pasa por `createPortal` y recibe `aria-modal`.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm test && pnpm typecheck`
Expected: PASS; el contrato devuelve lista vacía.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web/app
git commit -m "refactor(web): un solo cajón, no nueve con cinco anchos"
```

---

### Task 17: Los anchos salen del JSX

**Files:**
- Create: `apps/web/app/_design/Page.tsx`
- Modify: `apps/web/app/globals.css`, y las 12 pantallas con `maxWidth` de página

- [ ] **Step 1: Escribir el test que falla**

```tsx
// apps/web/app/_design/Page.test.tsx
it("ninguna pantalla se queda con el ancho pegado en el JSX", async () => {
  // Un estilo en línea le gana a cualquier @media. Este repo ya lo aprendió con
  // .topbar-search: "an inline style beats an author rule, so the box kept its
  // desktop width on a 412px screen". Mientras el ancho viva en el JSX, no hay
  // responsive posible.
  const files = globSync("app/_screens/**/*.tsx", { cwd: process.cwd() })
    .filter((f) => !f.includes(".test."));
  const offenders = files.filter((f) => /maxWidth:\s*1[0-9]{3}/.test(readFileSync(f, "utf8")));
  expect(offenders).toEqual([]);
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm vitest run app/_design/Page.test.tsx`
Expected: FAIL — lista las 12 pantallas.

- [ ] **Step 3: Implementar**

En `globals.css`:

```css
/* ============================================================
   ANCHOS DE PÁGINA Y DE CAJÓN

   Viven aquí, y no en el JSX, por una razón que este archivo ya documenta más
   arriba a propósito de .topbar-search: un estilo en línea le gana a cualquier
   @media, así que mientras el ancho estuviera en el JSX no había responsive
   posible. Había además CINCO anchos de página distintos (820/900/1160/1180/
   1320) sin ninguna regla: en un monitor de 2560px, Avisos usaba el 32%.

   El teléfono queda fuera a propósito: este sistema se usa en escritorio y en
   tablet acostada. El almacén de uniformes es la excepción y tiene su propia
   suite (e2e/narrow.mobile.spec.ts).
   ============================================================ */
:root { --page-max: 1320px; --page-pad: 28px; --reading-max: 960px; }

/* Monitor grande: el contenido se ensancha y las rejillas meten más columnas.
   La tipografía NO se escala — se pidió ver más, no ver más grande. */
@media (min-width: 1600px) { :root { --page-max: 1800px; } }

/* Tablet acostada, el piso que debe verse perfecto. */
@media (max-width: 1180px) { :root { --page-pad: 18px; } }

.page {
  width: 100%;
  max-width: var(--page-max);
  margin: 0 auto;
  padding: 24px var(--page-pad) 96px;
}
/* Avisos y ventanilla: la página usa todo el monitor, pero la columna que se
   LEE se acota. Un párrafo de 1800px hace que el ojo pierda el renglón al
   regresar. */
.page-reading > .page-column { max-width: var(--reading-max); }

.drawer-sm { width: min(460px, 100%); }
.drawer-md { width: min(560px, 100%); }
/* El tabulador de colegiaturas —inscripción multi-concepto, tramos, colegiatura,
   cobranza y cuotas— se capturaba por un túnel de 520px tuviera el monitor 1280
   o 2560. */
.drawer-lg { width: min(920px, 100%); }
@media (min-width: 1600px) {
  .drawer-lg { width: min(1100px, 100%); }
  /* La forma deja de ser una tira vertical de cuarenta campos. */
  .drawer-lg .drawer-body { column-count: 2; column-gap: 26px; }
}
```

`Page.tsx`:

```tsx
export function Page({ variant = "wide", children }: {
  variant?: "wide" | "reading";
  children: ReactNode;
}): ReactElement {
  return (
    <div className={variant === "reading" ? "page page-reading" : "page"}>
      {variant === "reading" ? <div className="page-column">{children}</div> : children}
    </div>
  );
}
```

Las 12 pantallas sustituyen su `<div style={{ maxWidth: …, margin: "0 auto", padding: … }}>`
por `<Page>`. Avisos y Ventanilla usan `<Page variant="reading">`.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm vitest run app/_design/Page.test.tsx && pnpm test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web/app
git commit -m "feat(web): un solo ancho de página, en CSS y no en el JSX

Eran cinco (820/900/1160/1180/1320) sin regla; en 2560px Avisos usaba
el 32% del monitor."
```

---

### Task 18: Las tablas dejan de aplastarse en tablet

**Files:**
- Modify: `apps/web/app/globals.css`, `finance/FinanceScreen.tsx:361/462`, `finance/ChargesPanel.tsx:105`, `finance/ChargeRow.tsx:47`, `students/StudentsScreen.tsx:596/665`, `admin/AdminScreen.tsx:225`

- [ ] **Step 1: Escribir la prueba (e2e — jsdom no aplica CSS)**

`apps/web/e2e/tablet.spec.ts`:

```ts
import { devices, expect, test } from "@playwright/test";
import { openModule, signIn } from "./helpers";

test.use({ ...devices["Desktop Chrome"], viewport: { width: 1024, height: 768 } });

const MODULOS = ["Finanzas", "Alumnos", "Configuración", "Administración", "Docentes"];

test.describe("la escuela en una tablet acostada", () => {
  for (const nombre of MODULOS) {
    test(`${nombre} cabe sin scroll lateral`, async ({ page }) => {
      await signIn(page);
      await openModule(page, nombre);
      const overflow = await page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      );
      expect(overflow, `${nombre} se sale de la tablet`).toBeLessThanOrEqual(1);
    });
  }
});
```

- [ ] **Step 2: Correr y verlo fallar**

Run: `pnpm exec playwright test e2e/tablet.spec.ts`
Expected: FAIL en Finanzas y Alumnos — sus tablas de 5-6 columnas fijas no caben.

- [ ] **Step 3: Implementar**

Las plantillas de rejilla salen del JSX a `globals.css`:

```css
.data-head, .data-row { display: grid; align-items: center; gap: 16; }
.data-charges { grid-template-columns: 2.2fr 1fr 1.3fr .9fr 1.1fr auto; }
.data-students { grid-template-columns: 2.4fr 1.1fr 1.6fr 1fr auto; }

/* Tablet: sueltan las columnas secundarias en vez de aplastarse. Lo que se
   pierde es lo que se puede consultar abriendo el renglón; lo que se queda es
   lo que se necesita para decidir cuál abrir. */
@media (max-width: 1100px) {
  .data-charges { grid-template-columns: 2.2fr 1.1fr auto; }
  .data-charges .col-nivel, .data-charges .col-vence,
  .data-students .col-tutor, .data-students .col-nivel { display: none; }
  .data-students { grid-template-columns: 2.4fr 1fr auto; }
}
```

Cada celda que puede desaparecer recibe su clase (`col-nivel`, `col-vence`, `col-tutor`),
tanto en el encabezado como en el renglón, para que no se descuadren.

- [ ] **Step 4: Correr y verlo pasar**

Run: `pnpm exec playwright test e2e/tablet.spec.ts`
Expected: PASS, 5 tests.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web
git commit -m "feat(web): las tablas sueltan columnas en tablet en vez de aplastarse"
```

---

### Task 19: e2e de monitor grande y del cajón

**Files:**
- Create: `apps/web/e2e/wide.spec.ts`, `apps/web/e2e/drawer.spec.ts`
- Modify: `apps/web/playwright.config.ts`, `apps/web/package.json`

- [ ] **Step 1: Escribir las pruebas**

`e2e/wide.spec.ts`:

```ts
test.use({ ...devices["Desktop Chrome"], viewport: { width: 2560, height: 1440 } });

test("el monitor grande SE APROVECHA, no sólo cabe", async ({ page }) => {
  await signIn(page);
  await openModule(page, "Finanzas");
  const ancho = await page.locator(".page").evaluate((el) => el.getBoundingClientRect().width);
  // Antes: 1320px fijos, el 52% de un monitor de 2560.
  expect(ancho).toBeGreaterThan(1700);
});

test("sin scroll lateral en 2560", async ({ page }) => {
  await signIn(page);
  await openModule(page, "Configuración");
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(1);
});
```

`e2e/drawer.spec.ts`:

```ts
test.use({ ...devices["Desktop Chrome"], viewport: { width: 2560, height: 1440 } });

test("el velo cubre la pantalla ENTERA, barra superior incluida", async ({ page }) => {
  await signIn(page);
  await openModule(page, "Configuración");
  await page.getByRole("button", { name: /nuevo ciclo/i }).click();

  const velo = await page.getByRole("dialog").locator("..").boundingBox();
  const vp = page.viewportSize();
  expect(velo?.width).toBe(vp?.width);
  expect(velo?.height).toBe(vp?.height);
  expect(velo?.y).toBe(0);
});

test("con el cajón abierto no se puede cerrar sesión por encima de él", async ({ page }) => {
  await signIn(page);
  await openModule(page, "Configuración");
  await page.getByRole("button", { name: /nuevo ciclo/i }).click();
  // El velo se lo come; si el clic llega, el cajón no era modal.
  await expect(page.getByRole("button", { name: /cerrar sesión/i })).not.toBeVisible();
});

test("el cajón toca el borde derecho del monitor", async ({ page }) => {
  await signIn(page);
  await openModule(page, "Configuración");
  await page.getByRole("button", { name: /nuevo ciclo/i }).click();
  const caja = await page.getByRole("dialog").boundingBox();
  const vp = page.viewportSize();
  expect((caja?.x ?? 0) + (caja?.width ?? 0)).toBe(vp?.width);
});
```

- [ ] **Step 2: Correr y verlas fallar**

Run: `pnpm exec playwright test e2e/drawer.spec.ts`
Expected: FAIL si la Task 15 no estuviera hecha. Con ella hecha: PASS. Confirmar que
fallan revirtiendo el portal un momento (`git stash` sobre `SlideOver.tsx`).

- [ ] **Step 3: Registrar los proyectos**

En `playwright.config.ts`, añadir a `projects`:

```ts
{ name: "tablet", use: { ...devices["Desktop Chrome"], viewport: { width: 1024, height: 768 } }, testMatch: /tablet\.spec\.ts/ },
{ name: "wide", use: { ...devices["Desktop Chrome"], viewport: { width: 2560, height: 1440 } }, testMatch: /(wide|drawer)\.spec\.ts/ },
```

Y a `chromium.testIgnore`: `/tablet\.spec\.ts/`, `/wide\.spec\.ts/`, `/drawer\.spec\.ts/`.
En `package.json`: `"e2e": "playwright test --project=chromium --project=mobile --project=tablet --project=wide"`.

- [ ] **Step 4: Correr todo**

Run: `pnpm e2e`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A apps/web
git commit -m "test(web): tablet 1024, monitor 2560 y el velo que sí cubre la pantalla"
```

---

### Task 20: Verificación final

- [ ] **Step 1: Las cuatro puertas**

```bash
cd apps/web
pnpm typecheck && pnpm lint && pnpm test && pnpm build && pnpm check:no-sourcemaps
```
Expected: las cinco en verde. `check:no-sourcemaps` es la puerta no negociable del repo.

- [ ] **Step 2: Barrido de lo que debía morir**

```bash
grep -rn "finance/money"        apps/web/app                       # vacío
grep -rn "maxWidth: 13\|maxWidth: 11\|maxWidth: 9\|maxWidth: 8" apps/web/app/_screens  # vacío
grep -rn 'justifyContent: "flex-end"' apps/web/app/_screens         # vacío
ls apps/web/app/page.tsx "apps/web/app/(auth)"                      # no existen
```

- [ ] **Step 3: e2e completa**

Run: `pnpm e2e && pnpm e2e:demo`
Expected: PASS.

- [ ] **Step 4: Commit final y resumen para el dueño**

```bash
git add -A && git commit -m "chore(web): verificación final del PR de usabilidad"
```

---

## Autorrevisión del plan

**Cobertura del spec:** Petición 1 (responsive) → Tasks 17, 18, 19. Petición 2
(navegación) → Tasks 1-5. Petición 3 (calendario/mayúsculas/pesos) → Tasks 6, 9, 10, 11,
12, 13, 14. Petición 4 (`*` y rojo) → Tasks 7, 8, 12, 13, 14. Petición 5 (correo en
minúsculas) → Tasks 9, 14. Petición 6 (config anidada) → Tasks 15, 16, 17.
Hallazgos H1→T1-3, H2→T2, H3→T15, H4→T16, H5→T17, H6→T13, H7→T17, H8→T6-14.
H9 (toast) queda fuera por decisión, registrado en el spec §5.

**Nombres consistentes:** `pathToView`/`viewToPath` (T1) se usan igual en T3 y T4.
`useRequired` devuelve `{ errors, firstMissing, missingCount, check, clear }` en T7 y se
consume con esas mismas llaves en T12, T13 y T14. `inputStyle(invalid)` se define en T8 y
se usa en T9, T10 y T11. `Field` recibe hijos como función `(id, describedBy)` en T8 y así
lo consumen T9-T11.
