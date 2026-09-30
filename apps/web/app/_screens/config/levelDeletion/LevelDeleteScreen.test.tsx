import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as configApi from "../../../config/api";
import { ConfigError, type LevelDeletionPreview } from "../../../config/api";
import { LevelDeleteScreen } from "./LevelDeleteScreen";

vi.mock("../../../config/api", async (importOriginal) => ({
  ConfigError: (await importOriginal<typeof import("../../../config/api")>()).ConfigError,
  getLevelDeletionPreview: vi.fn(),
  deleteLevel: vi.fn(),
}));
const api = vi.mocked(configApi);

const COUNTS = {
  gradeLevels: 6, groups: 12, subjects: 0, terms: 3, teachingAssignments: 30, students: 120, enrollments: 120,
  attendanceMarks: 900, monthlyGrades: 2000, reportCards: 0, tuitionPlans: 1, uniformItems: 0, schoolEvents: 2,
};
const WITH_DATA: LevelDeletionPreview = {
  levelId: "l1", levelName: "PRIMARIA", empty: false, counts: COUNTS, keeps: { charges: 10, payments: 8 },
  confirmToken: "tok-1", expiresAt: "2026-09-28T18:00:00Z",
};
const EMPTY: LevelDeletionPreview = {
  ...WITH_DATA, levelName: "MATERNAL", empty: true,
  counts: Object.fromEntries(Object.keys(COUNTS).map((k) => [k, 0])) as unknown as LevelDeletionPreview["counts"],
  keeps: { charges: 0, payments: 0 },
};
const DONE = { batchId: "b1", deletedAt: "2026-09-28T17:00:00Z", restorableUntil: "2026-10-05T17:00:00Z", counts: COUNTS };

beforeEach(() => {
  api.getLevelDeletionPreview.mockResolvedValue(WITH_DATA);
});
afterEach(() => vi.clearAllMocks());

function renderScreen() {
  const onBack = vi.fn();
  const onDeleted = vi.fn();
  render(<LevelDeleteScreen levelId="l1" onBack={onBack} onDeleted={onDeleted} />);
  return { onBack, onDeleted, user: userEvent.setup() };
}

async function toNameStep(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await user.click(await screen.findByRole("button", { name: "Continuar" }));
}

async function toPasswordStep(user: ReturnType<typeof userEvent.setup>): Promise<void> {
  await toNameStep(user);
  await user.type(screen.getByLabelText("Escribe el nombre del nivel para confirmar: PRIMARIA"), "primaria");
  await user.click(screen.getByRole("button", { name: "Continuar" }));
}

async function submitPassword(user: ReturnType<typeof userEvent.setup>, pw = "secreto"): Promise<void> {
  await user.type(screen.getByLabelText("Tu contraseña de director"), pw);
  await user.click(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }));
}

describe("step 1 — the summary", () => {
  it("lists what goes (zeros hidden) and what stays", async () => {
    renderScreen();

    expect(await screen.findByText("Se eliminará PRIMARIA y todo lo académico que depende de él.")).toBeInTheDocument();
    const goes = screen.getByRole("list", { name: "Se eliminará" });
    expect(within(goes).getByText("6 grados")).toBeInTheDocument();
    expect(within(goes).getByText("2000 calificaciones")).toBeInTheDocument();
    expect(within(goes).queryByText(/asignaturas|boletas/)).toBeNull();
    const stays = screen.getByRole("list", { name: "Se conserva" });
    expect(within(stays).getByText("10 cargos y 8 pagos en Finanzas")).toBeInTheDocument();
    expect(within(stays).getByText("120 alumnos quedarán sin nivel, grado ni grupo para reasignarlos")).toBeInTheDocument();
    expect(screen.getByText("Podrás restaurarlo durante 7 días desde la Papelera.")).toBeInTheDocument();
    expect(api.getLevelDeletionPreview).toHaveBeenCalledWith(expect.any(String), "l1");
  });

  it("an empty level says it goes entirely", async () => {
    api.getLevelDeletionPreview.mockResolvedValue(EMPTY);
    renderScreen();

    expect(await screen.findByText("Este nivel no tiene grados ni alumnos: se eliminará por completo.")).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "Se conserva" })).toBeNull();
  });

  it("marks the current step in the stepper", async () => {
    renderScreen();
    const steps = await screen.findByRole("list", { name: "Pasos" });
    expect(within(steps).getByText(/Resumen/).closest("li")).toHaveAttribute("aria-current", "step");
  });

  it("Cancelar goes back without deleting", async () => {
    const { onBack, user } = renderScreen();
    await user.click(await screen.findByRole("button", { name: "Cancelar" }));
    expect(onBack).toHaveBeenCalled();
    expect(api.deleteLevel).not.toHaveBeenCalled();
  });

  it("a level that no longer exists says so and points at the trash (QA, deferred)", async () => {
    api.getLevelDeletionPreview.mockRejectedValueOnce(new ConfigError(404, "not found"));
    renderScreen();

    expect(await screen.findByRole("alert")).toHaveTextContent("Este nivel ya no existe (puede estar en la Papelera).");
    expect(screen.queryByText(/No se pudo cargar/)).not.toBeInTheDocument();
  });

  it("a failed summary offers a retry", async () => {
    api.getLevelDeletionPreview.mockRejectedValueOnce(new TypeError("offline"));
    const { user } = renderScreen();

    expect(await screen.findByRole("alert")).toHaveTextContent("No se pudo cargar el resumen");
    await user.click(screen.getByRole("button", { name: "Reintentar" }));
    expect(await screen.findByText("Se eliminará PRIMARIA y todo lo académico que depende de él.")).toBeInTheDocument();
  });
});

