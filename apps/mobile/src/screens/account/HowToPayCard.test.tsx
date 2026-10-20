import { act, fireEvent, screen } from "@testing-library/react-native";
import * as Clipboard from "expo-clipboard";
import { HowToPayCard } from "./HowToPayCard";
import { renderWithProviders } from "../../test/renderWithProviders";

jest.mock("expo-clipboard", () => ({ setStringAsync: jest.fn(async () => true) }));

const INFO = { clabe: "012180001234567899", bank: "BBVA México", beneficiary: "Educación García Lorca, S.C.", reference: "1234567" };

describe("HowToPayCard (CLABE y referencia)", () => {
  afterEach(() => jest.clearAllMocks());

  it("shows the bank, who receives, the CLABE and the child's reference", async () => {
    await renderWithProviders(<HowToPayCard info={INFO} />);

    expect(screen.getByText("Cómo pagar por transferencia")).toBeTruthy();
    expect(screen.getByText("BBVA México")).toBeTruthy();
    expect(screen.getByText("Educación García Lorca, S.C.")).toBeTruthy();
    // Grouped as the bank prints it, so a parent can read it back digit by digit.
    expect(screen.getByText("012 180 00123456789 9")).toBeTruthy();
    expect(screen.getByText("1234567")).toBeTruthy();
  });

  it("copies the CLABE without the spaces, and says so", async () => {
    await renderWithProviders(<HowToPayCard info={INFO} />);

    // The copy settles inside act, so its "copiada" lands before the assertion.
    await act(async () => fireEvent.press(screen.getByLabelText("Copiar CLABE")));

    expect(Clipboard.setStringAsync).toHaveBeenCalledWith("012180001234567899");
    expect(screen.getByText("CLABE copiada")).toBeTruthy();
  });

  it("copies the reference", async () => {
    await renderWithProviders(<HowToPayCard info={INFO} />);

    await act(async () => fireEvent.press(screen.getByLabelText("Copiar referencia")));

    expect(Clipboard.setStringAsync).toHaveBeenCalledWith("1234567");
    expect(screen.getByText("Referencia copiada")).toBeTruthy();
  });

  // Owner's decision: without a reference the CLABE still shows, with a warning
  // so nobody makes a deposit the school cannot tell apart.
  it("without a reference, shows the CLABE and asks the family to request one first", async () => {
    await renderWithProviders(<HowToPayCard info={{ ...INFO, reference: "" }} />);

    expect(screen.getByText("012 180 00123456789 9")).toBeTruthy();
    expect(screen.getByText(/Pide tu referencia en la escuela antes de pagar/)).toBeTruthy();
    expect(screen.queryByLabelText("Copiar referencia")).toBeNull();
  });

  it("shows nothing while the school has not captured a CLABE", async () => {
    await renderWithProviders(<HowToPayCard info={{ ...INFO, clabe: "" }} />);

    expect(screen.queryByText("Cómo pagar por transferencia")).toBeNull();
  });
});
