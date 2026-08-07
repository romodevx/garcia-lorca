"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { listCycles, listGroups, type Cycle, type Group } from "../../config/api";
import {
  applyDiscount,
  cancelCharge,
  createConcept,
  generateCharges,
  getAccount,
  getOverdue,
  listCharges,
  listConcepts,
  registerPayment,
  updateConcept,
  type AccountStatement,
  type Charge,
  type ChargeConcept,
  type CreateConceptInput,
  type GenerateChargesInput,
  type RegisterPaymentInput,
  type UpdateConceptInput,
} from "../../finance/api";
import { listStudents, type Student } from "../../students/api";
import type { ChargeFilter } from "./ChargesPanel";
import { formatCents } from "./money";

// Gateway base URL for the browser (public env; matches AppShell/compose).
const API_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";
const TOAST_MS = 2600;

/** The finance surface's sections; also the palette's deep-link targets. */
export type FinanceSection = "charges" | "payment" | "concepts" | "generate" | "account";

const detailOf = (cause: unknown): string => (cause instanceof Error ? cause.message : String(cause));

/** Everything the finance view needs: data, view state, and the write handlers. */
export interface FinanceState {
  section: FinanceSection;
  setSection: (section: FinanceSection) => void;
  cycles: readonly Cycle[];
  students: readonly Student[];
  concepts: readonly ChargeConcept[];
  groups: readonly Group[];
  charges: readonly Charge[];
  cycleId: string;
  setCycleId: (cycleId: string) => void;
  studentId: string;
  setStudentId: (studentId: string) => void;
  filter: ChargeFilter;
  setFilter: (filter: ChargeFilter) => void;
  includeInactive: boolean;
  setIncludeInactive: (include: boolean) => void;
  selectedChargeId: string;
  setSelectedChargeId: (chargeId: string) => void;
  loadingCharges: boolean;
  /** Set ONLY when the shared boot pickers fail: the surface is not usable. */
  loadError?: string;
  /** Set when only the charge read fails: the rest of the surface still works. */
  chargesError?: string;
  /** Set when only the concept read fails; shown inside the concepts panel. */
  conceptsError?: string;
  toast?: string;
  studentName: (studentId: string) => string;
  openPayment: (charge: Charge) => void;
  handlePayment: (input: RegisterPaymentInput) => Promise<void>;
  handleCreateConcept: (input: CreateConceptInput) => Promise<void>;
  handleUpdateConcept: (id: string, input: UpdateConceptInput) => Promise<void>;
  handleGenerate: (input: GenerateChargesInput) => Promise<void>;
  handleLoadAccount: (studentId: string) => Promise<AccountStatement>;
  /** Forgive part of a charge; rejects with the gateway's reason. */
  handleDiscount: (chargeId: string, discountCents: number, reason: string) => Promise<void>;
  /** Void a charge; rejects with the gateway's reason. */
  handleCancelCharge: (chargeId: string) => Promise<void>;
}

/**
 * All state, reads and writes of the finance surface — the single fetch owner,
 * kept out of the view so every panel stays presentational.
 *
 *   boot ──> cycles + students ──> cycleId ─┐
 *   includeInactive ──> concepts            ├─> charges (overdue | filtered)
 *   cycleId ──> groups                      ┘
 *
 * Write handlers deliberately DO NOT swallow gateway failures: they reject so the
 * submitting panel can show the reason (e.g. an overpay) next to its form. Money
 * is integer CENTS throughout; only toasts format pesos.
 */
