"use client";

import { useCallback, useEffect, useState } from "react";

import { API_URL } from "../../api-url";
import { getFamily, listFamilyContacts, type FamilyDetail, type FamilyPerson } from "../../families/api";

export type DetailStatus = "loading" | "ok" | "failed";

export interface FamilyDetailState {
  detail: FamilyDetail | undefined;
  /** The adults; always empty without guardians:read (never asked for). */
  people: readonly FamilyPerson[];
  status: DetailStatus;
  peopleFailed: boolean;
  reload: () => void;
}

/** One family and, for a caller holding guardians:read, its adults. */
export function useFamilyDetail(familyId: string, readAdults: boolean): FamilyDetailState {
  const [detail, setDetail] = useState<FamilyDetail | undefined>(undefined);
  const [people, setPeople] = useState<FamilyPerson[]>([]);
  const [status, setStatus] = useState<DetailStatus>("loading");
  const [peopleFailed, setPeopleFailed] = useState(false);
  const [version, setVersion] = useState(0);

  useEffect(() => {
    let alive = true;
    getFamily(API_URL, familyId)
      .then((d) => { if (alive) { setDetail(d); setStatus("ok"); } })
      .catch(() => { if (alive) setStatus("failed"); });
    if (readAdults) {
      listFamilyContacts(API_URL, familyId)
        .then((rows) => { if (alive) { setPeople(rows); setPeopleFailed(false); } })
        .catch(() => { if (alive) setPeopleFailed(true); });
    }
    return () => { alive = false; };
  }, [familyId, readAdults, version]);

  const reload = useCallback(() => setVersion((v) => v + 1), []);
  return { detail, people, status, peopleFailed, reload };
}
