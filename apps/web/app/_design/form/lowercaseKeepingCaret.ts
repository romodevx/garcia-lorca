import type { SyntheticEvent } from "react";

/**
 * Lowers what the field holds WITHOUT moving the caret, and returns the lowered
 * value for the caller to keep in state.
 *
 * Why: storing only `value.toLowerCase()` in state makes React see a value that
 * differs from the DOM, rewrite it, and jump the caret to the end. Somebody who
 * fixes "ana.ruiz@…" mid-address and types a capital ends up with
 * "ana.mruiz@colegio.mxaria". Writing the lowered value into the DOM before
 * React does, and restoring the selection, leaves state and DOM equal, so React
 * touches nothing.
 *
 * The caret is re-derived from the lowered PREFIX, not reused as an offset:
 * lowering can change length ("İ" → "i̇" is two UTF-16 units).
 *
 * While an IME is composing (Android keyboards with predictive text, CJK input)
 * the field is left alone: rewriting it mid-composition makes the keyboard
 * re-commit the composed text and characters duplicate. Call it again on
 * `compositionend`.
 *
 * Needs a field that exposes its selection: `type="text"` (with
 * `inputMode="email"` for the phone keyboard). `type="email"` does not.
 */
export function lowercaseKeepingCaret(input: HTMLInputElement, composing: boolean): string {
  const typed = input.value;
  const lowered = typed.toLowerCase();
  if (composing || lowered === typed) {
    return typed;
  }
  const start = input.selectionStart;
  const end = input.selectionEnd;
  input.value = lowered;
  // Only the focused field has a caret worth keeping; setting a selection on an
  // unfocused one (autofill) can pull the caret or focus back to it.
  if (start !== null && end !== null && input.ownerDocument.activeElement === input) {
    input.setSelectionRange(loweredOffset(typed, start), loweredOffset(typed, end));
  }
  return lowered;
}

/** Whether the event that changed the field is part of an IME composition. */
export function isComposing(event: SyntheticEvent): boolean {
  const native = event.nativeEvent as Partial<InputEvent>;
  return native.isComposing === true;
}

function loweredOffset(typed: string, offset: number): number {
  return typed.slice(0, offset).toLowerCase().length;
}
