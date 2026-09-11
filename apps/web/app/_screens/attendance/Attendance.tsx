"use client";

import type { ReactElement } from "react";
import { useTranslations } from "next-intl";
import { Icon } from "../../_design/Icon";
import { Page } from "../../_design/Page";
import { Toast } from "../../_design/Toast";
import { PassList } from "./PassList";
import { DateField, SelectField, type SelectOption } from "../../_design/form";
import { EmptyHint, ErrorAlert, Section, StatusCounters } from "./parts";
import { usePassList } from "./usePassList";

export interface AttendanceProps {
  onBack: () => void;
  /** Pins the day (tests, deep links); defaults to the local today. */
  today?: string;
}

/**
 * Daily pass-list surface (module_attendance): pick a group and a day, and the
 * whole roster opens PRESENT. The teacher marks only the exceptions and one
 * "Guardar" reconciles the day — the payload carries the exceptions alone.
 *
 * All reads/writes live in `usePassList`; this component only composes the design
 * system around them. The gateway (behind module_attendance + attendance:read /
 * attendance:write) is the security boundary — this screen is UX only.
 */
export function Attendance({ onBack, today }: AttendanceProps): ReactElement {
  const t = useTranslations("attendance");
  const list = usePassList(today);

  const cycleOptions: SelectOption[] = list.cycles.map((cycle) => ({ value: cycle.id, label: cycle.name }));
  const groupOptions: SelectOption[] = list.groups.map((group) => ({ value: group.id, label: group.name }));

  return (
    <Page>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        {t("action.back")}
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--surface-3)", color: "var(--text-muted)" }}>
          <Icon name="qr" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>{t("title")}</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>{t("subtitle")}</p>
        </div>
      </div>

      {list.loadError !== undefined ? (
        <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>
          {list.loadError}
        </p>
      ) : (
        <div className="fade-up">
          <Section title={t("list.title")} subtitle={t("list.subtitle")}>
            <form aria-label={t("picker.label")}>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12, marginBottom: 16 }}>
                <SelectField label={t("field.cycle")} value={list.cycleId} options={cycleOptions} onChange={list.setCycleId} />
                <SelectField
                  label={t("field.group")}
                  value={list.groupId}
                  options={groupOptions}
                  onChange={list.setGroupId}
                  placeholder={t("option.chooseGroup")}
                  disabled={list.groups.length === 0}
                />
                <DateField label={t("field.date")} value={list.date} onChange={list.setDate} />
              </div>
            </form>

            {list.rows.length > 0 && (
              <div style={{ display: "flex", flexWrap: "wrap", alignItems: "center", justifyContent: "space-between", gap: 12, marginBottom: 14 }}>
                <StatusCounters
                  counters={[
                    { status: "present", label: t("summary.present"), count: list.counts.present },
                    { status: "absent", label: t("summary.absent"), count: list.counts.absent },
                    { status: "late", label: t("summary.late"), count: list.counts.late },
                    { status: "excused", label: t("summary.excused"), count: list.counts.excused },
                  ]}
                />
                <button
                  onClick={() => void list.save()}
                  disabled={list.saving}
                  style={{
                    height: 44,
                    padding: "0 22px",
                    borderRadius: "var(--r-pill)",
                    background: "var(--primary)",
                    color: "var(--primary-ink)",
                    fontWeight: 700,
                    fontSize: 15,
                    opacity: list.saving ? 0.6 : 1,
                  }}
                >
                  {list.saving ? t("action.saving") : t("action.save")}
                </button>
              </div>
            )}

            {list.groupId === "" ? (
              <EmptyHint>{list.groups.length === 0 ? t("state.noGroups") : t("state.pickGroup")}</EmptyHint>
            ) : list.loadingRoster ? (
              <EmptyHint>{t("state.loadingRoster")}</EmptyHint>
            ) : list.rosterError !== undefined ? (
              <ErrorAlert message={list.rosterError} />
            ) : list.rows.length === 0 ? (
              <EmptyHint>{t("state.emptyRoster")}</EmptyHint>
            ) : (
              <PassList rows={list.rows} studentName={list.studentName} onStatusChange={list.setStatus} />
            )}

            {list.saveError !== undefined && <ErrorAlert message={list.saveError} />}
          </Section>
        </div>
      )}

      {list.toast !== undefined && <Toast>{list.toast}</Toast>}
    </Page>
  );
}
