import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FeatureGate } from "./FeatureGate";
import { FlagsProvider } from "./FlagsProvider";

describe("FeatureGate", () => {
  it("renders children when the flag is ON", () => {
    render(
      <FlagsProvider flags={{ module_attendance: true }}>
        <FeatureGate flag="module_attendance">
          <span>attendance UI</span>
        </FeatureGate>
      </FlagsProvider>
    );
    expect(screen.getByText("attendance UI")).toBeInTheDocument();
  });

  it("hides children (renders fallback) when the flag is OFF", () => {
    render(
      <FlagsProvider flags={{ module_attendance: false }}>
        <FeatureGate flag="module_attendance" fallback={<span>disabled</span>}>
          <span>attendance UI</span>
        </FeatureGate>
      </FlagsProvider>
    );
    expect(screen.queryByText("attendance UI")).not.toBeInTheDocument();
    expect(screen.getByText("disabled")).toBeInTheDocument();
  });

  it("treats an absent flag as OFF (flags default OFF)", () => {
    render(
      <FlagsProvider flags={{}}>
        <FeatureGate flag="module_grades">
          <span>grades UI</span>
        </FeatureGate>
      </FlagsProvider>
    );
    expect(screen.queryByText("grades UI")).not.toBeInTheDocument();
  });
});
