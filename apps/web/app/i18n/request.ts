import { getRequestConfig } from "next-intl/server";
import { buildMessages } from "./config";
import { getServerLocale } from "./server";

/**
 * next-intl request config (App Router, without i18n routing). Resolves the
 * active locale from the cookie and supplies the nested message tree, so both
 * Server Components (`useTranslations`/`getTranslations`) and the client provider
 * read from one source. Wired via `createNextIntlPlugin` in `next.config.js`.
 */
export default getRequestConfig(async () => {
  const locale = await getServerLocale();
  return { locale, messages: buildMessages(locale) };
});
