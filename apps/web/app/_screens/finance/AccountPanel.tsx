"use client";

import { useRef, useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Btn } from "../../_design/Controls";
import type { AccountStatement } from "../../finance/api";
import type { Student } from "../../students/api";
import { formatCents } from "../../_lib/money";
import { SelectField } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section } from "./parts";

export interface AccountPanelProps {
  students: readonly Student[];
  /** Rejects with the gateway's message so this panel can show it. */
  onLoad: (studentId: string) => Promise<AccountStatement>;
}

/**
 * A student's statement: every charge, every payment, and the outstanding
 * balance the gateway computed (never re-derived here — the server is the single
 * source of truth for money).
 */
export function AccountPanel({ students, onLoad }: AccountPanelProps): ReactElement {
  const t = useTranslations("finance");
  const [studentId, setStudentId] = useState("");
  const [statement, setStatement] = useState<AccountStatement | undefined>(undefined);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  // Monotonic id of the newest read: a slow response for a previously chosen
  // student must never be rendered as THIS student's statement.
  const request = useRef(0);

  const load = async (): Promise<void> => {
    setError(undefined);
    if (studentId === "") return;
    const id = (request.current += 1);
    const isCurrent = (): boolean => id === request.current;
    setPending(true);
    try {
      const loaded = await onLoad(studentId);
      if (isCurrent()) setStatement(loaded);
    } catch (cause) {
      if (isCurrent()) {
        setStatement(undefined);
        setError(t("error.account", { detail: cause instanceof Error ? cause.message : String(cause) }));
      }
    } finally {
      if (isCurrent()) setPending(false);
    }
  };

  return (
    <Section title={t("section.account")}>
      <div style={{ display: "grid", gap: 16 }}>
        <div style={{ display: "flex", gap: 14, alignItems: "flex-end", flexWrap: "wrap" }}>
          <div style={{ minWidth: 260 }}>
            <SelectField
              label={t("field.student")}
              value={studentId}
              options={students.map((student) => ({ value: student.id, label: `${student.firstName} ${student.lastName}` }))}
              onChange={setStudentId}
              placeholder={t("option.choose")}
            />
          </div>
          <Btn kind="forest" icon="reports" onClick={() => void load()} disabled={pending || studentId === ""}>
            {t("action.loadAccount")}
          </Btn>
        </div>

        {error !== undefined && <ErrorAlert message={error} />}

        {statement === undefined ? (
          <EmptyHint>{t("state.empty.account")}</EmptyHint>
        ) : (
          <div style={{ display: "grid", gap: 16 }}>
            <div
              style={{
                padding: "16px 20px",
                borderRadius: "var(--r-lg)",
                background: "var(--surface-3)",
                border: "1px solid var(--border)",
              }}
            >
              {/*
                What is being asked for TODAY leads, and the year's balance sits
                beside it. Both are true at once, and a family shown only the
                year is told a number nobody is asking them for.

                An empty asOf means the gateway is older than M18 and sent
                nothing. Rendering $0.00 there would tell a family that owes
                money that they owe none, so the figure is withheld instead.
              */}
              <div style={{ display: "flex", alignItems: "baseline", gap: 12, flexWrap: "wrap" }}>
                <span style={{ fontSize: 14, fontWeight: 600, color: "var(--text-muted)" }}>
                  A pagar hoy
                </span>
                <span
                  data-testid="due-today"
                  style={{ fontFamily: "var(--font-display)", fontWeight: 800, fontSize: 30, color: "var(--text)" }}>
                  {statement.asOf === "" ? "—" : formatCents(statement.dueTodayCents)}
                </span>
                <div style={{ flex: 1 }} />
                <span style={{ fontSize: 13.5, color: "var(--text-muted)" }}>{t("account.balance")}</span>
                <span
                  data-testid="year-balance"
                  style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 19, color: "var(--text-muted)" }}>
                  {formatCents(statement.totalBalanceCents)}
                </span>
              </div>
              {statement.asOf !== "" && (
                <p
                  data-testid="due-today-parts"
                  style={{ marginTop: 6, fontSize: 12.5, color: "var(--text-faint)" }}>
                  {statement.surchargeCents > 0 && `Incluye ${formatCents(statement.surchargeCents)} de recargo. `}
                  {statement.earlyDiscountCents > 0 && `Ya con ${formatCents(statement.earlyDiscountCents)} de pronto pago. `}
                  Al {statement.asOf}.
                </p>
              )}
            </div>

            <div>
              <h3 style={{ fontSize: 15, fontWeight: 700, color: "var(--text)", marginBottom: 8 }}>{t("account.charges")}</h3>
              {statement.charges.length === 0 ? (
                <EmptyHint>{t("state.empty.charges")}</EmptyHint>
              ) : (
                <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 6 }}>
                  {statement.charges.map((charge) => (
                    <li key={charge.id} style={{ display: "flex", justifyContent: "space-between", gap: 14, fontSize: 14.5, color: "var(--text-muted)" }}>
                      <span>
                        {charge.description} · {charge.dueDate}
                      </span>
                      <span style={{ color: "var(--text)", fontWeight: 600 }}>{formatCents(charge.remainingCents)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>

            <div>
              <h3 style={{ fontSize: 15, fontWeight: 700, color: "var(--text)", marginBottom: 8 }}>{t("account.payments")}</h3>
              {statement.payments.length === 0 ? (
                <EmptyHint>{t("state.empty.payments")}</EmptyHint>
              ) : (
                <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "grid", gap: 6 }}>
                  {statement.payments.map((payment) => (
                    <li key={payment.id} style={{ display: "flex", justifyContent: "space-between", gap: 14, fontSize: 14.5, color: "var(--text-muted)" }}>
                      <span>
                        {payment.paidAt} · {payment.method}
                        {payment.reference !== "" && ` · ${payment.reference}`}
                      </span>
                      <span style={{ color: "var(--text)", fontWeight: 600 }}>{formatCents(payment.amountCents)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          </div>
        )}
      </div>
    </Section>
  );
}
