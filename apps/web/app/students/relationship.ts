import type { Relationship } from "./api";

/** How the office says each relationship, in the order it offers them. */
export const RELATIONSHIPS: readonly { value: Relationship; label: string }[] = [
  { value: "mother", label: "Madre" },
  { value: "father", label: "Padre" },
  { value: "guardian", label: "Tutor" },
  { value: "other", label: "Otro" },
];

export function relationshipLabel(relationship: string): string {
  return RELATIONSHIPS.find((r) => r.value === relationship)?.label ?? relationship;
}
