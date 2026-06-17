"use client";

import { useLocale, useTranslations } from "next-intl";
import { useCallback, type ReactNode } from "react";
import { LOCALE_COOKIE } from "./config";

/**
 * Minimal es ⇆ en toggle. Writes the locale cookie and reloads so the Server
 * Component layout re-renders with the new catalog (server-authoritative — no
 * hydration mismatch). A full reload (vs router.refresh) keeps this free of the
 * App Router context, so it renders in any test harness. UX-only.
 */
export function LanguageToggle(): ReactNode {
  const locale = useLocale();
  const t = useTranslations("common");
  const next = locale === "es" ? "en" : "es";

  const toggle = useCallback(() => {
    document.cookie = `${LOCALE_COOKIE}=${next};path=/;max-age=31536000;samesite=lax`;
    window.location.reload();
  }, [next]);

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label={t("language.label")}
      title={t("language.label")}
      data-testid="language-toggle"
      style={{
        height: 40,
        minWidth: 44,
        padding: "0 14px",
        borderRadius: "var(--r-pill)",
        background: "var(--surface-3)",
        border: "1px solid var(--border)",
        color: "var(--text)",
        fontWeight: 700,
        fontSize: 13,
        fontFamily: "var(--font-display)",
        cursor: "pointer"
      }}
    >
      {next.toUpperCase()}
    </button>
  );
}
