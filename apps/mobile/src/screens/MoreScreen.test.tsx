import { screen } from "@testing-library/react-native";
import { MoreScreen } from "./MoreScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("MoreScreen module grid", () => {
  it("hides modules deactivated from the panel (Documentos) but keeps the rest", async () => {
    await renderWithProviders(<MoreScreen onLogout={() => undefined} />);

    // "Documentos" is deactivated (school doesn't offer it) — kept future-ready
    // in mock data, but never rendered in the "Más" grid.
    expect(screen.queryByText("Documentos")).toBeNull();

    // Other module tiles still render normally.
    expect(screen.getByText("Uniformes")).toBeTruthy();
    expect(screen.getByText("Becas")).toBeTruthy();
  });
});
