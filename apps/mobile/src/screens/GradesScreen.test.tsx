import { fireEvent, screen } from "@testing-library/react-native";
import { GradesScreen } from "./GradesScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("GradesScreen level toggle", () => {
  it("shows the numeric promedio for primaria", async () => {
    await renderWithProviders(<GradesScreen />, { initialLevelId: "primaria" });
    expect(screen.getByText("Promedio general")).toBeTruthy();
    expect(screen.getByText("9.3")).toBeTruthy();
    expect(screen.getByText("Materias")).toBeTruthy();
  });

  it("flips to qualitative rendering when Kínder is selected", async () => {
    await renderWithProviders(<GradesScreen />, { initialLevelId: "primaria" });

    fireEvent.press(screen.getByRole("tab", { name: "Kínder" }));

    // Qualitative hero + section replace the numeric average/materias.
    expect(await screen.findByText("Evaluación cualitativa")).toBeTruthy();
    expect(screen.getByText("Campos formativos")).toBeTruthy();
    expect(screen.queryByText("Promedio general")).toBeNull();
  });

  it("flips back to numeric when Secundaria is selected", async () => {
    await renderWithProviders(<GradesScreen />, { initialLevelId: "kinder" });
    expect(screen.getByText("Evaluación cualitativa")).toBeTruthy();

    fireEvent.press(screen.getByRole("tab", { name: "Secundaria" }));
    expect(await screen.findByText("Promedio general")).toBeTruthy();
  });
});
