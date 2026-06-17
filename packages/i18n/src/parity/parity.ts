import {
  isArgumentElement,
  isDateElement,
  isNumberElement,
  isPluralElement,
  isSelectElement,
  isTagElement,
  isTimeElement,
  type MessageFormatElement,
  parse,
} from "@formatjs/icu-messageformat-parser";

/**
 * Parity validator — the single guard that keeps the bilingual ICU catalog
 * honest across `es` and `en`. It is intentionally stricter than "do both
 * languages have the same keys", because key-parity alone lets divergent
 * interpolation through and that crashes at render time in one language only.
 *
 *   checkParity("common", { es, en })
 *      │
 *      ├─ per locale, per message ──> nested-key | empty-value | parse-error | missing-other
 *      └─ reference(es) vs en ──────> missing-key | arg-mismatch
 *
 * It validates the *contract*, not the prose: same keys, same ICU argument
 * signature (name + kind), valid ICU, mandatory `other` branch, non-empty,
 * flat keys.
 */

export type ParityIssueKind =
  | "missing-key"
  | "arg-mismatch"
  | "parse-error"
  | "missing-other"
  | "empty-value"
  | "nested-key"
  | "key-collision";

export interface ParityIssue {
  readonly kind: ParityIssueKind;
  readonly namespace: string;
  readonly key: string;
  /** Locale where the issue was observed (or the reference, for `missing-key`). */
  readonly locale?: string;
  readonly detail: string;
}

/** A catalog as loaded from a JSON file: flat by contract, `unknown` until checked. */
export type RawCatalog = Record<string, unknown>;

const DEFAULT_REFERENCE_LOCALE = "es";

/** argName -> kind label (argument | number | date | time | select | plural). */
type ArgSignature = Map<string, string>;

interface ParsedMessage {
  readonly ok: true;
  readonly args: ArgSignature;
  /** Arg names of plural/select branches that lack the mandatory `other`. */
  readonly missingOther: readonly string[];
}

interface UnparsableMessage {
  readonly ok: false;
  readonly error: string;
}

/**
 * Compare two or more locale catalogs for one namespace. Every locale is
 * validated on its own (structure/ICU), then each non-reference locale is
 * compared key-for-key and arg-for-arg against the reference.
 */
export function checkParity(
  namespace: string,
  catalogs: Record<string, RawCatalog>,
  referenceLocale: string = DEFAULT_REFERENCE_LOCALE,
): ParityIssue[] {
  const issues: ParityIssue[] = [];
  const locales = Object.keys(catalogs);
  const signatures: Record<string, ArgSignature> = {};
  const validKeysByLocale: Record<string, Set<string>> = {};

  for (const locale of locales) {
    const catalog = catalogs[locale] ?? {};
    const keys = Object.keys(catalog);
    validKeysByLocale[locale] = new Set(keys);
    collectCollisions(namespace, locale, keys, issues);
    for (const key of keys) {
      validateMessage(namespace, locale, key, catalog[key], issues, signatures);
    }
  }

  const referenceKeys = validKeysByLocale[referenceLocale] ?? new Set<string>();
  for (const locale of locales) {
    if (locale === referenceLocale) continue;
    const localeKeys = validKeysByLocale[locale] ?? new Set<string>();
    collectMissingKeys(namespace, referenceLocale, locale, referenceKeys, localeKeys, issues);
    collectArgMismatches(namespace, locale, referenceLocale, referenceKeys, localeKeys, signatures, issues);
  }

  return issues;
}

/**
 * Detect leaf/branch collisions within one locale: a key that is also a strict
 * dot-prefix of another key (`a` alongside `a.b`). Legal in flat JSON but fatal
 * to the nested transform every framework binding runs, so it is a parity error.
 */
function collectCollisions(namespace: string, locale: string, keys: string[], issues: ParityIssue[]): void {
  const keySet = new Set(keys);
  const reported = new Set<string>();
  for (const key of keys) {
    const parts = key.split(".");
    for (let depth = 1; depth < parts.length; depth += 1) {
      const ancestor = parts.slice(0, depth).join(".");
      if (keySet.has(ancestor) && !reported.has(ancestor)) {
        reported.add(ancestor);
        issues.push({
          kind: "key-collision",
          namespace,
          key: ancestor,
          locale,
          detail: `'${ancestor}' is a leaf but also an ancestor of '${key}' (breaks the nested transform)`,
        });
      }
    }
  }
}

