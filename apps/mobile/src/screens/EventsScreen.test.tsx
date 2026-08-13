import { screen } from "@testing-library/react-native";
import { EventsScreen } from "./EventsScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("EventsScreen", () => {
  it("shows a neutral 'Próximamente' state instead of fake agenda items", async () => {
    // Agenda has no backend yet (comms agenda → Phase 9d). The mock `events`
    // stay in source but must NOT be rendered — showing fake events misleads.
    await renderWithProviders(<EventsScreen />);

    expect(screen.getByText("Próximamente")).toBeTruthy();
    expect(screen.queryByText("Suspensión de clases")).toBeNull();
    expect(screen.queryByText("Reunión de padres")).toBeNull();
    expect(screen.queryByText("Festival de Primavera")).toBeNull();
  });
});
