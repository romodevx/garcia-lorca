import { screen } from "@testing-library/react-native";
import { renderWithProviders } from "@/src/test/renderWithProviders";
import PerfilRoute from "../../app/perfil";

describe("PerfilRoute", () => {
  it("is a pushed detail screen titled Perfil del alumno, with a way back", async () => {
    await renderWithProviders(<PerfilRoute />);
    expect(screen.getByText("Perfil del alumno")).toBeTruthy();
    expect(screen.getByLabelText("Atrás")).toBeTruthy();
  });
});
