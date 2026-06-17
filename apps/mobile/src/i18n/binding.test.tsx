import { render, screen } from "@testing-library/react-native";
import { type ReactNode } from "react";
import { I18nextProvider, useTranslation } from "react-i18next";
import { Text } from "react-native";
import { renderWithProviders } from "../test/renderWithProviders";
import i18n from "./i18n";
import { LanguageToggle } from "./LanguageToggle";

/**
 * Proves the mobile binding contract: flat catalog → toNested → i18next-icu
 * renders the right text and resolves ICU plurals. The plural case is the
 * Hermes-relevant one — it exercises Intl.PluralRules, which the polyfill
 * guarantees on device (here it runs under Node's Intl, with the polyfill forced
 * so the path is identical to Hermes).
 */
function Wrap({ children }: { children: ReactNode }): ReactNode {
  return <I18nextProvider i18n={i18n}>{children}</I18nextProvider>;
}

function SaveLabel(): ReactNode {
  const { t } = useTranslation("common");
  return <Text>{t("action.save")}</Text>;
}

function ResultsCount({ count }: { count: number }): ReactNode {
  const { t } = useTranslation("common");
  return <Text>{t("label.results.count", { count })}</Text>;
}

function LoginTitle(): ReactNode {
  const { t } = useTranslation("auth");
  return <Text>{t("login.title")}</Text>;
}

describe("mobile react-i18next + i18next-icu binding", () => {
  beforeEach(async () => {
    await i18n.changeLanguage("es");
  });

  it("renders Spanish by default", async () => {
    await render(<Wrap><SaveLabel /></Wrap>);
    expect(screen.getByText("Guardar")).toBeTruthy();
  });

  it("switches to English via changeLanguage", async () => {
    await i18n.changeLanguage("en");
    await render(<Wrap><SaveLabel /></Wrap>);
    expect(screen.getByText("Save")).toBeTruthy();
  });

  it("resolves an ICU plural (es, plural branch) — exercises Intl.PluralRules", async () => {
    await render(<Wrap><ResultsCount count={3} /></Wrap>);
    expect(screen.getByText("3 resultados")).toBeTruthy();
  });

  it("resolves an ICU plural (es, singular branch)", async () => {
    await render(<Wrap><ResultsCount count={1} /></Wrap>);
    expect(screen.getByText("1 resultado")).toBeTruthy();
  });

  it("serves a second namespace (auth) from the same instance", async () => {
    await render(<Wrap><LoginTitle /></Wrap>);
    expect(screen.getByText("Bienvenido de vuelta")).toBeTruthy();
  });

  it("language toggle shows the target locale code (full provider stack)", async () => {
    await renderWithProviders(<LanguageToggle />);
    expect(screen.getByText("EN")).toBeTruthy();
  });
});
