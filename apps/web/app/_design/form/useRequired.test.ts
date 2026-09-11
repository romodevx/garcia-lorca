import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { useRequired } from "./useRequired";

describe("useRequired", () => {
  it("no reclama nada antes de guardar", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    expect(result.current.errors).toEqual({});
    expect(result.current.missingCount).toBe(0);
  });

  it("al guardar marca TODOS los faltantes a la vez", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ nombre: "", fecha: "", nota: "" }); });
    expect(Object.keys(result.current.errors).sort()).toEqual(["fecha", "nombre"]);
  });

  it("no reclama un campo opcional vacío", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    act(() => { result.current.check({ nombre: "Ana", nota: "" }); });
    expect(result.current.errors).toEqual({});
  });

  it("el espacio en blanco no llena un campo", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    act(() => { result.current.check({ nombre: "   " }); });
    expect(result.current.errors.nombre).toBeDefined();
  });

  it("un campo que ni siquiera viene cuenta como faltante", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    act(() => { result.current.check({}); });
    expect(result.current.errors.nombre).toBeDefined();
  });

  it("check dice si se puede guardar", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    let ok = false;
    act(() => { ok = result.current.check({ nombre: "" }); });
    expect(ok).toBe(false);
    act(() => { ok = result.current.check({ nombre: "Ana" }); });
    expect(ok).toBe(true);
  });

  it("escribir en un campo limpia SU rojo y deja los demás", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ nombre: "", fecha: "" }); });
    act(() => { result.current.clear("nombre"); });
    expect(result.current.errors.nombre).toBeUndefined();
    expect(result.current.errors.fecha).toBeDefined();
  });

  it("limpiar un campo que no estaba en rojo no cuesta un repintado", () => {
    const { result } = renderHook(() => useRequired(["nombre"]));
    const antes = result.current.errors;
    act(() => { result.current.clear("nombre"); });
    expect(result.current.errors).toBe(antes);
  });

  it("nombra el primer faltante en el orden DECLARADO, no el del objeto", () => {
    // La vista salta al primero que el ojo encuentra bajando por la forma, y el
    // orden de las llaves de un objeto no es el de la pantalla.
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ fecha: "", nombre: "" }); });
    expect(result.current.firstMissing).toBe("nombre");
  });

  it("cuenta cuántos faltan, para el resumen de arriba", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ nombre: "", fecha: "" }); });
    expect(result.current.missingCount).toBe(2);
  });

  it("un segundo intento no arrastra los rojos ya resueltos", () => {
    const { result } = renderHook(() => useRequired(["nombre", "fecha"]));
    act(() => { result.current.check({ nombre: "", fecha: "" }); });
    act(() => { result.current.check({ nombre: "Ana", fecha: "" }); });
    expect(result.current.errors.nombre).toBeUndefined();
    expect(result.current.missingCount).toBe(1);
  });
});