export function useFinance(initialSection: FinanceSection): FinanceState {
  const t = useTranslations("finance");
  const [section, setSection] = useState<FinanceSection>(initialSection);
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [concepts, setConcepts] = useState<ChargeConcept[]>([]);
  const [groups, setGroups] = useState<Group[]>([]);
  const [charges, setCharges] = useState<Charge[]>([]);
  const [cycleId, setCycleId] = useState("");
  const [studentId, setStudentId] = useState("");
  const [filter, setFilter] = useState<ChargeFilter>("overdue");
  const [includeInactive, setIncludeInactive] = useState(false);
  const [selectedChargeId, setSelectedChargeId] = useState("");
  const [booted, setBooted] = useState(false);
  const [loadingCharges, setLoadingCharges] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);
  const [chargesError, setChargesError] = useState<string | undefined>(undefined);
  const [conceptsError, setConceptsError] = useState<string | undefined>(undefined);
  const [toast, setToast] = useState<string | undefined>(undefined);
  // Monotonic id of the newest charge read; an older response is dropped so a
  // slow request can never repaint a previous cycle's/student's table.
  const chargesRequest = useRef(0);

  const announce = useCallback((message: string): void => {
    setToast(message);
    window.setTimeout(() => setToast(undefined), TOAST_MS);
  }, []);

  // Boot: the pickers every panel shares. The active cycle (or the first one)
  // seeds the filters so the debts table opens on real data.
  useEffect(() => {
    let active = true;
    Promise.all([listCycles(API_URL), listStudents(API_URL)])
      .then(([cs, ss]) => {
        if (!active) return;
        setCycles(cs);
        setStudents(ss);
        setCycleId((cs.find((cycle) => cycle.status === "active") ?? cs[0])?.id ?? "");
      })
      .catch(() => active && setLoadError(t("error.load")))
      .finally(() => active && setBooted(true));
    return () => {
      active = false;
    };
  }, [t]);

  // Concepts: re-read when the archived ones are toggled in. A failure here is
  // the concepts panel's own problem — it must NOT blank the debts table.
  useEffect(() => {
    let active = true;
    setConceptsError(undefined);
    listConcepts(API_URL, includeInactive)
      .then((cs) => {
        if (active) setConcepts(cs);
      })
      .catch((cause: unknown) => {
        if (active) setConceptsError(t("error.concepts", { detail: detailOf(cause) }));
      });
    return () => {
      active = false;
    };
  }, [includeInactive, t]);

  // Groups of the selected cycle (the bulk-generation scope).
  useEffect(() => {
    if (cycleId === "") return;
    let active = true;
    listGroups(API_URL, cycleId)
      .then((gs) => active && setGroups(gs))
      .catch(() => active && setGroups([]));
    return () => {
      active = false;
    };
  }, [cycleId]);

  // The debts table: the overdue queue, or a student's/cycle's charges. Reads can
  // overlap (every filter change starts one), so only the newest may write state.
  const reloadCharges = useCallback(async (): Promise<void> => {
    const request = (chargesRequest.current += 1);
    const isCurrent = (): boolean => request === chargesRequest.current;
    setLoadingCharges(true);
    setChargesError(undefined);
    try {
      const rows = filter === "overdue" ? await getOverdue(API_URL, cycleId) : await listCharges(API_URL, { studentId, cycleId });
      if (isCurrent()) setCharges(rows);
    } catch (cause) {
      if (isCurrent()) {
        setCharges([]);
        setChargesError(t("error.charges", { detail: detailOf(cause) }));
      }
    } finally {
      if (isCurrent()) setLoadingCharges(false);
    }
  }, [filter, cycleId, studentId, t]);

  useEffect(() => {
    if (!booted) return;
    void reloadCharges();
  }, [booted, reloadCharges]);

  const studentName = useCallback(
    (id: string): string => {
      const student = students.find((candidate) => candidate.id === id);
      return student !== undefined ? `${student.firstName} ${student.lastName}` : id;
    },
    [students],
  );

  /** Row action: jump to the window form with this charge preselected. */
  const openPayment = useCallback((charge: Charge): void => {
    setSelectedChargeId(charge.id);
    setSection("payment");
  }, []);

  /** Every money write answers with the recomputed charge, so the row updates in place. */
  const replaceCharge = useCallback((updated: Charge): void => {
    setCharges((prev) => prev.map((row) => (row.id === updated.id ? updated : row)));
  }, []);

  const handlePayment = useCallback(
    async (input: RegisterPaymentInput): Promise<void> => {
      // The response carries the recomputed charge, so no follow-up read is needed.
      const { payment, charge } = await registerPayment(API_URL, input);
      replaceCharge(charge);
      if (charge.remainingCents <= 0) setSelectedChargeId("");
      announce(t("toast.payment", { amount: formatCents(payment.amountCents) }));
    },
    [announce, replaceCharge, t],
  );

  const handleCreateConcept = useCallback(
    async (input: CreateConceptInput): Promise<void> => {
      const created = await createConcept(API_URL, input);
      setConcepts((prev) => [...prev, created]);
      announce(t("toast.conceptCreated", { name: created.name }));
    },
    [announce, t],
  );

  const handleUpdateConcept = useCallback(
    async (id: string, input: UpdateConceptInput): Promise<void> => {
      const updated = await updateConcept(API_URL, id, input);
      setConcepts((prev) => prev.map((concept) => (concept.id === updated.id ? updated : concept)));
      announce(t("toast.conceptUpdated", { name: updated.name }));
    },
    [announce, t],
  );

  const handleGenerate = useCallback(
    async (input: GenerateChargesInput): Promise<void> => {
      const { created, skipped } = await generateCharges(API_URL, input);
      announce(t("toast.generated", { created, skipped }));
      await reloadCharges();
    },
    [announce, reloadCharges, t],
  );

  const handleLoadAccount = useCallback((id: string): Promise<AccountStatement> => getAccount(API_URL, id), []);

  const handleDiscount = useCallback(
    async (chargeId: string, discountCents: number, reason: string): Promise<void> => {
      replaceCharge(await applyDiscount(API_URL, chargeId, discountCents, reason));
      announce(t("toast.discount", { amount: formatCents(discountCents) }));
    },
    [announce, replaceCharge, t],
  );

  const handleCancelCharge = useCallback(
    async (chargeId: string): Promise<void> => {
      replaceCharge(await cancelCharge(API_URL, chargeId));
      announce(t("toast.cancelled"));
    },
    [announce, replaceCharge, t],
  );

  return {
    section,
    setSection,
    cycles,
    students,
    concepts,
    groups,
    charges,
    cycleId,
    setCycleId,
    studentId,
    setStudentId,
    filter,
    setFilter,
    includeInactive,
    setIncludeInactive,
    selectedChargeId,
    setSelectedChargeId,
    loadingCharges,
    loadError,
    chargesError,
    conceptsError,
    toast,
    studentName,
    openPayment,
    handlePayment,
    handleCreateConcept,
    handleUpdateConcept,
    handleGenerate,
    handleLoadAccount,
    handleDiscount,
    handleCancelCharge,
  };
}
