import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useUndoableRemoval } from "./useUndoableRemoval";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe("useUndoableRemoval", () => {
  /**
   * ISSUE-CC-105: el aviso se iba solo a los ocho segundos y el borrado se
   * ejecutaba en ese momento, aunque la persona todavía estuviera leyendo. La
   * regla de la casa pide que el Deshacer NO se cierre solo: el borrado espera a
   * que la persona cierre el aviso, haga otra cosa o se vaya.
   */
  it("no borra por temporizador: el plazo es de la persona, no del reloj", () => {
    const commit = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());

    act(() => result.current.remove("a", commit));
    act(() => { vi.advanceTimersByTime(60_000); });

    expect(commit).not.toHaveBeenCalled();
    expect(result.current.latest).toBe("a");
  });

  it("cerrar el aviso borra de verdad lo pendiente", () => {
    const commit = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    act(() => result.current.dismiss());

    expect(commit).toHaveBeenCalledTimes(1);
    expect(result.current.latest).toBeUndefined();
  });

  it("deshacer cancela el borrado y devuelve lo quitado", () => {
    const commit = vi.fn();
    const restored: string[] = [];
    const { result, unmount } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    act(() => { restored.push(...(result.current.undo() ?? [])); });
    unmount();

    expect(restored).toEqual(["a"]);
    expect(commit).not.toHaveBeenCalled();
  });

  /**
   * Quitar otra cosa es "otra acción": lo anterior ya no se ofrece para
   * deshacer, así que se borra en ese momento. Y nunca se pierde: el defecto
   * original era que el segundo renglón tiraba el compromiso del primero.
   */
  it("quitar una segunda cosa borra la primera y deja deshacible sólo la segunda", () => {
    const primero = vi.fn();
    const segundo = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());

    act(() => result.current.remove("a", primero));
    act(() => result.current.remove("b", segundo));

    expect(primero).toHaveBeenCalledTimes(1);
    expect(segundo).not.toHaveBeenCalled();
    expect(result.current.latest).toBe("b");
    expect(result.current.pending).toEqual(["b"]);
  });

  it("flush borra lo pendiente cuando la pantalla lanza otra acción", () => {
    const commit = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    act(() => result.current.flush());

    expect(commit).toHaveBeenCalledTimes(1);
    expect(result.current.latest).toBeUndefined();
  });

  /**
   * Irse de la pantalla NO puede cancelar el borrado: la lista ya lo mostró
   * como ido. Se compromete de inmediato.
   */
  it("al desmontarse borra lo que quedaba pendiente, en vez de olvidarlo", () => {
    const commit = vi.fn();
    const { result, unmount } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    unmount();

    expect(commit).toHaveBeenCalledTimes(1);
  });

  it("cerrar la pestaña también compromete lo pendiente", () => {
    const commit = vi.fn();
    const { result } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    act(() => { window.dispatchEvent(new Event("pagehide")); });

    expect(commit).toHaveBeenCalledTimes(1);
  });

  it("nada se borra dos veces", () => {
    const commit = vi.fn();
    const { result, unmount } = renderHook(() => useUndoableRemoval<string>());
    act(() => result.current.remove("a", commit));

    act(() => result.current.dismiss());
    act(() => { window.dispatchEvent(new Event("pagehide")); });
    unmount();

    expect(commit).toHaveBeenCalledTimes(1);
  });

  it("lo pendiente se puede mirar, para anunciarlo", () => {
    const { result } = renderHook(() => useUndoableRemoval<string>());

    act(() => result.current.remove("INDEPENDENCIA", vi.fn()));

    expect(result.current.latest).toBe("INDEPENDENCIA");
  });
});
