import { FlagsProvider, type FlagSet } from "@garcia-lorca/ui/flags";
import { NextIntlClientProvider } from "next-intl";
import { useSyncExternalStore, type ReactElement, type ReactNode } from "react";
import { buildMessages } from "./i18n/config";
import { ThemeProvider } from "./theme/ThemeProvider";

/**
 * Wraps a UI tree in the app providers (i18n + theme + flags) for component
 * tests, mirroring the real layout so a component that calls `useTranslations`
 * renders in isolation. Defaults to Spanish; pass `flags` to drive feature-gate
 * behavior under test.
 */
export function Providers({
  children,
  flags = {}
}: {
  children: ReactNode;
  flags?: FlagSet;
}): ReactElement {
  return (
    <NextIntlClientProvider locale="es" messages={buildMessages("es")}>
      <ThemeProvider>
        <FlagsProvider flags={flags}>{children}</FlagsProvider>
      </ThemeProvider>
    </NextIntlClientProvider>
  );
}

/* ============================================================
   ROUTER FALSO PARA PRUEBAS

   La navegación de la web es la barra de direcciones (app/routing/routes.ts),
   así que probarla necesita un router con estado REAL: un `vi.fn()` suelto
   registra el empujón pero nunca vuelve a pintar, y toda prueba que navegue con
   un clic quedaría mirando la pantalla anterior.

   Esto es lo mínimo que hace falta: una dirección, una suscripción y
   useSyncExternalStore, que es como React quiere que se lea un estado externo.
   ============================================================ */

let currentPath = "/";
const pathListeners = new Set<() => void>();

function setPath(next: string): void {
  currentPath = next;
  for (const listener of pathListeners) listener();
}

function subscribePath(listener: () => void): () => void {
  pathListeners.add(listener);
  return () => pathListeners.delete(listener);
}

/** El router falso: las mismas dos operaciones que AppShell usa de verdad. */
export const fakeRouter = {
  push: (path: string): void => setPath(path),
  replace: (path: string): void => setPath(path),
};

/** El `usePathname` falso, suscrito de verdad para que re-pinte. */
export function useFakePathname(): string {
  return useSyncExternalStore(subscribePath, () => currentPath, () => currentPath);
}

/** Dónde está parado el router falso ahora mismo. */
export function fakePath(): string {
  return currentPath;
}

/**
 * Deja el router en una dirección concreta. Llamar en `beforeEach`: la
 * dirección es estado de módulo y se filtraría de una prueba a la siguiente.
 */
export function resetFakeRouter(path = "/"): void {
  currentPath = path;
}
