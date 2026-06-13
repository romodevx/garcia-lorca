import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { AccountScreen } from "./AccountScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("AccountScreen Pagar ahora flow", () => {
  it("reduces the balance to zero and toasts after paying", async () => {
    await renderWithProviders(<AccountScreen />);

    // Starts with the outstanding balance.
    expect(screen.getByText("$1,850.00")).toBeTruthy();

    // Open the pay sheet and confirm.
    fireEvent.press(screen.getByRole("button", { name: "Pagar $1,850.00" }));
    fireEvent.press(await screen.findByRole("button", { name: "Confirmar pago" }));

    // Balance flips to settled and the success toast fires.
    await waitFor(() => {
      expect(screen.getByText("$0.00")).toBeTruthy();
    });
    expect(screen.getByText("Al corriente · ¡Gracias!")).toBeTruthy();
    expect(screen.getByText("¡Pago realizado con éxito!")).toBeTruthy();
  });
});
