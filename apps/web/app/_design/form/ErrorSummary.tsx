"use client";

import { useEffect, useRef, type MouseEvent, type ReactElement } from "react";

import { Icon } from "../Icon";

/**
 * The list of what stopped a save, at the top of a form page.
 *
 * A long form marks every bad field in red, but the eye is on the button at
 * the bottom: the summary is where the failed save lands focus (GOV.UK and
 * the house rule ui-forms-and-modals), and each line takes the user to its
 * field. The inline messages stay; the wording is the same in both places.
 */
export interface ErrorSummaryItem {
  /** The field's visible label: the line links to the control it labels. */
  field: string;
  message: string;
}

export interface ErrorSummaryProps {
  items: readonly ErrorSummaryItem[];
  /**
   * Bumped on every failed save. A counter, not a boolean: a second failure
   * with the same errors must take focus again.
   */
  attempt: number;
}

const TITLE = "Revisa esto antes de guardar";

/** The control a visible label points at, if it is on the page. */
function controlLabelled(field: string): HTMLElement | null {
  const label = Array.from(document.querySelectorAll("label")).find((l) => l.textContent?.trim() === field);
  return label?.htmlFor ? document.getElementById(label.htmlFor) : null;
}

/**
 * Focuses the field a visible label names, and scrolls it clear of any
 * sticky header or footer. The form layer's fields take no ref (their id
 * comes from Field), so the label is the stable handle.
 */
export function focusFieldLabelled(field: string): void {
  const control = controlLabelled(field);
  control?.focus({ preventScroll: true });
  control?.scrollIntoView?.({ block: "center", inline: "nearest" });
}

function goToField(event: MouseEvent<HTMLAnchorElement>, field: string): void {
  event.preventDefault();
  focusFieldLabelled(field);
}

export function ErrorSummary({ items, attempt }: ErrorSummaryProps): ReactElement | null {
  const ref = useRef<HTMLDivElement>(null);
  const visible = items.length > 0;

  useEffect(() => {
    if (attempt === 0 || ref.current === null) return;
    ref.current.focus({ preventScroll: true });
    // `center`, not the minimum: a sticky header must never cover it (WCAG 2.2 · 2.4.11).
    ref.current.scrollIntoView?.({ block: "center", inline: "nearest" });
  }, [attempt, visible]);

  if (!visible) return null;
  return (
    <div
      ref={ref}
      role="group"
      aria-labelledby="error-summary-title"
      tabIndex={-1}
      style={{ padding: "14px 18px", borderRadius: "var(--r-md)", background: "var(--bad-bg)", border: "1px solid var(--red)", color: "var(--bad-fg)" }}
    >
      <h2 id="error-summary-title" style={{ display: "flex", alignItems: "center", gap: 7, fontSize: 15.5, fontWeight: 800, margin: 0 }}>
        <Icon name="warning" size={17} stroke={2.4} />
        {TITLE}
      </h2>
      <ul style={{ margin: "8px 0 0", paddingLeft: 20, display: "grid", gap: 4 }}>
        {items.map((item) => (
          <li key={item.field}>
            <a href="#" onClick={(e) => goToField(e, item.field)} style={{ color: "inherit", fontWeight: 600, fontSize: 14, textDecoration: "underline" }}>
              {`${item.field}: ${item.message}`}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
