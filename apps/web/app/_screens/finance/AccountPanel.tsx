"use client";

import { useRef, useState, type ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Btn } from "../../_design/Controls";
import type { AccountStatement } from "../../finance/api";
import type { Student } from "../../students/api";
import { formatCents } from "./money";
import { EmptyHint, ErrorAlert, Section, SelectField } from "./parts";

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
      <div style={{ display: "grid", gap: 16, maxWidth: 820 }}>
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
                display: "flex",
                alignItems: "baseline",
                gap: 12,
                padding: "16px 20px",
                borderRadius: "var(--r-lg)",
                background: "var(--surface-3)",
                border: "1px solid var(--border)",
              }}
            >
              <span style={{ fontSize: 14, fontWeight: 600, color: "var(--text-muted)" }}>{t("account.balance")}</span>
              <span style={{ fontFamily: "var(--font-display)", fontWeight: 700, fontSize: 26, color: "var(--text)" }}>
                {formatCents(statement.totalBalanceCents)}
              </span>
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
