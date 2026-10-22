import { type ReactElement } from "react";
import { useAppState } from "../state/AppState";
import { ScreenScroll, type ScreenScrollProps } from "./ScreenScroll";

/**
 * A tab's ScreenScroll, a new one for each selected child: another child's
 * screen opens at the top, not at the offset the previous child's was left at
 * (QA 21-oct: Cuenta opened at the bottom of the other child's ledger). The
 * screen under it remounts too, which is what a different child's page is.
 */
export function ChildScreenScroll(props: ScreenScrollProps): ReactElement {
  const { selectedChildId } = useAppState();
  return <ScreenScroll key={selectedChildId ?? "no-child"} {...props} />;
}
