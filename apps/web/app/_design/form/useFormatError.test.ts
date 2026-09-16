import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useFormatError, type FormatRule } from "./useFormatError";

const RULES: Readonly<Record<string, FormatRule>> = {
  endsOn: (value, all) => {
    const startsOn = all.startsOn ?? "";
    return startsOn !== "" && value < startsOn ? "El fin es antes que el inicio." : undefined;
  },
};

describe("useFormatError", () => {
  /**
   * La decisión que `useRequired` ya tomó, y que aquí NO se toca: un campo
   * VACÍO no se reclama al salir de él. Perseguir con rojos a quien apenas va
   * llegando al siguiente campo es justo lo que ese código quiso evitar, y tiene
   * razón. Lo que sí gana con revisarse en blur es el dato imposible.
   */
  it("no dice nada de un campo vacío", () => {
    const { result } = renderHook(() => useFormatError(RULES));

    act(() => result.current.onBlur("endsOn", "", { startsOn: "2026-09-01" }));

    expect(result.current.errors.endsOn).toBeUndefined();
  });

  it("reclama el dato imposible al salir del campo", () => {
    const { result } = renderHook(() => useFormatError(RULES));

    act(() => result.current.onBlur("endsOn", "2026-08-01", { startsOn: "2026-09-01" }));

    expect(result.current.errors.endsOn).toBe("El fin es antes que el inicio.");
  });

  // Se valida al salir, pero se PERDONA por tecla: en cuanto el valor vuelve a
  // ser válido el rojo se va, sin esperar a que la persona salga otra vez.
  it("borra el error por tecla en cuanto el valor vuelve a ser válido", () => {
    const { result } = renderHook(() => useFormatError(RULES));
    act(() => result.current.onBlur("endsOn", "2026-08-01", { startsOn: "2026-09-01" }));

    act(() => result.current.onChange("endsOn", "2026-10-01", { startsOn: "2026-09-01" }));

    expect(result.current.errors.endsOn).toBeUndefined();
  });

  it("nunca acusa mientras se teclea", () => {
    const { result } = renderHook(() => useFormatError(RULES));

    act(() => result.current.onChange("endsOn", "2026-08-01", { startsOn: "2026-09-01" }));

    expect(result.current.errors.endsOn).toBeUndefined();
  });

  it("ignora los campos sin regla", () => {
    const { result } = renderHook(() => useFormatError(RULES));

    act(() => result.current.onBlur("label", "cualquier cosa", {}));

    expect(result.current.errors.label).toBeUndefined();
  });
});
