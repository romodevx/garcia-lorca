/**
 * Typed fetch helpers for the gateway's comms endpoints (`/api/v1/comms/*`),
 * gated behind the `module_comms` flag + cookie auth.
 *
 * The school SENDS through this module; what a family receives is materialised
 * from other services' events and read elsewhere. The only write here is the
 * announcement: one message to a group, a level, or the whole school.
 */

import { authFetch } from "../auth/authFetch";

const BASE_PATH = "/api/v1/comms";

/** Who an announcement is for. */
export type AnnouncementScope = "student" | "group" | "level" | "cycle";

export interface AnnounceInput {
  scope: AnnouncementScope;
  /** The pupil, group or level. Unused when the scope is the whole cycle. */
  targetId?: string;
  /** Several groups (or levels) at once: one message, written once. */
  targetIds?: string[];
  cycleId: string;
  title: string;
  body: string;
  /**
   * One key per attempt. An announcement can reach four hundred families, and a
   * phone buzzing twice for the same junta is what makes a school turn
   * notifications off for good.
   */
  idempotencyKey: string;
}

export interface AnnounceResult {
  /** How many families it actually reached. A retry reports zero. */
  created: number;
  /** How many pupils the scope named. */
  audience: number;
}

/** Sends one message to every active pupil the scope names. */
export async function announce(baseURL: string, input: AnnounceInput): Promise<AnnounceResult> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}${BASE_PATH}/announcements`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      scope: input.scope,
      ...(input.targetIds !== undefined && input.targetIds.length > 0
        ? { targetIds: input.targetIds }
        : input.targetId !== undefined && input.targetId !== ""
          ? { targetId: input.targetId }
          : {}),
      cycleId: input.cycleId,
      title: input.title,
      body: input.body,
      idempotencyKey: input.idempotencyKey,
    }),
  });
  if (!res.ok) {
    const detail = (await res.text().catch(() => "")).trim();
    throw new Error(detail !== "" ? detail : `HTTP ${res.status}`);
  }
  const data = (await res.json()) as { created?: number; audience?: number };
  return { created: data.created ?? 0, audience: data.audience ?? 0 };
}

// --- birthday greeting (T11) ------------------------------------------------

/**
 * The greeting comms sends every morning to the families of the pupils whose
 * birthday it is. `{nombre}` becomes the pupil's first name. `isDefault` is
 * true while the school has not saved its own text.
 */
export interface BirthdayTemplate {
  title: string;
  body: string;
  isDefault: boolean;
  /** RFC3339 instant of the last save; empty while isDefault. */
  updatedAt: string;
}

export interface BirthdayTemplateInput {
  title: string;
  body: string;
}

async function readTemplate(res: Response): Promise<BirthdayTemplate> {
  if (!res.ok) {
    const detail = (await res.text().catch(() => "")).trim();
    throw new Error(detail !== "" ? detail : `HTTP ${res.status}`);
  }
  const data = (await res.json()) as Partial<BirthdayTemplate>;
  return {
    title: data.title ?? "",
    body: data.body ?? "",
    isDefault: data.isDefault === true,
    updatedAt: data.updatedAt ?? "",
  };
}

/** Reads the school's birthday greeting (or the default text). */
export async function getBirthdayTemplate(baseURL: string): Promise<BirthdayTemplate> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}${BASE_PATH}/birthday-template`, {
    credentials: "include",
  });
  return readTemplate(res);
}

/** Replaces the school's birthday greeting. Applies from the next morning's run. */
export async function updateBirthdayTemplate(baseURL: string, input: BirthdayTemplateInput): Promise<BirthdayTemplate> {
  const res = await authFetch(`${baseURL.replace(/\/+$/, "")}${BASE_PATH}/birthday-template`, {
    method: "PUT",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ title: input.title, body: input.body }),
  });
  return readTemplate(res);
}
