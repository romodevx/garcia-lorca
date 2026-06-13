import { fireEvent, screen, waitFor } from "@testing-library/react-native";
import { AttendanceScreen } from "./AttendanceScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("AttendanceScreen Justificar flow", () => {
  it("flips the pending absence to excused after confirming a reason", async () => {
    await renderWithProviders(<AttendanceScreen />);

    // The unjustified-absence callout is visible.
    expect(screen.getByText("1 falta sin justificar")).toBeTruthy();

    // Open the excuse sheet via the callout's Justificar button.
    fireEvent.press(screen.getByRole("button", { name: "Justificar" }));

    // Pick a reason; the send button enables once a reason is selected.
    fireEvent.press(await screen.findByRole("radio", { name: "Cita médica" }));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Enviar justificante" }).props.accessibilityState.disabled).toBe(false);
    });
    fireEvent.press(screen.getByRole("button", { name: "Enviar justificante" }));

    // The callout disappears (no remaining unjustified absence) and the toast fires.
    await waitFor(() => {
      expect(screen.queryByText("1 falta sin justificar")).toBeNull();
    });
    expect(screen.getByText("Justificante enviado a la escuela")).toBeTruthy();
  });
});
