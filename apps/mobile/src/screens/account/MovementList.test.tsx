import { screen } from "@testing-library/react-native";

import type { FamilyMovement } from "../../api";
import { renderWithProviders } from "../../test/renderWithProviders";
import { MovementList } from "./MovementList";

const payment = {
  kind: "payment",
  date: "2026-10-17",
  description: "Pago · Inscripción",
  amountCents: 300000,
  method: "card",
} as FamilyMovement;

// ISSUE-T14-402: the line already says "Pago · …", so its second line must
// not say "Pago" again.
describe("MovementList", () => {
  it("does not repeat the kind when the description already names it", async () => {
    await renderWithProviders(<MovementList movements={[payment]} />);
    expect(screen.getByText("Pago · Inscripción")).toBeTruthy();
    expect(screen.getByText("17 oct · Tarjeta")).toBeTruthy();
  });

  it("keeps the kind when the description does not name it", async () => {
    await renderWithProviders(<MovementList movements={[{ ...payment, description: "Colegiatura octubre" }]} />);
    expect(screen.getByText("Pago · 17 oct · Tarjeta")).toBeTruthy();
  });
});