describe("step 2 — typing the name", () => {
  it("moves focus to the step's heading", async () => {
    const { user } = renderScreen();
    await toNameStep(user);
    expect(screen.getByRole("heading", { name: "Confirma el nombre" })).toHaveFocus();
  });

  it("refuses a different name on submit, inline and announced", async () => {
    const { user } = renderScreen();
    await toNameStep(user);
    await user.type(screen.getByLabelText("Escribe el nombre del nivel para confirmar: PRIMARIA"), "SECUNDARIA");
    await user.click(screen.getByRole("button", { name: "Continuar" }));

    expect(screen.getByRole("alert")).toHaveTextContent("El nombre no coincide con PRIMARIA.");
    expect(screen.getByRole("button", { name: "Continuar" })).toBeEnabled();
  });

  it("checks on blur too", async () => {
    const { user } = renderScreen();
    await toNameStep(user);
    await user.type(screen.getByLabelText("Escribe el nombre del nivel para confirmar: PRIMARIA"), "PRIM");
    await user.tab();
    expect(screen.getByRole("alert")).toHaveTextContent("El nombre no coincide");
  });

  it("Atrás returns to the summary", async () => {
    const { user } = renderScreen();
    await toNameStep(user);
    await user.click(screen.getByRole("button", { name: "Atrás" }));
    expect(screen.getByRole("heading", { name: "Resumen" })).toHaveFocus();
  });
});

