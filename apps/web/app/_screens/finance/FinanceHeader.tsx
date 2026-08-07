"use client";

import type { ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Icon } from "../../_design/Icon";

/** Breadcrumb + titled module header for the finance surface. */
export function FinanceHeader({ onBack }: { onBack: () => void }): ReactElement {
  const t = useTranslations("finance");
  return (
    <>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        {t("action.back")}
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div
          style={{
            width: 56,
            height: 56,
            borderRadius: 18,
            display: "grid",
            placeItems: "center",
            background: "var(--orange)",
            color: "#fff",
            boxShadow: "0 6px 16px color-mix(in oklab, var(--orange) 38%, transparent)",
          }}
        >
          <Icon name="finance" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>{t("title")}</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{t("subtitle")}</p>
        </div>
      </div>
    </>
  );
}
