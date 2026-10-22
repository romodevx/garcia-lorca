import { useGlobalSearchParams, usePathname, useSegments } from "expo-router";

type SearchParams = Record<string, string | string[] | undefined>;

// "[id]" or "[...rest]": a segment filled from the path, not from the query.
const DYNAMIC_SEGMENT = /^\[(?:\.\.\.)?([^\]]+)\]$/;

/** The names of the params a route takes from its own path segments. */
function pathParamNames(segments: readonly string[]): Set<string> {
  const names = segments.flatMap((segment) => DYNAMIC_SEGMENT.exec(segment)?.[1] ?? []);
  return new Set(names);
}

/** A param's values as a list: none, one, or the repeated ones. */
function valuesOf(value: string | string[] | undefined): string[] {
  if (value === undefined) {
    return [];
  }
  return Array.isArray(value) ? value : [value];
}

/** "key=value" pairs for the query, one per value, escaped. */
function queryPairs(params: SearchParams, skip: Set<string>): string[] {
  return Object.entries(params)
    .filter(([key]) => !skip.has(key))
    .flatMap(([key, value]) => valuesOf(value).map((item) => `${encodeURIComponent(key)}=${encodeURIComponent(item)}`));
}

/**
 * The href of the route on screen: its path plus the query, without the params
 * that are already in the path. usePathname alone drops the query, so a boleta
 * would lose its studentId.
 */
export function hrefOf(pathname: string, segments: readonly string[], params: SearchParams): string {
  const query = queryPairs(params, pathParamNames(segments));
  return query.length === 0 ? pathname : `${pathname}?${query.join("&")}`;
}

/** hrefOf for the route currently on screen. */
export function useCurrentHref(): { pathname: string; href: string } {
  const pathname = usePathname();
  const segments = useSegments();
  const params: SearchParams = useGlobalSearchParams();
  return { pathname, href: hrefOf(pathname, segments, params) };
}
