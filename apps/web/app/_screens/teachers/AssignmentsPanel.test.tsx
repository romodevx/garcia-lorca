import { render, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { Cycle } from "../../config/api";
import { AssignmentsPanel } from "./AssignmentsPanel";

const CYCLE: Cycle = { id: "c1", name: "2026-2027", status: "draft", startsOn: "2026-08-31", endsOn: "2027-07-09" };

// Regression: a group created in the GroupsPanel used to be invisible in the
// assignments group dropdown until a full page reload, because the panel only
// re-fetched groups on cycle change. A bumped refreshKey must force a re-fetch.
describe("AssignmentsPanel — refreshKey", () => {
  it("re-fetches groups when refreshKey changes (newly created group appears without reload)", async () => {
    const loadGroups = vi.fn().mockResolvedValue([]);
    const noop = vi.fn().mockResolvedValue([]);
    const props = {
      cycles: [CYCLE],
      teachers: [],
      loadGroups,
      loadSubjects: noop,
      loadAssignments: noop,
      onCreate: vi.fn().mockResolvedValue(undefined),
    };

    const { rerender } = render(<AssignmentsPanel {...props} refreshKey={0} />);
    // The panel auto-selects the only cycle and loads its groups once.
    await waitFor(() => expect(loadGroups).toHaveBeenCalledTimes(1));

    rerender(<AssignmentsPanel {...props} refreshKey={1} />);
    // A bump (a group was created elsewhere) forces a second fetch.
    await waitFor(() => expect(loadGroups).toHaveBeenCalledTimes(2));
  });
});
