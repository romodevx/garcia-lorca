import { render, screen } from "@testing-library/react";
import { NextIntlClientProvider, useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import { buildMessages } from "./config";
import { LanguageToggle } from "./LanguageToggle";
import type { Locale } from "@garcia-lorca/i18n";

/**
 * Proves the binding contract: the FLAT, type-safe catalog from
 * `@garcia-lorca/i18n` → `toNested` → next-intl renders the right text and
 * resolves ICU plurals. If flat dotted keys reached next-intl unconverted,
 * `t("action.save")` would look for a nested path and fail — these tests are
 * the guard for that.
 */
function Wrap({ locale, children }: { locale: Locale; children: ReactNode }): ReactNode {
  return (
    <NextIntlClientProvider locale={locale} messages={buildMessages(locale)}>
      {children}
    </NextIntlClientProvider>
  );
}

function SaveLabel(): ReactNode {
  const t = useTranslations("common");
  return <span>{t("action.save")}</span>;
}

function ResultsCount({ count }: { count: number }): ReactNode {
  const t = useTranslations("common");
  return <span>{t("label.results.count", { count })}</span>;
}

function LoginTitle(): ReactNode {
  const t = useTranslations("auth");
  return <span>{t("login.title")}</span>;
}

describe("web next-intl binding", () => {
  it("renders Spanish by default", () => {
    render(<Wrap locale="es"><SaveLabel /></Wrap>);
    expect(screen.getByText("Guardar")).toBeInTheDocument();
  });

  it("renders English when locale is en", () => {
    render(<Wrap locale="en"><SaveLabel /></Wrap>);
    expect(screen.getByText("Save")).toBeInTheDocument();
  });

  it("resolves an ICU plural through the binding (es)", () => {
    render(<Wrap locale="es"><ResultsCount count={3} /></Wrap>);
    expect(screen.getByText("3 resultados")).toBeInTheDocument();
  });

  it("resolves an ICU plural through the binding (en, singular branch)", () => {
    render(<Wrap locale="en"><ResultsCount count={1} /></Wrap>);
    expect(screen.getByText("1 result")).toBeInTheDocument();
  });

  it("serves a second namespace (auth) from the same provider", () => {
    render(<Wrap locale="es"><LoginTitle /></Wrap>);
    expect(screen.getByText("Bienvenido de vuelta")).toBeInTheDocument();
  });

  it("language toggle shows the target locale code for the active locale", () => {
    const { unmount } = render(<Wrap locale="es"><LanguageToggle /></Wrap>);
    expect(screen.getByTestId("language-toggle")).toHaveTextContent("EN");
    unmount();
    render(<Wrap locale="en"><LanguageToggle /></Wrap>);
    expect(screen.getByTestId("language-toggle")).toHaveTextContent("ES");
  });
});
