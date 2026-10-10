import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { SchoolProfilePanel } from "./SchoolProfilePanel";

const api = vi.hoisted(() => ({
  getSchoolProfile: vi.fn(),
  saveSchoolProfile: vi.fn(),
}));
vi.mock("../../config/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../config/api")>()),
  getSchoolProfile: api.getSchoolProfile,
  saveSchoolProfile: api.saveSchoolProfile,
}));

const EMPTY = { name: "", legalName: "", rfc: "", cct: "", address: "", phone: "", legend: "" };
const FILLED = { ...EMPTY, name: "Colegio García Lorca" };

beforeEach(() => {
  api.getSchoolProfile.mockReset();
  api.saveSchoolProfile.mockReset();
});

// The setup view learns step 1's status from the panel that already reads it:
// no second GET. The callback only notifies; it never changes what the panel does.
describe("SchoolProfilePanel onLoaded", () => {
  it("hands over the profile it read", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: FILLED, found: true });
    const onLoaded = vi.fn();
    render(<SchoolProfilePanel onLoaded={onLoaded} />);

    await waitFor(() => expect(onLoaded).toHaveBeenCalledWith(FILLED));
    expect(api.getSchoolProfile).toHaveBeenCalledTimes(1);
  });

  it("hands over the empty profile when the school has none yet", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: FILLED, found: false });
    const onLoaded = vi.fn();
    render(<SchoolProfilePanel onLoaded={onLoaded} />);

    await waitFor(() => expect(onLoaded).toHaveBeenCalledWith(EMPTY));
  });

  it("says the read failed", async () => {
    api.getSchoolProfile.mockRejectedValue(new Error("boom"));
    const onLoaded = vi.fn();
    render(<SchoolProfilePanel onLoaded={onLoaded} />);

    await waitFor(() => expect(onLoaded).toHaveBeenCalledWith("failed"));
    expect(await screen.findByText("No se pudieron leer los datos del colegio.")).toBeInTheDocument();
  });

  it("hands over what was saved", async () => {
    api.getSchoolProfile.mockResolvedValue({ profile: EMPTY, found: false });
    api.saveSchoolProfile.mockImplementation(async (_: string, p: typeof EMPTY) => p);
    const onLoaded = vi.fn();
    const user = userEvent.setup();
    render(<SchoolProfilePanel onLoaded={onLoaded} />);

    await user.click(await screen.findByRole("button", { name: "Capturar datos" }));
    await user.type(screen.getByLabelText(/Nombre del colegio/), "Colegio");
    await user.click(screen.getByRole("button", { name: "Guardar datos" }));

    await waitFor(() => expect(onLoaded).toHaveBeenLastCalledWith(expect.objectContaining({ name: "COLEGIO" })));
    expect(onLoaded).toHaveBeenCalledTimes(2);
  });
});
