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
    ["/egresos", { screen: "expenses" }],
    ["/horarios", { screen: "schedule" }],
    ["/avisos", { screen: "announcements" }],
    ["/configuracion", { screen: "config" }],
    ["/administracion", { screen: "admin" }],
    ["/modulo/reportes", { screen: "module", moduleId: "reportes" }],
    ["/alumnos/stu-1/editar", { screen: "studentEdit", studentId: "stu-1" }],
    ["/docentes/tch-1/carga", { screen: "teacherLoad", teacherId: "tch-1" }],
    ["/configuracion/niveles/lv-1/eliminar", { screen: "levelDelete", levelId: "lv-1" }],
    ["/configuracion/niveles/lv-1/periodos", { screen: "levelPeriods", levelId: "lv-1" }],
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

  it("un id de docente con diagonal o vacío no se cuela", () => {
    expect(pathToView("/docentes//carga")).toEqual({ screen: "dashboard" });
    expect(pathToView("/docentes/a/b/carga")).toEqual({ screen: "dashboard" });
    expect(pathToView("/docentes/%E0%A4%A/carga")).toEqual({ screen: "dashboard" });
  });

  it("el id del docente viaja escapado en la dirección", () => {
    expect(viewToPath({ screen: "teacherLoad", teacherId: "a b" })).toBe("/docentes/a%20b/carga");
    expect(pathToView("/docentes/a%20b/carga")).toEqual({ screen: "teacherLoad", teacherId: "a b" });
  });

  it("un id de nivel con diagonal, vacío o roto no se cuela", () => {
    expect(pathToView("/configuracion/niveles//eliminar")).toEqual({ screen: "dashboard" });
    expect(pathToView("/configuracion/niveles/a/b/eliminar")).toEqual({ screen: "dashboard" });
    expect(pathToView("/configuracion/niveles/%E0%A4%A/eliminar")).toEqual({ screen: "dashboard" });
  });

  it("el id del nivel viaja escapado en la dirección", () => {
    expect(viewToPath({ screen: "levelDelete", levelId: "a b" })).toBe("/configuracion/niveles/a%20b/eliminar");
    expect(viewToPath({ screen: "levelPeriods", levelId: "a b" })).toBe("/configuracion/niveles/a%20b/periodos");
    expect(pathToView("/configuracion/niveles//periodos")).toEqual({ screen: "dashboard" });
  });

  it("un id con un escape roto no tumba la app", () => {
    expect(pathToView("/alumnos/%E0%A4%A/editar")).toEqual({ screen: "dashboard" });
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
