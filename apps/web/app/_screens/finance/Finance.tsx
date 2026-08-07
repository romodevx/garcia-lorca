"use client";

import { useMemo, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { AccountPanel } from "./AccountPanel";
import { ChargesPanel } from "./ChargesPanel";
import { ConceptsPanel } from "./ConceptsPanel";
import { FinanceHeader } from "./FinanceHeader";
import { GeneratePanel } from "./GeneratePanel";
import { PaymentPanel } from "./PaymentPanel";
import { formatCents } from "./money";
import { SummaryCards, TabBar, Toast, type SummaryCard } from "./parts";
import { collectable, totalsOf } from "./totals";
import { useFinance, type FinanceSection } from "./useFinance";

/** Deep-link intent from the command palette (`create.payment`). */
export type FinanceIntent = "payment";

export interface FinanceProps {
  onBack: () => void;
  intent?: FinanceIntent;
}

/**
 * Finance surface (module_finance): debts, ventanilla payments, charge concepts,
 * bulk generation and account statements. All reads/writes live in `useFinance`;
 * this component only composes the design system around them, mirroring the
 * config Catalog / admin Admin orchestrators.
 *
 * The gateway (behind module_finance + finance:read/finance:write) is the
 * security boundary — this screen is UX only.
 */
export function Finance({ onBack, intent }: FinanceProps): ReactElement {
  const t = useTranslations("finance");
  const fin = useFinance(intent === "payment" ? "payment" : "charges");
  const totals = useMemo(() => totalsOf(fin.charges), [fin.charges]);

  const cards: readonly SummaryCard[] = [
    {
      icon: "clock",
      color: "orange",
      label: t("summary.pending"),
      value: formatCents(totals.remainingCents),
      sub: t("summary.pendingSub", { count: totals.withBalance }),
    },
    { icon: "trend", color: "forest", label: t("summary.paid"), value: formatCents(totals.paidCents), sub: t("summary.paidSub") },
    { icon: "finance", color: "lime", label: t("summary.charges"), value: String(totals.count), sub: t("summary.chargesSub") },
  ];

  const sections: readonly { id: FinanceSection; label: string }[] = [
    { id: "charges", label: t("section.charges") },
    { id: "payment", label: t("section.payment") },
    { id: "concepts", label: t("section.concepts") },
    { id: "generate", label: t("section.generate") },
    { id: "account", label: t("section.account") },
  ];

  return (
    <div style={{ maxWidth: 1320, margin: "0 auto", padding: "24px 28px 64px" }}>
      <FinanceHeader onBack={onBack} />

      {fin.loadError !== undefined ? (
        <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>
          {fin.loadError}
        </p>
      ) : (
        <div className="fade-up" style={{ display: "grid", gap: 22 }}>
          <SummaryCards cards={cards} />
          <TabBar label={t("section.label")} tabs={sections} active={fin.section} onSelect={fin.setSection} />

          {fin.section === "charges" && (
            <ChargesPanel
              charges={fin.charges}
              students={fin.students}
              cycles={fin.cycles}
              filter={fin.filter}
              studentId={fin.studentId}
              cycleId={fin.cycleId}
              loading={fin.loadingCharges}
              error={fin.chargesError}
              onFilterChange={fin.setFilter}
              onStudentChange={fin.setStudentId}
              onCycleChange={fin.setCycleId}
              studentName={fin.studentName}
              onPay={fin.openPayment}
              onDiscount={fin.handleDiscount}
              onCancel={fin.handleCancelCharge}
            />
          )}
          {fin.section === "payment" && (
            <PaymentPanel
              charges={collectable(fin.charges)}
              selectedChargeId={fin.selectedChargeId}
              onSelectCharge={fin.setSelectedChargeId}
              studentName={fin.studentName}
              onSubmit={fin.handlePayment}
            />
          )}
          {fin.section === "concepts" && (
            <ConceptsPanel
              concepts={fin.concepts}
              loadError={fin.conceptsError}
              includeInactive={fin.includeInactive}
              onIncludeInactiveChange={fin.setIncludeInactive}
              onCreate={fin.handleCreateConcept}
              onUpdate={fin.handleUpdateConcept}
            />
          )}
          {fin.section === "generate" && (
            <GeneratePanel
              concepts={fin.concepts}
              cycles={fin.cycles}
              cycleId={fin.cycleId}
              onCycleChange={fin.setCycleId}
              groups={fin.groups}
              onSubmit={fin.handleGenerate}
            />
          )}
          {fin.section === "account" && <AccountPanel students={fin.students} onLoad={fin.handleLoadAccount} />}
        </div>
      )}

      {fin.toast !== undefined && <Toast>{fin.toast}</Toast>}
    </div>
  );
}
