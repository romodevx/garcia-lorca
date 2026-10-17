import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as api from "../../schedule/api";
import { GridSettings } from "./GridSettings";

vi.mock("../../schedule/api", async (importOriginal) => {
  const real = await importOriginal<typeof import("../../schedule/api")>();
  return { ...real, putSettings: vi.fn() };
});
const m = vi.mocked(api);
afterEach(() => vi.clearAllMocks());

const LEVEL = "lv-1";

function renderHours(gridStart = "07:00", gridEnd = "14:00"): ReturnType<typeof userEvent.setup> {
  const user = userEvent.setup();
  render(<GridSettings levelId={LEVEL} settings={[{ levelId: LEVEL, gridStart, gridEnd }]} onSaved={vi.fn()} />);
  return user;
}

async function typeTime(user: ReturnType<typeof userEvent.setup>, label: string, value: string): Promise<void> {
  const input = screen.getByLabelText(label);
  await user.click(input);
  fireEvent.change(input, { target: { value } });
  await user.tab();
}

describe("GridSettings · the level's hours are typed, within the school day (ISSUE-028)", () => {
  it("offers a typed time between 07:00 and 20:00, in 5-minute steps — not 288 options", () => {
    renderHours();
    for (const label of ["Empieza", "Termina"]) {
      const input = screen.getByLabelText(label);
      expect(input).toHaveAttribute("type", "time");
      expect(input).toHaveAttribute("min", "07:00");
      expect(input).toHaveAttribute("max", "20:00");
      expect(input).toHaveAttribute("step", "300");
    }
    expect(screen.queryAllByRole("option")).toHaveLength(0);
  });

  it("refuses a typed time outside the school day on blur, without asking the server", async () => {
    const user = renderHours();
    await typeTime(user, "Empieza", "05:00");
    expect(screen.getByLabelText("Empieza")).toHaveAccessibleDescription("Escribe una hora entre 07:00 y 20:00.");
    await user.click(screen.getByRole("button", { name: "Guardar horas" }));
    expect(m.putSettings).not.toHaveBeenCalled();
  });

  it("shows hours saved outside the school day as they are, and saves them untouched", async () => {
    m.putSettings.mockResolvedValue({ levelId: LEVEL, gridStart: "06:30", gridEnd: "21:00" });
    const user = renderHours("06:30", "21:00");
    expect(screen.getByLabelText("Empieza")).toHaveValue("06:30");
    expect(screen.getByLabelText("Termina")).toHaveValue("21:00");
    await user.click(screen.getByRole("button", { name: "Guardar horas" }));
    await waitFor(() => expect(m.putSettings).toHaveBeenCalledWith(expect.any(String), LEVEL, { gridStart: "06:30", gridEnd: "21:00" }));
  });

  it("saves typed hours", async () => {
    m.putSettings.mockResolvedValue({ levelId: LEVEL, gridStart: "07:30", gridEnd: "15:00" });
    const user = renderHours();
    await typeTime(user, "Empieza", "07:30");
    await typeTime(user, "Termina", "15:00");
    await user.click(screen.getByRole("button", { name: "Guardar horas" }));
    await waitFor(() => expect(m.putSettings).toHaveBeenCalledWith(expect.any(String), LEVEL, { gridStart: "07:30", gridEnd: "15:00" }));
  });
});