function validateMessage(
  namespace: string,
  locale: string,
  key: string,
  value: unknown,
  issues: ParityIssue[],
  signatures: Record<string, ArgSignature>,
): void {
  if (typeof value !== "string") {
    issues.push({
      kind: "nested-key",
      namespace,
      key,
      locale,
      detail: `value is ${typeof value}, expected a string (catalogs must be flat)`,
    });
    return;
  }
  if (value.trim() === "") {
    issues.push({ kind: "empty-value", namespace, key, locale, detail: "value is empty or whitespace-only" });
    return;
  }

  const parsed = parseMessage(value);
  if (!parsed.ok) {
    issues.push({ kind: "parse-error", namespace, key, locale, detail: parsed.error });
    return;
  }
  for (const arg of parsed.missingOther) {
    issues.push({
      kind: "missing-other",
      namespace,
      key,
      locale,
      detail: `plural/select on '${arg}' has no 'other' branch (ICU requires it)`,
    });
  }
  signatures[signatureId(locale, key)] = parsed.args;
}

function collectMissingKeys(
  namespace: string,
  referenceLocale: string,
  locale: string,
  referenceKeys: Set<string>,
  localeKeys: Set<string>,
  issues: ParityIssue[],
): void {
  for (const key of referenceKeys) {
    if (!localeKeys.has(key)) {
      issues.push({
        kind: "missing-key",
        namespace,
        key,
        locale,
        detail: `present in '${referenceLocale}', missing in '${locale}'`,
      });
    }
  }
  for (const key of localeKeys) {
    if (!referenceKeys.has(key)) {
      issues.push({
        kind: "missing-key",
        namespace,
        key,
        locale: referenceLocale,
        detail: `present in '${locale}', missing in '${referenceLocale}'`,
      });
    }
  }
}

function collectArgMismatches(
  namespace: string,
  locale: string,
  referenceLocale: string,
  referenceKeys: Set<string>,
  localeKeys: Set<string>,
  signatures: Record<string, ArgSignature>,
  issues: ParityIssue[],
): void {
  for (const key of referenceKeys) {
    if (!localeKeys.has(key)) continue;
    const reference = signatures[signatureId(referenceLocale, key)];
    const candidate = signatures[signatureId(locale, key)];
    if (!reference || !candidate) continue; // a structural issue was already reported.
    const diff = diffSignatures(reference, candidate);
    if (diff) {
      issues.push({ kind: "arg-mismatch", namespace, key, locale, detail: diff });
    }
  }
}

function diffSignatures(reference: ArgSignature, candidate: ArgSignature): string | null {
  const names = new Set([...reference.keys(), ...candidate.keys()]);
  const problems: string[] = [];
  for (const name of names) {
    const referenceKind = reference.get(name);
    const candidateKind = candidate.get(name);
    if (referenceKind === undefined) problems.push(`'${name}' only in this locale`);
    else if (candidateKind === undefined) problems.push(`'${name}' only in reference`);
    else if (referenceKind !== candidateKind) problems.push(`'${name}' is ${referenceKind} vs ${candidateKind}`);
  }
  return problems.length > 0 ? problems.join("; ") : null;
}

function parseMessage(message: string): ParsedMessage | UnparsableMessage {
  try {
    const ast = parse(message, { requiresOtherClause: false, captureLocation: false });
    const args: ArgSignature = new Map();
    const missingOther: string[] = [];
    walk(ast, args, missingOther);
    return { ok: true, args, missingOther };
  } catch (error) {
    return { ok: false, error: error instanceof Error ? error.message : String(error) };
  }
}

function walk(elements: MessageFormatElement[], args: ArgSignature, missingOther: string[]): void {
  for (const element of elements) {
    if (isArgumentElement(element)) {
      args.set(element.value, "argument");
    } else if (isNumberElement(element)) {
      args.set(element.value, "number");
    } else if (isDateElement(element)) {
      args.set(element.value, "date");
    } else if (isTimeElement(element)) {
      args.set(element.value, "time");
    } else if (isSelectElement(element)) {
      args.set(element.value, "select");
      if (!("other" in element.options)) missingOther.push(element.value);
      for (const option of Object.values(element.options)) walk(option.value, args, missingOther);
    } else if (isPluralElement(element)) {
      args.set(element.value, "plural");
      if (!("other" in element.options)) missingOther.push(element.value);
      for (const option of Object.values(element.options)) walk(option.value, args, missingOther);
    } else if (isTagElement(element)) {
      walk(element.children, args, missingOther);
    }
    // literal & pound elements carry no arguments.
  }
}

function signatureId(locale: string, key: string): string {
  return `${locale} ${key}`;
}
