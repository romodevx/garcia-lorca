import { screen } from "@testing-library/react-native";
import { renderWithProviders } from "../test/renderWithProviders";
import { ModuleDetailScreen } from "./ModuleDetailScreen";

describe("ModuleDetailScreen — Documentos", () => {
  it("no longer lists a mock boleta: real boletas live in Calificaciones", async () => {
    await renderWithProviders(<ModuleDetailScreen id="documentos" />);

    expect(screen.queryByText(/Boleta/)).toBeNull();
    expect(screen.getByText("Constancia de estudios")).toBeTruthy();
  });
});
