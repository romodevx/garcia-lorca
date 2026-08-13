import { render, screen, waitFor } from "@testing-library/react-native";
import { useMemo, type ReactElement } from "react";
import { Text } from "react-native";
import { useAsyncResource } from "./useAsyncResource";

/** Surfaces a resource's state as plain text for assertions. */
function Probe({ load }: { load: (() => Promise<string>) | null }): ReactElement {
  const memoized = useMemo(() => load, [load]);
  const { data, loading, error } = useAsyncResource(memoized);
  return (
    <>
      <Text testID="data">{data ?? "null"}</Text>
      <Text testID="loading">{String(loading)}</Text>
      <Text testID="error">{String(error)}</Text>
    </>
  );
}

const text = (id: string): unknown => screen.getByTestId(id).props.children;

describe("useAsyncResource", () => {
  it("settles to empty/non-loading/non-error with no fetcher", async () => {
    await render(<Probe load={null} />);
    await waitFor(() => expect(text("loading")).toBe("false"));
    expect(text("data")).toBe("null");
    expect(text("error")).toBe("false");
  });

  it("loads and exposes the resolved data", async () => {
    const load = jest.fn().mockResolvedValue("value");
    await render(<Probe load={load} />);
    await waitFor(() => expect(text("data")).toBe("value"));
    expect(text("loading")).toBe("false");
    expect(load).toHaveBeenCalledTimes(1);
  });

  it("flags an error when the fetcher rejects", async () => {
    const load = jest.fn().mockRejectedValue(new Error("boom"));
    await render(<Probe load={load} />);
    await waitFor(() => expect(text("error")).toBe("true"));
    expect(text("data")).toBe("null");
  });
});
