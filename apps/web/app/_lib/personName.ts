/** Somebody the school names "first last". */
export interface Named {
  firstName: string;
  lastName: string;
}

const LOCALE = "es-MX";

/** "Ana Ruiz". */
export function fullName(person: Named): string {
  return `${person.firstName} ${person.lastName}`.trim();
}

/**
 * A copy, in the order a reader expects of the label shown: by full name,
 * accents folded the Spanish way (ISSUE-CC-122). Sorting by last name while
 * showing "first last" read as no order at all.
 */
export function byFullName<T extends Named>(people: readonly T[]): T[] {
  return [...people].sort((a, b) => fullName(a).localeCompare(fullName(b), LOCALE, { sensitivity: "base" }));
}
