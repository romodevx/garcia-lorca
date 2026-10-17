"use client";

import type { ReactElement } from "react";

import { EmptyHint, Section } from "./parts";

/**
 * Colegiaturas for a role without finance:read (Secretaría, CC-311). The
 * tabuladores live in finance and are owner-only; instead of asking anyway and
 * showing an empty card over a 403, the card says who sees them.
 */
export function TuitionNotReadable(): ReactElement {
  return (
    <Section title="Colegiaturas">
      <EmptyHint>Las colegiaturas las ve quien administra Finanzas.</EmptyHint>
    </Section>
  );
}
