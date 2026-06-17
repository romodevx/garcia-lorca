import "server-only";

import { cookies } from "next/headers";
import { DEFAULT_LOCALE, isLocale, type Locale } from "@garcia-lorca/i18n";
import { LOCALE_COOKIE } from "./config";

/** Resolve the active locale from the cookie in a Server Component. */
export async function getServerLocale(): Promise<Locale> {
  const store = await cookies();
  const value = store.get(LOCALE_COOKIE)?.value;
  return value !== undefined && isLocale(value) ? value : DEFAULT_LOCALE;
}
