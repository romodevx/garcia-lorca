import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState, type ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { ConfigError, type Level } from "../../config/api";
import { LevelOrder } from "./LevelOrder";

const level = (id: string, name: string): Level => ({ id, name, sepCode: "", enrollmentGranularity: "cycle" });
const LEVELS = [level("l1", "PREESCOLAR"), level("l2", "PRIMARIA"), level("l3", "SECUNDARIA")];

function setup(onReorder = vi.fn().mockResolvedValue(undefined)) {
  const user = userEvent.setup();
  render(<LevelOrder levels={LEVELS} onReorder={onReorder} />);
  return { user, onReorder };
}

describe("LevelOrder", () => {
  it("lists the levels in the order given (the server's), numbered", () => {
    setup();
    const items = within(screen.getByRole("list", { name: "Orden de los niveles" })).getAllByRole("listitem");
    expect(items.map((li) => li.textContent)).toEqual([
      expect.stringContaining("PREESCOLAR"),
      expect.stringContaining("PRIMARIA"),
      expect.stringContaining("SECUNDARIA"),
    ]);
  });

  it("the first has no Subir and the last no Bajar: absent, not disabled", () => {
    setup();
    expect(screen.queryByRole("button", { name: "Subir PREESCOLAR" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Bajar SECUNDARIA" })).toBeNull();
    expect(screen.getByRole("button", { name: "Subir PRIMARIA" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Bajar PRIMARIA" })).toBeEnabled();
  });

  it("each move sends the whole ordered id list", async () => {
    const { user, onReorder } = setup();
    await user.click(screen.getByRole("button", { name: "Bajar PREESCOLAR" }));
    expect(onReorder).toHaveBeenCalledWith(["l2", "l1", "l3"]);
    await user.click(screen.getByRole("button", { name: "Subir SECUNDARIA" }));
    expect(onReorder).toHaveBeenLastCalledWith(["l1", "l3", "l2"]);
  });

  it("a refused move shows why (the server's sentence)", async () => {
    const onReorder = vi.fn().mockRejectedValue(new ConfigError(400, "La lista de niveles no coincide con los niveles del colegio.", "LEVEL_ORDER_MISMATCH"));
    const { user } = setup(onReorder);
    await user.click(screen.getByRole("button", { name: "Subir PRIMARIA" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("La lista de niveles no coincide con los niveles del colegio.");
  });

  it("a move to an edge keeps the focus on the level's remaining arrow", async () => {
    const user = userEvent.setup();
    function Host(): ReactElement {
      const [levels, setLevels] = useState(LEVELS);
      const onReorder = async (ids: string[]): Promise<void> => setLevels(ids.map((id) => LEVELS.find((l) => l.id === id) as Level));
      return <LevelOrder levels={levels} onReorder={onReorder} />;
    }
    render(<Host />);
    await user.click(screen.getByRole("button", { name: "Subir PRIMARIA" }));
    expect(screen.queryByRole("button", { name: "Subir PRIMARIA" })).toBeNull();
    expect(screen.getByRole("button", { name: "Bajar PRIMARIA" })).toHaveFocus();
  });

  it("arrows are ≥44px targets", () => {
    setup();
    const b = screen.getByRole("button", { name: "Subir PRIMARIA" });
    expect(b.style.minHeight).toBe("44px");
    expect(b.style.minWidth).toBe("44px");
  });

  // PLAN-redesign-estructura (tech-lead M3): the arrow takes the focus once; a
  // later change to the list (a new level, a rename) must not pull it back.
  it("puts the focus on the moved arrow once, never again on a later list change", async () => {
    function Host(): ReactElement {
      const [levels, setLevels] = useState(LEVELS);
      return (
        <>
          <input aria-label="Otro campo" />
          <button type="button" onClick={() => setLevels((prev) => [...prev, level("l4", "BACHILLERATO")])}>Agregar</button>
          <LevelOrder levels={levels} onReorder={async () => undefined} />
        </>
      );
    }
    const user = userEvent.setup();
    render(<Host />);
    await user.click(screen.getByRole("button", { name: "Bajar PREESCOLAR" }));
    await user.click(screen.getByRole("textbox", { name: "Otro campo" }));
    await user.click(screen.getByRole("button", { name: "Agregar" }));
    expect(screen.getByRole("button", { name: "Agregar" })).toHaveFocus();
  });
});

