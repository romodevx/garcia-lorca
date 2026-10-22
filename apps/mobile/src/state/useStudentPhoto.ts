import { useMemo } from "react";
import { getCardPhoto } from "../api";
import { useAsyncResource, type AsyncResource } from "./useAsyncResource";

/**
 * One child's photo as a data URI, or null when the school has none. A child
 * switch or an unmount discards the in-flight download (useAsyncResource).
 */
export function useStudentPhoto(studentId: string | null): AsyncResource<string | null> {
  const load = useMemo(() => (studentId ? () => getCardPhoto(studentId) : null), [studentId]);
  return useAsyncResource(load);
}
