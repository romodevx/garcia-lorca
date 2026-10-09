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

  it("gates the legacy student import on its own flag, OFF unless set", () => {
    const { rerender } = render(
      <FlagsProvider flags={{ module_students: true, module_config: true }}>
        <FeatureGate flag="module_student_import">
          <span>import UI</span>
        </FeatureGate>
      </FlagsProvider>
    );
    expect(screen.queryByText("import UI")).not.toBeInTheDocument();

    rerender(
      <FlagsProvider flags={{ module_student_import: true }}>
        <FeatureGate flag="module_student_import">
          <span>import UI</span>
        </FeatureGate>
      </FlagsProvider>
    );
    expect(screen.getByText("import UI")).toBeInTheDocument();
  });

  it("gates the Configuración setup view on its own flag, OFF while only module_config is on", () => {
    const { rerender } = render(
      <FlagsProvider flags={{ module_config: true }}>
        <FeatureGate flag="module_config_setup">
          <span>setup view</span>
        </FeatureGate>
      </FlagsProvider>
    );
    expect(screen.queryByText("setup view")).not.toBeInTheDocument();

    rerender(
      <FlagsProvider flags={{ module_config: true, module_config_setup: true }}>
        <FeatureGate flag="module_config_setup">
          <span>setup view</span>
        </FeatureGate>
      </FlagsProvider>
    );
    expect(screen.getByText("setup view")).toBeInTheDocument();
  });
});
