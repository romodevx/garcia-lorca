"use client";

import { useEffect, useState, type FormEvent } from "react";
import { announce, type AnnouncementScope } from "../../comms/api";
import { listCycles, listLabelledGroups, listLevels, type Cycle, type LabelledGroup, type Level } from "../../config/api";
import { listStudentsWithRooms, type Student } from "../../students/api";
import { API_URL } from "../../api-url";

/** One key per attempt: a retry must not buzz four hundred phones twice. */
function newAttemptKey(): string {
  return globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/** What the last successful send reached. */
export interface SendResult {
  created: number;
  audience: number;
}

/** D5: "Enviado a 28 alumnos", singular at one, and whether it was a repeat. */
export function sentMessage({ created, audience }: SendResult): string {
  const reached = `Enviado a ${audience} ${audience === 1 ? "alumno" : "alumnos"}`;
  return created === 0 ? `${reached} · ya se había enviado, no se repitió` : reached;
}

/** Whether each list's own load has settled; A2–A4 fail silently, as empty. */
export interface ListsLoaded {
  groups: boolean;
  levels: boolean;
  students: boolean;
}

/** Everything a view of /avisos needs: the data, the draft and the rules. */
export interface AnnouncementComposer {
  cycle: Cycle | undefined;
  levels: Level[];
  groups: LabelledGroup[];
  students: Student[];
  loaded: ListsLoaded;
  scope: AnnouncementScope;
  picked: string[];
  title: string;
  body: string;
  sending: boolean;
  sent: SendResult | null;
  error: string | undefined;
  /** B2: a new audience starts with nothing picked. */
  chooseScope: (scope: AnnouncementScope) => void;
  /** B4 (several groups or levels) and B5 (one pupil at a time). */
  toggle: (id: string) => void;
  setTitle: (title: string) => void;
  setBody: (body: string) => void;
  /** D1–D8: validates in order, sends once, and keeps the key on failure. */
  submit: (e: FormEvent) => Promise<void>;
}

/**
 * The Avisos screen without its looks (PLAN-redesign-avisos §3.1): the loads,
 * the audience, the draft and the send. Every view of /avisos goes through
 * here, so they call exactly the same endpoints with the same arguments.
 */
export function useAnnouncementComposer(): AnnouncementComposer {
  const [cycles, setCycles] = useState<Cycle[]>([]);
  const [levels, setLevels] = useState<Level[]>([]);
  const [groups, setGroups] = useState<LabelledGroup[]>([]);
  const [scope, setScope] = useState<AnnouncementScope>("group");
  // A list, always: one pupil is a list of one, and "1° A y 1° B" is a list of
  // two. Keeping two shapes for the same question is how the send ends up
  // addressing the wrong people.
  const [picked, setPicked] = useState<string[]>([]);
  const [students, setStudents] = useState<Student[]>([]);
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [sending, setSending] = useState(false);
  const [sent, setSent] = useState<SendResult | null>(null);
  const [error, setError] = useState<string | undefined>(undefined);
  const [attemptKey, setAttemptKey] = useState(newAttemptKey);
  const [cyclesSettled, setCyclesSettled] = useState(false);
  const [levelsSettled, setLevelsSettled] = useState(false);
  const [studentsSettled, setStudentsSettled] = useState(false);
  const [groupsSettled, setGroupsSettled] = useState(false);

  const cycle = cycles.find((c) => c.status === "active") ?? cycles[0];

  useEffect(() => {
    listCycles(API_URL).then(setCycles).catch(() => setError("No pudimos leer los ciclos.")).finally(() => setCyclesSettled(true));
    listLevels(API_URL).then(setLevels).catch(() => {}).finally(() => setLevelsSettled(true));
    listStudentsWithRooms(API_URL).then(setStudents).catch(() => {}).finally(() => setStudentsSettled(true));
  }, []);

  useEffect(() => {
    if (cycle === undefined) {
      return;
    }
    listLabelledGroups(API_URL, cycle.id).then(setGroups).catch(() => {}).finally(() => setGroupsSettled(true));
  }, [cycle]);

  const chooseScope = (next: AnnouncementScope): void => {
    setScope(next);
    setPicked([]);
  };

  const toggle = (id: string): void => {
    setPicked((prev) => {
      if (prev.includes(id)) {
        return prev.filter((p) => p !== id);
      }
      // One pupil at a time: a note to a family is not a circular, and picking
      // several would be a group announcement wearing the wrong label.
      return scope === "student" ? [id] : [...prev, id];
    });
  };

  const submit = async (e: FormEvent): Promise<void> => {
    e.preventDefault();
    if (cycle === undefined) {
      setError("No hay un ciclo activo al que dirigir el aviso.");
      return;
    }
    if (title.trim() === "" || body.trim() === "") {
      setError("Un aviso necesita título y mensaje.");
      return;
    }
    if (scope !== "cycle" && picked.length === 0) {
      setError("Elige a quién va dirigido.");
      return;
    }
    setSending(true);
    setError(undefined);
    try {
      const result = await announce(API_URL, {
        scope,
        ...(scope === "cycle" ? {} : { targetIds: picked }),
        cycleId: cycle.id,
        title: title.trim(),
        body: body.trim(),
        idempotencyKey: attemptKey,
      });
      setSent(result);
      setTitle("");
      setBody("");
      setPicked([]);
      // A fresh key for the next message: the old one now means THIS junta, and
      // reusing it would silently deliver nothing.
      setAttemptKey(newAttemptKey());
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "No se pudo enviar el aviso.");
    } finally {
      setSending(false);
    }
  };

  // Without a cycle there are no groups to wait for.
  const loaded: ListsLoaded = {
    groups: groupsSettled || (cyclesSettled && cycle === undefined),
    levels: levelsSettled,
    students: studentsSettled,
  };

  return {
    cycle, levels, groups, students, loaded, scope, picked, title, body, sending, sent, error,
    chooseScope, toggle, setTitle, setBody, submit,
  };
}
