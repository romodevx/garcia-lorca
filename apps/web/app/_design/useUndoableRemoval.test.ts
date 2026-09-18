import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useUndoableRemoval } from "./useUndoableRemoval";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useUndoableRemoval", () => {
  it("borra de verdad cuando se acaba el plazo", () => {
    const commit = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());

    act(() => result.current.remove("a", commit));
    expect(commit).not.toHaveBeenCalled();

    act(() => { vi.advanceTimersByTime(8000); });

    expect(commit).toHaveBeenCalledTimes(1);
  });

  it("deshacer cancela el borrado y devuelve lo quitado", () => {
    const commit = vi.fn();
    const restored: string[] = [];
    const { result } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    act(() => { restored.push(...(result.current.undo() ?? [])); });
    act(() => { vi.advanceTimersByTime(8000); });

    expect(restored).toEqual(["a"]);
    expect(commit).not.toHaveBeenCalled();
  });

  /**
   * El defecto que esto vino a arreglar: se guardaba UN solo pendiente, así que
   * quitar un segundo renglón dentro del plazo tiraba el compromiso del primero.
   * La pantalla decía que se había ido y en el servidor seguía ahí — y para un
   * día cerrado eso deja mal la ventana de pronto pago de todas las familias.
   */
  it("quitar dos cosas seguidas borra las DOS", () => {
    const primero = vi.fn();
    const segundo = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());

    act(() => result.current.remove("a", primero));
    act(() => { vi.advanceTimersByTime(3000); });
    act(() => result.current.remove("b", segundo));
    act(() => { vi.advanceTimersByTime(8000); });

    expect(primero).toHaveBeenCalledTimes(1);
    expect(segundo).toHaveBeenCalledTimes(1);
  });

  it("deshacer sólo alcanza a lo último quitado", () => {
    const primero = vi.fn();
    const segundo = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", primero));
    act(() => result.current.remove("b", segundo));

    act(() => { result.current.undo(); });
    act(() => { vi.advanceTimersByTime(8000); });

    expect(segundo).not.toHaveBeenCalled();
    expect(primero).toHaveBeenCalledTimes(1);
  });

  /**
   * Irse de la pantalla dentro del plazo NO puede cancelar el borrado: la lista
   * ya lo mostró como ido. Se compromete de inmediato.
   */
  it("al desmontarse borra lo que quedaba pendiente, en vez de olvidarlo", () => {
    const commit = vi.fn();
    const { result, unmount } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    unmount();

    expect(commit).toHaveBeenCalledTimes(1);
  });

  it("lo pendiente se puede mirar, para anunciarlo", () => {
    const { result } = renderHook(() => useUndoableRemoval<string>());

    act(() => result.current.remove("INDEPENDENCIA", vi.fn()));

    expect(result.current.latest).toBe("INDEPENDENCIA");
  });
});
