import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { downloadBlob } from "./downloadBlob";

describe("downloadBlob", () => {
  const createObjectURL = vi.fn(() => "blob:boleta");
  const revokeObjectURL = vi.fn();

  beforeEach(() => {
    vi.useFakeTimers({ toFake: ["setTimeout"] });
    vi.stubGlobal("URL", Object.assign(class extends URL {}, { createObjectURL, revokeObjectURL }));
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("clicks a temporary link with the file name, then frees the URL", () => {
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});

    downloadBlob(new Blob(["%PDF"]), "boleta-a-v1.pdf");

    const anchor = click.mock.contexts[0] as HTMLAnchorElement;
    expect(anchor.download).toBe("boleta-a-v1.pdf");
    expect(anchor.href).toBe("blob:boleta");
    expect(anchor.isConnected).toBe(false);
    expect(revokeObjectURL).not.toHaveBeenCalled();
    vi.runAllTimers();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:boleta");
  });
});
