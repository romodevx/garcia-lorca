"use client";

import { useCallback, useEffect, useMemo, useState } from "react";

import { API_URL } from "../../api-url";
import { listFamilies, type Family } from "../../families/api";
import { membershipOf } from "../../families/model";

export interface FamilyDirectory {
  families: readonly Family[];
  /** Each pupil's family, by pupil id. */
  membership: ReadonlyMap<string, Family>;
  failed: boolean;
  reload: () => void;
}

const NONE: readonly Family[] = [];

/**
 * The school's families, read ONCE for the whole Alumnos screen: the Familias
 * view, the directory's "Familia" column and its "Sin familia" chip all read
 * this list. Membership comes from it rather than from each pupil's
 * `familyId`, so a family change re-reads one list, not six hundred pupils.
 * `enabled` false (module_families off) reads nothing at all.
 */
export function useFamilyDirectory(enabled: boolean): FamilyDirectory {
  const [families, setFamilies] = useState<readonly Family[]>(NONE);
  const [failed, setFailed] = useState(false);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    if (!enabled) return;
    let alive = true;
    listFamilies(API_URL)
      .then((rows) => { if (alive) { setFamilies(rows); setFailed(false); } })
      .catch(() => { if (alive) setFailed(true); });
    return () => { alive = false; };
  }, [enabled, version]);

  const reload = useCallback(() => setVersion((v) => v + 1), []);
  const membership = useMemo(() => membershipOf(families), [families]);
  return { families, membership, failed, reload };
}
