"use client";

import type { ReactElement, ReactNode } from "react";

import { Btn } from "./Controls";
import { DrawerShell, type DrawerSize } from "./DrawerShell";
import type { IconName } from "./Icon";

/**
 * El cajón de "crear uno de estos": la cáscara común más el pie estándar de
 * Cancelar / Guardar.
 *
 * Existe porque el patrón ya estaba escrito a mano tres veces —un alumno nuevo,
 * un pago en ventanilla, la ficha de un alumno— y el catálogo académico era la
 * superficie que NO lo tenía: ponía sus formas en línea, en paneles que crecían
 * conforme se seleccionaba, y el resultado era una pantalla que el colegio
 * llamó confusa.
 *
 * Todo lo que tiene que ver con dónde se monta, el velo, Esc y el ancho vive en
 * <DrawerShell>, que es lo que comparten los cajones con pie propio. Aquí sólo
 * queda la decisión de este: un <form> de verdad, para que Enter guarde, y un
 * pie con dos botones.
 */

export type SlideOverSize = DrawerSize;

export interface SlideOverProps {
  title: string;
  subtitle?: string;
  icon: IconName;
  /** What the primary button says. "Crear nivel" beats "Guardar". */
  submitLabel: string;
  /** True while the save is in flight: the button says so and stops accepting clicks. */
  pending?: boolean;
  /** Why the last save was refused, in words the person can act on. */
  error?: string;
  onClose: () => void;
  onSubmit: () => void;
  size?: SlideOverSize;
  children: ReactNode;
}

export function SlideOver({
  title,
  subtitle,
  icon,
  submitLabel,
  pending = false,
  error,
  onClose,
  onSubmit,
  size = "md",
  children,
}: SlideOverProps): ReactElement | null {
  return (
    <DrawerShell
      title={title}
      subtitle={subtitle}
      icon={icon}
      size={size}
      onClose={onClose}
      onSubmit={() => {
        if (!pending) onSubmit();
      }}
      footer={
        <>
          <Btn kind="ghost" type="button" onClick={onClose}>
            Cancelar
          </Btn>
          <div style={{ flex: 1 }} />
          {/* Se apaga SÓLO mientras la petición va en camino. Una forma
              incompleta recibe una frase, nunca un control muerto que no
              dice nada. */}
          <Btn
            icon="check"
            type="submit"
            disabled={pending}
            style={{ opacity: pending ? 0.45 : 1, height: 52, fontSize: 16 }}
          >
            {pending ? "Guardando…" : submitLabel}
          </Btn>
        </>
      }
    >
      {children}
      {error !== undefined && (
        <p role="alert" style={{ color: "var(--bad-fg)", fontSize: 14, margin: 0 }}>
          {error}
        </p>
      )}
    </DrawerShell>
  );
}
