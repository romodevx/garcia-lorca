"use client";

import type { ReactElement } from "react";

import { Btn } from "../../_design/Controls";
import { DrawerShell } from "../../_design/DrawerShell";
import { EmptyState } from "../uniforms/parts";

/**
 * What "Registrar egreso" opens while the school has no category to file it
 * under (ISSUE-025). The form used to open with an empty "Categoría" that
 * nothing could fill; this says why and goes straight to creating one.
 */
export function FirstCategoryInvite({ onCreateCategory, onClose }: { onCreateCategory: () => void; onClose: () => void }): ReactElement {
  return (
    <DrawerShell title="Registrar egreso" icon="trend" onClose={onClose}>
      <EmptyState
        icon="trend"
        title="Primero, una categoría"
        hint="Cada egreso se archiva en una categoría —papelería, mantenimiento, nómina— para el balance y los reportes. Aún no hay ninguna activa."
        action={<Btn icon="plus" onClick={onCreateCategory}>Crear la primera categoría</Btn>}
      />
    </DrawerShell>
  );
}
