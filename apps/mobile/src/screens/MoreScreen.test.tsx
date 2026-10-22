import { screen } from "@testing-library/react-native";
import { MoreScreen } from "./MoreScreen";
import { renderWithProviders } from "../test/renderWithProviders";
import { useAppState } from "../state/AppState";

jest.mock("../state/AppState", () => ({
  ...jest.requireActual("../state/AppState"),
  useAppState: jest.fn(),
}));

describe("MoreScreen module grid (flag-gated)", () => {
  it("shows the shared title-cased display name and grade-level-group subtitle", async () => {
    jest.mocked(useAppState).mockReturnValue({
      child: { id: "s1", firstName: "VALENTINA", lastName: "GÓMEZ", maternalLastName: "RUIZ", level: "PRIMARIA", levelId: "l1", gradeGroup: "1° B" },
    } as ReturnType<typeof useAppState>);
    await renderWithProviders(<MoreScreen onLogout={() => undefined} />);
    expect(screen.getByText("Valentina Gómez")).toBeTruthy();
    expect(screen.getByText("1° PRIMARIA · B")).toBeTruthy();
    expect(screen.queryByText("GÓMEZ RUIZ VALENTINA")).toBeNull();
  });

  it("does not render a module tile whose flag is OFF", async () => {
    // Default demo flags keep the no-backend modules OFF (docs/billing/uniforms/
    // scholarships). None of their tiles should appear until the backend lands —
    // the mock content stays in source but is unreachable.
    await renderWithProviders(<MoreScreen onLogout={() => undefined} />);

    expect(screen.queryByText("Documentos")).toBeNull();
    expect(screen.queryByText("Uniformes")).toBeNull();
    expect(screen.queryByText("Facturas")).toBeNull();
    expect(screen.queryByText("Becas")).toBeNull();
  });

  it("never shows the mock Becas tile, even with module_scholarships ON (T18)", async () => {
    // Becas are real since T18, but the family sees them on the account
    // statement (a "Beca" line per charge); the tile's content is a mock.
    await renderWithProviders(<MoreScreen onLogout={() => undefined} />, {
      flags: { module_scholarships: true },
    });

    expect(screen.queryByText("Becas")).toBeNull();
  });

  // QA 21-oct: with every tile's flag OFF the "Módulos" label sat over an empty grid.
  it("hides the Módulos section when no tile is visible", async () => {
    await renderWithProviders(<MoreScreen onLogout={() => undefined} />);

    expect(screen.queryByText("Módulos")).toBeNull();
  });

  it("renders a module tile when its flag is turned ON", async () => {
    await renderWithProviders(<MoreScreen onLogout={() => undefined} />, {
      flags: { module_uniforms: true },
    });

    expect(screen.getByText("Uniformes")).toBeTruthy();
    expect(screen.getByText("Módulos")).toBeTruthy();
    // A sibling tile whose flag stays OFF is still hidden.
    expect(screen.queryByText("Becas")).toBeNull();
  });
});
