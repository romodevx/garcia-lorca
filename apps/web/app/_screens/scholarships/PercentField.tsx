"use client";

import type { ChangeEvent, ReactElement } from "react";

import { Field, inputStyle, type BaseFieldProps } from "../../_design/form";

/** Width kept clear on the right for the "%" suffix. */
const SUFFIX_WIDTH = 34;

/**
 * A percent with up to two decimals ("12.5"), the "%" drawn beside it. It
 * stays text: the caller turns it into basis points on save, so a typo is
 * shown back, never read as some other share.
 */
export function PercentField(props: BaseFieldProps): ReactElement {
  const { label, value, onChange, required, error, onClearError, help, onBlur } = props;
  return (
    <Field label={label} required={required} error={error} help={help}>
      {(id, describedBy) => (
        <div style={{ position: "relative", minWidth: 0 }}>
          <input
            id={id}
            type="text"
            inputMode="decimal"
            value={value}
            aria-describedby={describedBy}
            aria-invalid={error !== undefined ? true : undefined}
            aria-required={required === true ? true : undefined}
            onChange={(e: ChangeEvent<HTMLInputElement>) => {
              onChange(e.target.value);
              onClearError?.();
            }}
            onBlur={onBlur}
            style={{ ...inputStyle(error !== undefined), paddingRight: SUFFIX_WIDTH }}
          />
          <span aria-hidden="true" style={{ position: "absolute", right: 14, top: "50%", transform: "translateY(-50%)", color: "var(--text-faint)", fontSize: 15, pointerEvents: "none" }}>
            %
          </span>
        </div>
      )}
    </Field>
  );
}
