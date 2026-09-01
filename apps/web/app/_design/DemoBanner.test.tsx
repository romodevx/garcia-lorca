import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { DemoBanner } from "./DemoBanner";

describe("DemoBanner", () => {
  it("tells the reader not to enter real data", () => {
    render(<DemoBanner />);
    expect(screen.getByRole("note")).toHaveTextContent(/no captures información real/i);
  });

  it("names the environment so nobody mistakes it for production", () => {
    render(<DemoBanner />);
    expect(screen.getByText(/ambiente de demostración/i)).toBeInTheDocument();
  });
});
