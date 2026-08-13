import { screen } from "@testing-library/react-native";
import { MoreScreen } from "./MoreScreen";
import { renderWithProviders } from "../test/renderWithProviders";

describe("MoreScreen module grid (flag-gated)", () => {
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

  it("renders a module tile when its flag is turned ON", async () => {
    await renderWithProviders(<MoreScreen onLogout={() => undefined} />, {
      flags: { module_uniforms: true },
    });

    expect(screen.getByText("Uniformes")).toBeTruthy();
    // A sibling tile whose flag stays OFF is still hidden.
    expect(screen.queryByText("Becas")).toBeNull();
  });
});
