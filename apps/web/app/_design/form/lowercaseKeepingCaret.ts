/**
 * Baja a minúsculas lo que hay en el campo SIN mover el cursor, y devuelve el
 * valor ya bajado para guardarlo en el estado.
 *
 * Por qué hace falta: si sólo se guarda `value.toLowerCase()` en el estado,
 * React ve un valor distinto al del DOM, lo reescribe y el cursor salta al
 * final. Quien corrige "ana.ruiz@…" a media dirección y teclea una mayúscula
 * termina con "ana.mruiz@colegio.mxaria". Escribiendo el valor bajado en el
 * DOM antes que React y restaurando la selección, el estado y el DOM ya
 * coinciden y React no toca nada.
 *
 * Requiere un campo que exponga la selección: `type="text"` (con
 * `inputMode="email"` para el teclado del teléfono). `type="email"` no la
 * expone.
 */
export function lowercaseKeepingCaret(input: HTMLInputElement): string {
  const typed = input.value;
  const lowered = typed.toLowerCase();
  if (lowered === typed) {
    return typed;
  }
  const start = input.selectionStart;
  const end = input.selectionEnd;
  input.value = lowered;
  if (start !== null && end !== null) {
    input.setSelectionRange(Math.min(start, lowered.length), Math.min(end, lowered.length));
  }
  return lowered;
}
