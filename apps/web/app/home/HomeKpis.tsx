"use client";

import { useFlags } from "@garcia-lorca/ui/flags";
import { useTranslations } from "next-intl";
import { useId, type CSSProperties, type ReactElement } from "react";

import { COLOR } from "../_design/colors";
import { Icon } from "../_design/Icon";
import { homeKpiCards, type KpiCard, type KpiFace, type KpiTarget } from "./kpiFaces";
import type { HomeKpis as Kpis } from "./useHomeKpis";

type Open = (id: string, intent?: string) => void;
type KpiVars = CSSProperties & { "--kpi-c": string };

const go = (onOpen: Open, t: KpiTarget): void => (t.intent === undefined ? onOpen(t.id) : onOpen(t.id, t.intent));

/** The line under the label: never a number it does not know (§3.4). */
function FaceBody({ face }: { face: KpiFace }): ReactElement {
  const t = useTranslations("home");
  switch (face.kind) {
    case "loading":
      return (
        <>
          <span className="home-kpi__skeleton" aria-hidden />
          <span className="home-sr">{t("kpi.loading")}</span>
        </>
      );
    case "error":
      return <span className="home-kpi__note">{t("kpi.error")}</span>;
    case "noCycle":
      return <span className="home-kpi__note">{t("kpi.noCycle")}</span>;
    case "zero":
      return (
        <>
          {/* Two lines on screen; one sentence for a screen reader. */}
          <span className="home-kpi__empty" aria-hidden>{t(face.textKey)}</span>
          <span className="home-kpi__cta" aria-hidden>
            {t(face.ctaKey)} →
          </span>
          <span className="home-sr">{`${t(face.textKey)} · ${t(face.ctaKey)}`}</span>
        </>
      );
    case "value":
      return (
        <>
          <span className="home-kpi__value">{face.value}</span>
          <span className="home-kpi__sub">{t(face.subKey, face.subValues)}</span>
        </>
      );
  }
}

/** A button of its own inside the card, above the stretched main button. */
function FaceAction({ face, onOpen }: { face: KpiFace; onOpen: Open }): ReactElement | null {
  const t = useTranslations("home");
  if (face.kind === "error") {
    return (
      <button type="button" className="home-kpi__action" onClick={face.retry}>
        {t("kpi.retry")}
      </button>
    );
  }
  if (face.kind === "noCycle" && face.toConfig) {
    return (
      <button type="button" className="home-kpi__action" onClick={() => onOpen("settings")}>
        {t("kpi.toConfig")} <span aria-hidden>→</span>
      </button>
    );
  }
  return null;
}

function KpiCardView({ card, onOpen }: { card: KpiCard; onOpen: Open }): ReactElement {
  const t = useTranslations("home");
  const bodyId = useId();
  const vars: KpiVars = { "--kpi-c": COLOR[card.color].v };
  const target = card.face.kind === "zero" ? card.face.target : card.opens;
  return (
    <li className="home-kpi" data-kpi={card.id} style={vars}>
      <span className="home-kpi__icon">
        <Icon name={card.icon} size={22} />
      </span>
      <div className="home-kpi__text">
        <button type="button" className="home-kpi__main" aria-describedby={bodyId} onClick={() => go(onOpen, target)}>
          {t(card.labelKey)}
        </button>
        <div id={bodyId} className="home-kpi__body">
          <FaceBody face={card.face} />
        </div>
        <FaceAction face={card.face} onOpen={onOpen} />
      </div>
    </li>
  );
}

export interface HomeKpisProps {
  kpis: Kpis;
  permissions: readonly string[];
  onOpen: Open;
}

/** The KPI row of the refreshed home; nothing at all for a caller who reads none. */
export function HomeKpis({ kpis, permissions, onOpen }: HomeKpisProps): ReactElement | null {
  const flags = useFlags();
  const cards = homeKpiCards(kpis, permissions, flags);
  if (cards.length === 0) return null;
  return (
    <ul className="home-kpis">
      {cards.map((card) => (
        <KpiCardView key={card.id} card={card} onOpen={onOpen} />
      ))}
    </ul>
  );
}
