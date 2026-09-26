import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";

import { TeachingPicker } from "./TeachingPicker";
import type { TeachingOption } from "./selection";

const mat2B: TeachingOption = { id: "a1", groupId: "g2b", groupName: "2° B", subjectId: "s-mat", subjectName: "Matemáticas" };
const mat3A: TeachingOption = { id: "a2", groupId: "g3a", groupName: "3° A", subjectId: "s-mat", subjectName: "Matemáticas" };
const esp2B: TeachingOption = { id: "a3", groupId: "g2b", groupName: "2° B", subjectId: "s-esp", subjectName: "Español" };
const ALL = [mat2B, mat3A, esp2B];

function List({ option }: { option: TeachingOption }): React.ReactElement {
  return <p>Lista de {option.id}</p>;
}

function renderPicker(options: readonly TeachingOption[], bySubject = true): void {
  render(
    <TeachingPicker title="Calificaciones" options={options} bySubject={bySubject}>
      {(selected) => <List option={selected} />}
    </TeachingPicker>,
  );
}

const crumbs = (): HTMLElement => screen.getByRole("navigation", { name: "Ruta" });

beforeEach(() => {
  window.history.replaceState(null, "", "/calificaciones");
});

describe("TeachingPicker — steps", () => {
  it("asks for the subject first, as large tappable choices", () => {
    renderPicker(ALL);

    expect(screen.getByRole("heading", { name: "Elige la asignatura" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Español/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Matemáticas/ })).toBeInTheDocument();
    expect(screen.queryByText(/Lista de/)).toBeNull();
  });

  it("subject → group → list, offering only that subject's groups", async () => {
    const user = userEvent.setup();
    renderPicker(ALL);

    await user.click(screen.getByRole("button", { name: /^Matemáticas/ }));
    expect(screen.getByRole("heading", { name: "Elige el grupo" })).toBeInTheDocument();
    const groupChoices = screen.getByRole("list", { name: "Grupos" });
    expect(within(groupChoices).getAllByRole("button").map((b) => b.textContent)).toEqual(["2° B", "3° A"]);

    await user.click(within(groupChoices).getByRole("button", { name: "3° A" }));
    expect(screen.getByText("Lista de a2")).toBeInTheDocument();
  });

  it("a subject with a single group opens the list directly", async () => {
    const user = userEvent.setup();
    renderPicker(ALL);

    await user.click(screen.getByRole("button", { name: /^Español/ }));

    expect(screen.getByText("Lista de a3")).toBeInTheDocument();
  });

  it("a teacher with one subject starts at the group step", () => {
    renderPicker([mat2B, mat3A]);

    expect(screen.queryByRole("heading", { name: "Elige la asignatura" })).toBeNull();
    expect(screen.getByRole("heading", { name: "Elige el grupo" })).toBeInTheDocument();
  });

  it("a teacher with one class goes straight to the list", () => {
    renderPicker([mat2B]);

    expect(screen.getByText("Lista de a1")).toBeInTheDocument();
  });

  it("group-only mode (attendance) is a single step", async () => {
    const user = userEvent.setup();
    const g1 = { id: "g1", groupId: "g1", groupName: "1° A" };
    const g2 = { id: "g2", groupId: "g2", groupName: "2° B" };
    renderPicker([g1, g2], false);

    await user.click(screen.getByRole("button", { name: "2° B" }));

    expect(screen.getByText("Lista de g2")).toBeInTheDocument();
    expect(window.location.search).toBe("?grupo=g2");
  });

  it("a subject with no name is offered as such, and opens", async () => {
    const user = userEvent.setup();
    const nameless: TeachingOption = { id: "a7", groupId: "g4c", groupName: "4° C", subjectId: "s-x", subjectName: "" };
    renderPicker([mat2B, nameless]);

    await user.click(screen.getByRole("button", { name: /^Asignatura sin nombre/ }));

    expect(screen.getByText("Lista de a7")).toBeInTheDocument();
    expect(crumbs()).toHaveTextContent("Calificaciones›Asignatura sin nombre›4° C");
  });

  it("two subjects with the same name are two choices, each opening its own class", async () => {
    const user = userEvent.setup();
    const matSec: TeachingOption = { id: "a9", groupId: "g1s", groupName: "1° Sec", subjectId: "s-mat-sec", subjectName: "Matemáticas" };
    renderPicker([mat2B, matSec]);

    const [first, second] = screen.getAllByRole("button", { name: /^Matemáticas/ });
    expect(first).toBeDefined();
    expect(second).toBeDefined();
    await user.click(second as HTMLElement);
    const opened = screen.getByText(/Lista de a/).textContent;
    await user.click(within(crumbs()).getByRole("button", { name: "Calificaciones" }));
    await user.click(screen.getAllByRole("button", { name: /^Matemáticas/ })[0] as HTMLElement);

    expect(screen.getByText(/Lista de a/).textContent).not.toBe(opened);
  });

  it("with no classes it says who to ask", () => {
    renderPicker([]);

    expect(screen.getByText("Aún no tienes grupos asignados. Pide a la dirección que te asigne tu carga académica.")).toBeInTheDocument();
  });
});

