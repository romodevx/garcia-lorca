import { describe, expect, it } from "vitest";

import { pathToView, viewToPath, type View } from "./routes";

describe("el mapa de rutas", () => {
  const pares: ReadonlyArray<readonly [string, View]> = [
    ["/", { screen: "dashboard" }],
    ["/entrar", { screen: "login" }],
    ["/cambiar-contrasena", { screen: "changePassword" }],
    ["/finanzas", { screen: "finance" }],
    ["/pagos", { screen: "payments" }],
    ["/alumnos", { screen: "students" }],
    ["/calificaciones", { screen: "grades" }],
    ["/asistencia", { screen: "attendance" }],
    ["/docentes", { screen: "teachers" }],
    ["/uniformes", { screen: "uniforms" }],
    ["/avisos", { screen: "announcements" }],
    ["/configuracion", { screen: "config" }],
    ["/administracion", { screen: "admin" }],
    ["/modulo/reportes", { screen: "module", moduleId: "reportes" }],
    ["/alumnos/stu-1/editar", { screen: "studentEdit", studentId: "stu-1" }],
  ];

  it.each(pares)("%s ida y vuelta", (path, view) => {
    expect(pathToView(path)).toEqual(view);
    expect(viewToPath(view)).toBe(path);
  });

  it("una ruta desconocida aterriza en el tablero, no en blanco", () => {
    expect(pathToView("/no-existe")).toEqual({ screen: "dashboard" });
  });

  it("un id de alumno con diagonal o vacío no se cuela", () => {
    expect(pathToView("/alumnos//editar")).toEqual({ screen: "dashboard" });
    expect(pathToView("/alumnos/a/b/editar")).toEqual({ screen: "dashboard" });
  });

  it("tolera la diagonal final", () => {
    expect(pathToView("/finanzas/")).toEqual({ screen: "finance" });
  });

  it("un id de módulo vacío no finge ser un módulo", () => {
    expect(pathToView("/modulo/")).toEqual({ screen: "dashboard" });
  });

  it("un id de módulo con diagonal no se cuela", () => {
    expect(pathToView("/modulo/a/b")).toEqual({ screen: "dashboard" });
  });

  it("una URL de fuera no puede empujar a nadie a otro sitio", () => {
    // El destino pendiente tras un link directo se guarda como View ya
    // resuelta, así que esto es lo que impide una redirección abierta.
    expect(viewToPath(pathToView("//evil.example.com"))).toBe("/");
    expect(viewToPath(pathToView("https://evil.example.com"))).toBe("/");
  });
});
