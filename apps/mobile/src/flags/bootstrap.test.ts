import { demoFlags } from "./bootstrap";

describe("demoFlags", () => {
  it("ships the virtual card on, so the floating button reaches families", () => {
    expect(demoFlags.module_student_card).toBe(true);
  });

  it("keeps modules without a backend off", () => {
    expect(demoFlags.module_docs).toBe(false);
    expect(demoFlags.module_billing ?? false).toBe(false);
  });
});
