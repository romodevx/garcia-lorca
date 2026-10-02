import { describe, expect, it } from "vitest";

import { gradeFailure, GradeRequestError } from "./requestError";

describe("gradeFailure", () => {
  it("keeps the server's own words and status", async () => {
    const err = await gradeFailure(new Response("Solo se permiten calificaciones enteras\n", { status: 400 }));
    expect(err).toBeInstanceOf(GradeRequestError);
    expect(err.status).toBe(400);
    expect(err.message).toBe("Solo se permiten calificaciones enteras");
  });

  it("falls back to the status when the body is empty", async () => {
    const err = await gradeFailure(new Response("", { status: 503 }));
    expect(err.message).toBe("HTTP 503");
    expect(err.detail).toBe("");
  });

  it("carries the refusal's machine code from X-Error-Code", async () => {
    const res = new Response("Solo se permiten calificaciones enteras", { status: 400, headers: { "X-Error-Code": "GRADE_NOT_INTEGER" } });
    expect((await gradeFailure(res)).code).toBe("GRADE_NOT_INTEGER");
  });

  it("has no code when the server sent none", async () => {
    expect((await gradeFailure(new Response("x", { status: 400 }))).code).toBeUndefined();
  });
});
