"use client";

import { useCallback, useEffect, useState, type ReactElement } from "react";
import { Icon } from "../../_design/Icon";
import { listGroups, listCycles, type Cycle, type Group } from "../../config/api";
import {
  createStudent,
  enroll,
  listEnrollments,
  listStudents,
  transfer,
  withdraw,
  type CreateStudentInput,
  type EnrollInput,
  type Enrollment as EnrollmentRecord,
  type Student,
  type TransferInput,
} from "../../students/api";
import { EnrollPanel } from "./EnrollPanel";
import { RosterPanel } from "./RosterPanel";
import { StudentCreatePanel } from "./StudentCreatePanel";
import { StudentDirectoryPanel } from "./StudentDirectoryPanel";

import { API_URL } from "../../api-url";

// Gateway base URL for the browser (public env; matches AppShell/compose).

export interface EnrollmentProps {
  onBack: () => void;
}

/**
 * Enrollment surface (module_students). Centralizes data fetching — students
 * (from the students service) and cycles/groups (from config) — and passes plain
 * data + async callbacks down to two dumb panels: EnrollPanel (place a student
 * into a group; cycle derived server-side) and RosterPanel (a group's roster with
 * transfer + withdraw). The gateway (behind the flag + students:read/enroll) is
 * the security boundary; this screen is UX only.
 */
export function Enrollment({ onBack }: EnrollmentProps): ReactElement {
  const [students, setStudents] = useState<Student[]>([]);
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | undefined>(undefined);

  // Boot: load the enrollable students + the cycles catalog.
  useEffect(() => {
    let active = true;
    setLoading(true);
    Promise.all([listStudents(API_URL), listCycles(API_URL)])
      .then(([ss, cs]) => {
        if (!active) return;
        setStudents(ss);
        setCycles(cs);
      })
      .catch(() => active && setLoadError("No se pudieron cargar los datos de inscripción."))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  // Loaders kept here so this orchestrator stays the single fetch owner; memoized
  // so the panels' cascade effects don't re-fire.
  const loadGroups = useCallback((cycleId: string): Promise<Group[]> => listGroups(API_URL, cycleId), []);
  const loadEnrollments = useCallback(
    (groupId: string): Promise<EnrollmentRecord[]> => listEnrollments(API_URL, { groupId }),
    [],
  );

  const handleCreateStudent = useCallback(async (input: CreateStudentInput): Promise<void> => {
    const created = await createStudent(API_URL, input);
    // Prepend so the freshly registered applicant is immediately enrollable in
    // the panel beside this one — no page reload needed.
    setStudents((prev) => [created, ...prev]);
  }, []);

  const handleEnroll = useCallback(async (input: EnrollInput): Promise<void> => {
    await enroll(API_URL, input);
  }, []);
  const handleTransfer = useCallback(async (input: TransferInput): Promise<void> => {
    await transfer(API_URL, input);
  }, []);
  const handleWithdraw = useCallback(async (enrollmentId: string): Promise<void> => {
    await withdraw(API_URL, enrollmentId);
  }, []);

  return (
    <div style={{ maxWidth: 1320, margin: "0 auto", padding: "24px 28px 64px" }}>
      <button
        onClick={onBack}
        className="fade-up"
        style={{ display: "inline-flex", alignItems: "center", gap: 7, color: "var(--text-muted)", fontSize: 14.5, fontWeight: 600, marginBottom: 14 }}
      >
        <Icon name="back" size={18} stroke={2.4} />
        Volver al inicio
      </button>

      <div className="fade-up" style={{ display: "flex", alignItems: "center", gap: 16, marginBottom: 24 }}>
        <div style={{ width: 56, height: 56, borderRadius: 18, display: "grid", placeItems: "center", background: "var(--surface-3)", color: "var(--text-muted)" }}>
          <Icon name="students" size={30} />
        </div>
        <div>
          <h1 style={{ fontSize: 30, fontWeight: 800, color: "var(--text)" }}>Inscripciones</h1>
          <p style={{ color: "var(--text-muted)", fontSize: 15, marginTop: 3 }}>Inscribe alumnos, consulta el roster y traslada entre grupos</p>
        </div>
      </div>

      {loading ? (
        <p className="fade-up" style={{ color: "var(--text-muted)", fontSize: 15 }}>Cargando…</p>
      ) : loadError !== undefined ? (
        <p role="alert" className="fade-up" style={{ color: "var(--bad-fg)", fontSize: 15 }}>{loadError}</p>
      ) : (
        <div className="fade-up" style={{ display: "grid", gap: 18 }}>
          <StudentDirectoryPanel students={students} />
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(360px, 1fr))", gap: 18 }}>
            <StudentCreatePanel onCreate={handleCreateStudent} />
            <EnrollPanel students={students} cycles={cycles} loadGroups={loadGroups} onEnroll={handleEnroll} />
          <RosterPanel
            students={students}
            cycles={cycles}
            loadGroups={loadGroups}
            loadEnrollments={loadEnrollments}
            onTransfer={handleTransfer}
            onWithdraw={handleWithdraw}
          />
          </div>
        </div>
      )}
    </div>
  );
}
