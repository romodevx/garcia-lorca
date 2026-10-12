import type { Metadata } from "next";
import type { ReactNode } from "react";
import { DM_Sans, Libre_Baskerville, Poppins } from "next/font/google";
import { NextIntlClientProvider } from "next-intl";
import { AppFlagsProvider } from "./flags-provider";
import { buildMessages } from "./i18n/config";
import { getServerLocale } from "./i18n/server";
import { ThemeProvider } from "./theme/ThemeProvider";
import { prePaintScript } from "./theme/pre-paint";
import "./globals.css";

import { DemoBanner } from "./_design/DemoBanner";

/**
 * Self-hosted fonts via next/font (no render-blocking @import, no external
 * request). Each exposes a CSS variable consumed by the design tokens in
 * globals.css (`--font-display` / `--font-ui`).
 */
const poppins = Poppins({
  subsets: ["latin"],
  weight: ["500", "600", "700", "800"],
  variable: "--font-poppins",
  display: "swap"
});

const dmSans = DM_Sans({
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
  variable: "--font-dm-sans",
  display: "swap"
});

/**
 * The greeting's serif under the look refresh (module_ui_refresh, F2b), read by
 * `--font-serif`. Not preloaded: with the look OFF nothing paints it, and a
 * preload would make every user download it anyway.
 */
const libreBaskerville = Libre_Baskerville({
  subsets: ["latin"],
  weight: ["400"],
  variable: "--font-libre-baskerville",
  display: "swap",
  preload: false
});

export const metadata: Metadata = {
  title: "Centro Educativo García Lorca — Sistema",
  description: "Sistema de Gestión Escolar García Lorca"
};

export default async function RootLayout({
  children
}: {
  children: ReactNode;
}): Promise<ReactNode> {
  // Server-authoritative locale: read from the cookie so the first paint is in
  // the right language (no hydration text mismatch, no `[locale]` route segment).
  const locale = await getServerLocale();
  const messages = buildMessages(locale);

  return (
    // suppressHydrationWarning: the pre-paint script sets `data-theme` (and
    // `data-look`) on <html> before React hydrates, so the client <html>
    // carries attributes the server HTML does not. This is the intended
    // no-flash pattern; the warning would otherwise fire on <html> every load.
    // Scope is THIS element's attributes only — it does not mask mismatches in
    // children.
    <html
      lang={locale}
      className={`${poppins.variable} ${dmSans.variable} ${libreBaskerville.variable}`}
      suppressHydrationWarning
    >
      <head>
        {/*
          No-flash bootstrap. Runs before paint to set `data-theme` (from
          `gl-theme`) and `data-look` (from `gl-look`) on <html>, avoiding both
          a flash and a hydration mismatch. Static, app-authored string — not
          user input — so it is safe as a pre-hydration inline script.
        */}
        <script dangerouslySetInnerHTML={{ __html: prePaintScript }} />
      </head>
      <body>
        <NextIntlClientProvider locale={locale} messages={messages}>
          <ThemeProvider>
            <AppFlagsProvider>
              <DemoBanner />
              {children}
            </AppFlagsProvider>
          </ThemeProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
