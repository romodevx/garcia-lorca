import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { Toast } from "./Toast";

describe("Toast", () => {
  it("announces its message", () => {
    render(<Toast>Guardado</Toast>);

    expect(screen.getByRole("status")).toHaveTextContent("Guardado");
  });

  // Like UndoToast: a toast painted inside a hidden or transformed ancestor
  // vanishes or loses its fixed position. In document.body it does neither.
  it("lives in document.body, outside the surface that renders it", () => {
    const { container } = render(
      <section hidden>
        <Toast>Guardado</Toast>
      </section>,
    );

    const toast = screen.getByRole("status");
    expect(toast.parentElement).toBe(document.body);
    expect(container.contains(toast)).toBe(false);
    expect(toast).toBeVisible();
  });
});
