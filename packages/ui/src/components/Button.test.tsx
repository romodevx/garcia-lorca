import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "./Button";

describe("Button", () => {
  it("renders its label and defaults to the primary variant", () => {
    render(<Button>Guardar</Button>);
    const button = screen.getByRole("button", { name: "Guardar" });
    expect(button).toBeInTheDocument();
    expect(button).toHaveAttribute("data-variant", "primary");
  });
});