describe("TeachingPicker — the URL", () => {
  it("each choice is written to the query string, keeping the path", async () => {
    const user = userEvent.setup();
    renderPicker(ALL);

    await user.click(screen.getByRole("button", { name: /^Matemáticas/ }));
    expect(window.location.pathname).toBe("/calificaciones");
    expect(window.location.search).toBe("?asignatura=s-mat");

    await user.click(screen.getByRole("button", { name: "3° A" }));
    expect(window.location.search).toBe("?asignatura=s-mat&clase=a2");
  });

  it("a reload keeps the place", () => {
    window.history.replaceState(null, "", "/calificaciones?asignatura=s-mat&clase=a1");
    renderPicker(ALL);

    expect(screen.getByText("Lista de a1")).toBeInTheDocument();
  });

  it("the browser's Back returns to the previous step", async () => {
    const user = userEvent.setup();
    renderPicker(ALL);
    await user.click(screen.getByRole("button", { name: /^Matemáticas/ }));
    await user.click(screen.getByRole("button", { name: "3° A" }));

    act(() => window.history.back());
    await waitFor(() => expect(screen.getByRole("heading", { name: "Elige el grupo" })).toBeInTheDocument());

    act(() => window.history.back());
    await waitFor(() => expect(screen.getByRole("heading", { name: "Elige la asignatura" })).toBeInTheDocument());
  });
});

describe("TeachingPicker — breadcrumbs", () => {
  it("names the whole path, the last crumb being the current page", async () => {
    window.history.replaceState(null, "", "/calificaciones?asignatura=s-mat&clase=a1");
    renderPicker(ALL);

    expect(crumbs()).toHaveTextContent("Calificaciones›Matemáticas›2° B");
    expect(within(crumbs()).getByText("2° B")).toHaveAttribute("aria-current", "page");
  });

  it("each crumb goes back to its step", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/calificaciones?asignatura=s-mat&clase=a1");
    renderPicker(ALL);

    await user.click(within(crumbs()).getByRole("button", { name: "Matemáticas" }));
    expect(screen.getByRole("heading", { name: "Elige el grupo" })).toBeInTheDocument();

    await user.click(within(crumbs()).getByRole("button", { name: "Calificaciones" }));
    expect(screen.getByRole("heading", { name: "Elige la asignatura" })).toBeInTheDocument();
  });

  it("a skipped step is not a crumb that goes anywhere", () => {
    renderPicker([mat2B]);

    expect(crumbs()).toHaveTextContent("Calificaciones›Matemáticas›2° B");
    expect(within(crumbs()).queryAllByRole("button")).toHaveLength(0);
  });
});

describe("TeachingPicker — Cambiar", () => {
  it("switches to any other class in one tap and closes", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/calificaciones?asignatura=s-mat&clase=a1");
    renderPicker(ALL);

    const toggle = screen.getByRole("button", { name: "Cambiar" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    await user.click(screen.getByRole("button", { name: "Español · 2° B" }));

    expect(screen.getByText("Lista de a3")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cambiar" })).toHaveAttribute("aria-expanded", "false");
  });

  it("Escape closes it and gives the focus back to Cambiar", async () => {
    const user = userEvent.setup();
    window.history.replaceState(null, "", "/calificaciones?asignatura=s-mat&clase=a1");
    renderPicker(ALL);

    await user.click(screen.getByRole("button", { name: "Cambiar" }));
    await user.keyboard("{Escape}");

    expect(screen.queryByRole("button", { name: "Español · 2° B" })).toBeNull();
    expect(screen.getByRole("button", { name: "Cambiar" })).toHaveFocus();
  });

  it("is not offered when there is nothing to switch to", () => {
    renderPicker([mat2B]);

    expect(screen.queryByRole("button", { name: "Cambiar" })).toBeNull();
  });
});