describe("step 3 — the password", () => {
  it("is a real password field and asks for it before sending", async () => {
    const { user } = renderScreen();
    await toPasswordStep(user);

    const pw = screen.getByLabelText("Tu contraseña de director");
    expect(pw).toHaveAttribute("type", "password");
    expect(pw).toHaveAttribute("autocomplete", "current-password");
    await user.click(screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Escribe tu contraseña.");
    expect(api.deleteLevel).not.toHaveBeenCalled();
  });

  it("deletes with the three confirmations and reports where to restore it", async () => {
    api.deleteLevel.mockResolvedValue(DONE);
    const { user, onDeleted } = renderScreen();
    await toPasswordStep(user);
    await submitPassword(user);

    // The server verifies what the human typed (trimmed), not the summary's copy.
    expect(api.deleteLevel).toHaveBeenCalledWith(expect.any(String), "l1", { confirmName: "primaria", confirmToken: "tok-1", password: "secreto" });
    await waitFor(() =>
      expect(onDeleted).toHaveBeenCalledWith("Nivel eliminado. Puedes restaurarlo desde la Papelera hasta el 5 de octubre de 2026."),
    );
  });

  it("a wrong password clears the field and says so", async () => {
    api.deleteLevel.mockRejectedValue(new ConfigError(401, "bad", "REAUTH_FAILED"));
    const { user, onDeleted } = renderScreen();
    await toPasswordStep(user);
    await submitPassword(user);

    expect(await screen.findByRole("alert")).toHaveTextContent("Contraseña incorrecta.");
    expect(screen.getByLabelText("Tu contraseña de director")).toHaveValue("");
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("NAME_MISMATCH sends the director back to the name", async () => {
    api.deleteLevel.mockRejectedValue(new ConfigError(400, "", "NAME_MISMATCH"));
    const { user } = renderScreen();
    await toPasswordStep(user);
    await submitPassword(user);

    expect(await screen.findByRole("heading", { name: "Confirma el nombre" })).toHaveFocus();
    expect(screen.getByRole("alert")).toHaveTextContent("El nombre no coincide con PRIMARIA.");
  });

  it("PREVIEW_STALE reloads the summary and starts over with a notice", async () => {
    api.deleteLevel.mockRejectedValue(new ConfigError(409, "", "PREVIEW_STALE"));
    api.getLevelDeletionPreview.mockResolvedValueOnce(WITH_DATA).mockResolvedValueOnce({ ...WITH_DATA, confirmToken: "tok-2" });
    const { user } = renderScreen();
    await toPasswordStep(user);
    await submitPassword(user);

    expect(await screen.findByRole("heading", { name: "Resumen" })).toBeInTheDocument();
    expect(await screen.findByRole("alert")).toHaveTextContent("El nivel cambió mientras confirmabas");
    expect(api.getLevelDeletionPreview).toHaveBeenCalledTimes(2);
  });

  it.each([
    [new ConfigError(403, ""), "Sólo el director puede eliminar niveles."],
    [new ConfigError(404, ""), "Este nivel ya no existe."],
    [new TypeError("Failed to fetch"), "No se pudo eliminar el nivel. Revisa tu conexión e intenta de nuevo."],
  ])("explains %s", async (err, text) => {
    api.deleteLevel.mockRejectedValue(err);
    const { user } = renderScreen();
    await toPasswordStep(user);
    await submitPassword(user);

    expect(await screen.findByRole("alert")).toHaveTextContent(text);
  });

  it("DELETION_IN_PROGRESS says a deletion already runs and points to the trash", async () => {
    api.deleteLevel.mockRejectedValue(new ConfigError(409, "Ya se está eliminando", "DELETION_IN_PROGRESS"));
    const { user, onDeleted } = renderScreen();
    await toPasswordStep(user);
    await submitPassword(user);

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Ya se está eliminando este nivel; espera un momento y revisa la Papelera.",
    );
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it("DELETION_INCOMPLETE shows the server's sentence, with its folio", async () => {
    const text = "No se pudo eliminar el nivel y no se pudo deshacer por completo (folio b-42). Aparece en la Papelera como eliminación incompleta.";
    api.deleteLevel.mockRejectedValue(new ConfigError(500, text, "DELETION_INCOMPLETE"));
    const { user } = renderScreen();
    await toPasswordStep(user);
    await submitPassword(user);

    expect(await screen.findByRole("alert")).toHaveTextContent("folio b-42");
  });

  it("a summary refused because the level is being deleted says so", async () => {
    api.getLevelDeletionPreview.mockReset();
    api.getLevelDeletionPreview.mockRejectedValue(new ConfigError(409, "", "DELETION_IN_PROGRESS"));
    renderScreen();

    expect(await screen.findByRole("alert")).toHaveTextContent("Ya se está eliminando este nivel; espera un momento y revisa la Papelera.");
  });

  it("a double click sends one delete", async () => {
    api.deleteLevel.mockReturnValue(new Promise(() => undefined));
    const { user } = renderScreen();
    await toPasswordStep(user);
    await user.type(screen.getByLabelText("Tu contraseña de director"), "secreto");
    const button = screen.getByRole("button", { name: "Eliminar nivel PRIMARIA" });
    await user.dblClick(button);
    await user.keyboard("{Enter}");

    expect(api.deleteLevel).toHaveBeenCalledTimes(1);
  });
});

describe("at phone width (360 px)", () => {
  it("wraps instead of scrolling sideways", async () => {
    const { container } = render(<LevelDeleteScreen levelId="l1" onBack={vi.fn()} onDeleted={vi.fn()} />);
    const steps = await screen.findByRole("list", { name: "Pasos" });

    expect(steps).toHaveStyle({ flexWrap: "wrap" });
    expect(screen.getByRole("button", { name: "Continuar" }).parentElement).toHaveStyle({ flexWrap: "wrap" });
    for (const el of container.querySelectorAll<HTMLElement>("[style]")) {
      const width = Number.parseFloat(el.style.width);
      const min = Number.parseFloat(el.style.minWidth);
      expect(Number.isNaN(width) || el.style.width.endsWith("%") || width <= 360).toBe(true);
      expect(Number.isNaN(min) || min <= 360).toBe(true);
    }
  });
});
