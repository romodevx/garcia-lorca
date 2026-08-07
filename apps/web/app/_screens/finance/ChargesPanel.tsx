"use client";

import type { ReactElement } from "react";
import { useTranslations } from "next-intl";
import type { Cycle } from "../../config/api";
import type { Charge } from "../../finance/api";
import type { Student } from "../../students/api";
import { ACTION_COL_WIDTH, CHARGE_GRID, ChargeRow } from "./ChargeRow";
import { AllClear, ErrorAlert, SelectField, TabBar } from "./parts";

/** Which charges the table shows: the collection queue, or everything filtered. */
export type ChargeFilter = "overdue" | "all";

export interface ChargesPanelProps {
  charges: readonly Charge[];
  students: readonly Student[];
  cycles: readonly Cycle[];
  filter: ChargeFilter;
  studentId: string;
  cycleId: string;
  loading: boolean;
  error?: string;
  onFilterChange: (filter: ChargeFilter) => void;
  onStudentChange: (studentId: string) => void;
  onCycleChange: (cycleId: string) => void;
  studentName: (studentId: string) => string;
  onPay: (charge: Charge) => void;
  onDiscount: (chargeId: string, discountCents: number, reason: string) => Promise<void>;
  onCancel: (chargeId: string) => Promise<void>;
}

/**
 * The debts table (default section). "Vencidos" is the collection queue (the
 * gateway's overdue read); "Todos" lists a student's/cycle's charges. Dumb by
 * design: every read and every filter change is the orchestrator's job.
 */
export function ChargesPanel({
  charges,
  students,
  cycles,
  filter,
  studentId,
  cycleId,
  loading,
  error,
  onFilterChange,
  onStudentChange,
  onCycleChange,
  studentName,
  onPay,
  onDiscount,
  onCancel,
}: ChargesPanelProps): ReactElement {
  const t = useTranslations("finance");

  const columns = [t("col.concept"), t("col.student"), t("col.amount"), t("col.paid"), t("col.remaining"), t("col.due")];

  return (
    <div style={{ display: "grid", gap: 18 }}>
      <div style={{ display: "flex", alignItems: "flex-end", gap: 14, flexWrap: "wrap" }}>
        <TabBar
          label={t("filter.label")}
          tabs={[
            { id: "overdue", label: t("filter.overdue") },
            { id: "all", label: t("filter.all") },
          ]}
          active={filter}
          onSelect={onFilterChange}
        />
        <div style={{ minWidth: 220 }}>
          <SelectField
            label={t("field.cycle")}
            value={cycleId}
            options={cycles.map((c) => ({ value: c.id, label: c.name }))}
            onChange={onCycleChange}
            placeholder={t("option.choose")}
          />
        </div>
        <div style={{ minWidth: 240 }}>
          <SelectField
            label={t("field.student")}
            value={studentId}
            options={students.map((s) => ({ value: s.id, label: `${s.firstName} ${s.lastName}` }))}
            onChange={onStudentChange}
            placeholder={t("option.choose")}
            disabled={filter === "overdue"}
          />
        </div>
      </div>

      {error !== undefined && <ErrorAlert message={error} />}

      <div
        style={{
          background: "var(--surface)",
          border: "1px solid var(--border)",
          borderRadius: "var(--r-xl)",
          boxShadow: "var(--shadow-card)",
          overflow: "hidden",
        }}
      >
        <div
          style={{
            display: "grid",
            gridTemplateColumns: CHARGE_GRID,
            gap: 16,
            padding: "14px 20px 12px",
            borderBottom: "1px solid var(--border)",
            fontSize: 12.5,
            fontWeight: 700,
            color: "var(--text-faint)",
            textTransform: "uppercase",
            letterSpacing: ".04em",
          }}
        >
          {columns.map((column) => (
            <div key={column}>{column}</div>
          ))}
          <div style={{ width: ACTION_COL_WIDTH }} />
        </div>
        <div style={{ padding: 8 }}>
          {loading ? (
            <p style={{ padding: "24px 12px", color: "var(--text-muted)", fontSize: 15 }}>{t("state.loading")}</p>
          ) : charges.length === 0 ? (
            <AllClear title={t("state.allClear")} hint={t("state.empty.charges")} />
          ) : (
            charges.map((charge) => (
              <ChargeRow
                key={charge.id}
                charge={charge}
                studentName={studentName(charge.studentId)}
                onPay={onPay}
                onDiscount={onDiscount}
                onCancel={onCancel}
              />
            ))
          )}
        </div>
      </div>
    </div>
  );
}
