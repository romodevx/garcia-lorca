import type { Metadata } from "next";
import type { ReactNode } from "react";
import { DM_Sans, Poppins } from "next/font/google";
import { NextIntlClientProvider } from "next-intl";
import { AppFlagsProvider } from "./flags-provider";
import { buildMessages } from "./i18n/config";
import { getServerLocale } from "./i18n/server";
import { ThemeProvider } from "./theme/ThemeProvider";
import { themeBootstrapScript } from "./theme/theme-script";
import "./globals.css";

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
    <html lang={locale} className={`${poppins.variable} ${dmSans.variable}`}>
      <head>
        {/*
          No-flash theme bootstrap. Runs before paint to set `data-theme` on
          <html> from localStorage (`gl-theme`), avoiding both a light/dark
          flash and a hydration mismatch. Static, app-authored string — not user
          input — so it is safe as a pre-hydration inline script.
        */}
        <script dangerouslySetInnerHTML={{ __html: themeBootstrapScript }} />
      </head>
      <body>
        <NextIntlClientProvider locale={locale} messages={messages}>
          <ThemeProvider>
            <AppFlagsProvider>{children}</AppFlagsProvider>
          </ThemeProvider>
        </NextIntlClientProvider>
      </body>
    </html>
  );
}
